package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	supabaseURL = "https://pqykrwpmhjqjhpsnjxbd.supabase.co"
	clientID    = "9114483b-1a59-460d-afa0-2534fd3bd1aa"
)

// PKCEResult contains the tokens from a successful PKCE flow.
type PKCEResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// CallbackServer handles the OAuth callback on localhost.
type CallbackServer struct {
	listener    net.Listener
	server      *http.Server
	redirectURI string
	codeCh      chan string
	errCh       chan error
}

// GenerateVerifier creates a cryptographically random PKCE code verifier.
func GenerateVerifier() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateChallenge creates the PKCE code challenge from a verifier.
func GenerateChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// AuthorizeURL builds the Supabase authorization URL.
func AuthorizeURL(challenge, redirectURI string) string {
	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return supabaseURL + "/auth/v1/authorize?" + params.Encode()
}

// NewCallbackServer creates and starts a localhost HTTP server for OAuth callbacks.
func NewCallbackServer() (*CallbackServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("starting callback server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	cs := &CallbackServer{
		listener:    listener,
		redirectURI: fmt.Sprintf("http://localhost:%d/callback", port),
		codeCh:      make(chan string, 1),
		errCh:       make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handleCallback)

	cs.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if serveErr := cs.server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			cs.errCh <- serveErr
		}
	}()

	return cs, nil
}

// RedirectURI returns the callback URL to use in the authorization request.
func (cs *CallbackServer) RedirectURI() string {
	return cs.redirectURI
}

// WaitForCode blocks until an authorization code is received or the context expires.
func (cs *CallbackServer) WaitForCode(ctx context.Context) (string, error) {
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cs.server.Shutdown(shutdownCtx)
	}()

	select {
	case code := <-cs.codeCh:
		return code, nil
	case err := <-cs.errCh:
		return "", err
	case <-ctx.Done():
		return "", fmt.Errorf("login timed out (2 minutes)")
	}
}

func (cs *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		if errMsg == "" {
			errMsg = "no authorization code received"
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(w, `<html><body><h2>Login failed</h2><p>%s</p><p>You can close this tab.</p></body></html>`, errMsg)
		cs.errCh <- fmt.Errorf("auth callback: %s", errMsg)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, `<html><body><h2>Login successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>`)
	cs.codeCh <- code
}

// ExchangeCode exchanges an authorization code for tokens via PKCE.
func ExchangeCode(code, verifier, redirectURI string) (*PKCEResult, error) {
	data := url.Values{
		"grant_type":    {"pkce"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	}

	resp, err := http.Post(
		supabaseURL+"/auth/v1/token?grant_type=pkce",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("token exchange failed (%d): %v", resp.StatusCode, errResp)
	}

	var result PKCEResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing token response: %w", err)
	}

	return &result, nil
}

// RefreshAccessToken uses a refresh token to get a new access token.
func RefreshAccessToken(refreshToken string) (*PKCEResult, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.Post(
		supabaseURL+"/auth/v1/token?grant_type=refresh_token",
		"application/x-www-form-urlencoded",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return nil, fmt.Errorf("token refresh: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed (%d) — please re-login with: weside auth login", resp.StatusCode)
	}

	var result PKCEResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing refresh response: %w", err)
	}

	return &result, nil
}
