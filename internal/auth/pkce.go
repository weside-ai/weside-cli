package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var supabaseHTTPClient = &http.Client{Timeout: 30 * time.Second}

const (
	supabaseURL     = "https://pqykrwpmhjqjhpsnjxbd.supabase.co"
	supabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InBxeWtyd3BtaGpxamhwc25qeGJkIiwicm9sZSI6ImFub24iLCJpYXQiOjE3Njk5ODU3NDksImV4cCI6MjA4NTU2MTc0OX0.ADx_HD7O-xNMx-j4MDrhaJbRO71R-hJO6yTcf5wFWUA"
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

// AuthorizeURL builds the Supabase social login authorization URL (PKCE flow).
func AuthorizeURL(challenge, redirectTo, provider string) string {
	params := url.Values{
		"provider":              {provider},
		"redirect_to":           {redirectTo},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return supabaseURL + "/auth/v1/authorize?" + params.Encode()
}

const callbackPort = 18520

// NewCallbackServer creates and starts a localhost HTTP server for OAuth callbacks.
// Uses a fixed port so it can be whitelisted in Supabase redirect URLs.
func NewCallbackServer() (*CallbackServer, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return nil, fmt.Errorf("starting callback server on port %d (is another login running?): %w", callbackPort, err)
	}

	port := callbackPort

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
		_, _ = fmt.Fprintf(w, `<html><body><h2>Login failed</h2><p>%s</p><p>You can close this tab.</p></body></html>`, html.EscapeString(errMsg))
		cs.errCh <- fmt.Errorf("auth callback: %s", errMsg)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	_, _ = fmt.Fprint(w, `<html><body><h2>Login successful!</h2><p>You can close this tab and return to the terminal.</p></body></html>`)
	cs.codeCh <- code
}

// ExchangeCode exchanges an authorization code for tokens via PKCE.
func ExchangeCode(code, verifier string) (*PKCEResult, error) {
	data := url.Values{
		"auth_code":     {code},
		"code_verifier": {verifier},
	}

	req, err := http.NewRequest("POST", supabaseURL+"/auth/v1/token?grant_type=pkce",
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("apikey", supabaseAnonKey)

	resp, err := supabaseHTTPClient.Do(req)
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

	req, err := http.NewRequest("POST", supabaseURL+"/auth/v1/token?grant_type=refresh_token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("apikey", supabaseAnonKey)

	resp, err := supabaseHTTPClient.Do(req)
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
