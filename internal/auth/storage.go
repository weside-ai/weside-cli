// Package auth handles authentication token storage and retrieval.
package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/weside-ai/weside-cli/internal/config"
)

// refreshLeeway is how close to its exp a stored access token may get before
// GetToken refreshes it, so a caller never sends a token that expires in flight.
const refreshLeeway = 60 * time.Second

// lockTimeout bounds the wait for another process's refresh (its HTTP call
// times out after 30 s, see supabaseHTTPClient).
const lockTimeout = 45 * time.Second

// Tokens holds the stored authentication tokens.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Storage handles token persistence.
type Storage struct {
	filePath string
}

// NewStorage creates a new token storage.
func NewStorage() *Storage {
	home, _ := os.UserHomeDir()
	return &Storage{
		filePath: filepath.Join(home, ".weside", "credentials.json"),
	}
}

// SetFilePath overrides the storage file path (for testing).
func (s *Storage) SetFilePath(path string) {
	s.filePath = path
}

// Save stores tokens to the filesystem with mode 0600. It writes a temp file in
// the same directory and renames it over the old one, so a reader never sees a
// half-written pair.
func (s *Storage) Save(tokens *Tokens) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tokens: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("writing tokens: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing tokens: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing tokens: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing tokens: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing tokens: %w", err)
	}
	if err := os.Rename(tmpName, s.filePath); err != nil {
		return fmt.Errorf("writing tokens: %w", err)
	}
	return nil
}

// Load retrieves stored tokens.
func (s *Storage) Load() (*Tokens, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("not logged in (run: weside auth login)")
		}
		return nil, fmt.Errorf("reading tokens: %w", err)
	}

	var tokens Tokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("parsing tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, fmt.Errorf("no access token found (run: weside auth login)")
	}

	return &tokens, nil
}

// Delete removes stored tokens.
func (s *Storage) Delete() error {
	if err := os.Remove(s.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing tokens: %w", err)
	}
	return nil
}

// GetToken returns a usable access token. WESIDE_TOKEN is returned as is.
// Otherwise the stored access token is returned, refreshed first when its exp
// lies within refreshLeeway — see Storage.freshAccessToken.
func GetToken() (string, error) {
	// Check env var first (CI/headless)
	if token := os.Getenv("WESIDE_TOKEN"); token != "" {
		return token, nil
	}
	return NewStorage().freshAccessToken(config.APIURL())
}

// freshAccessToken returns the stored access token, refreshing it when it
// expires within refreshLeeway. Supabase rotates the refresh token on every
// use and a replayed one can revoke the session, so the refresh runs under an
// exclusive lock on <credentials>.lock and re-reads the file after acquiring
// it: a concurrent process that refreshed first leaves a fresh pair behind,
// and this call returns that instead of spending the old refresh token again
// (pinned by TestGetTokenConcurrentGoroutinesRefreshOnce and
// TestGetTokenTwoProcessesRefreshOnce). On a failed refresh the file is left
// as it was.
func (s *Storage) freshAccessToken(apiURL string) (string, error) {
	tokens, err := s.Load()
	if err != nil {
		return "", err
	}
	if !needsRefresh(tokens.AccessToken) {
		return tokens.AccessToken, nil
	}

	unlock, err := lockFile(s.filePath+".lock", lockTimeout)
	if err != nil {
		return "", err
	}
	defer unlock()

	tokens, err = s.Load()
	if err != nil {
		return "", err
	}
	if !needsRefresh(tokens.AccessToken) {
		return tokens.AccessToken, nil
	}
	if tokens.RefreshToken == "" {
		return "", errors.New("session expired and no refresh token stored — please re-login with: weside auth login")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resolved := Resolve(ctx, apiURL)
	if errors.Is(resolved.FetchError, ErrPartialOverride) {
		return "", resolved.FetchError
	}
	cfg := resolved.Config
	result, err := RefreshAccessToken(cfg.SupabaseURL, cfg.SupabaseAnonKey, cfg.OAuthClientID, tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if result.AccessToken == "" {
		return "", errors.New("token refresh returned no access token — please re-login with: weside auth login")
	}
	rotated := &Tokens{AccessToken: result.AccessToken, RefreshToken: result.RefreshToken}
	if rotated.RefreshToken == "" {
		// The server may keep the refresh token unrotated (RFC 6749 § 6).
		rotated.RefreshToken = tokens.RefreshToken
	}
	if err := s.Save(rotated); err != nil {
		return "", fmt.Errorf("saving refreshed session: %w", err)
	}
	return rotated.AccessToken, nil
}

// needsRefresh reports whether a JWT's exp lies within refreshLeeway. A token
// without a readable exp (not a JWT) is used as stored: only the backend can
// judge it.
func needsRefresh(token string) bool {
	exp, ok := jwtExpiry(token)
	if !ok {
		return false
	}
	return time.Until(exp) < refreshLeeway
}

// jwtExpiry reads the exp claim of a JWT without verifying its signature.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp *float64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Exp == nil {
		return time.Time{}, false
	}
	return time.Unix(int64(*claims.Exp), 0), true
}
