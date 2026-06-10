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

// PKCEResult contains the tokens from a successful PKCE flow.
type PKCEResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// CallbackServer handles the OAuth callback on localhost.
type CallbackServer struct {
	listener      net.Listener
	server        *http.Server
	redirectURI   string
	expectedState string
	codeCh        chan string
	errCh         chan error
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

// GenerateState creates a cryptographically random OAuth `state` value used to
// bind the authorization request to its callback (CSRF protection).
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// AuthorizeURL builds the Supabase OAuth 2.1 authorization-server URL (PKCE flow).
//
// Unlike the social-login path (`/auth/v1/authorize?provider=...`), this targets
// the OAuth 2.1 server (`/auth/v1/oauth/authorize`) with a registered client_id.
// Supabase redirects the user to the weside-hosted consent/login page where they
// pick their sign-in method — instead of jumping straight to a single provider.
// supabaseURL + clientID come from the resolved Config — never hardcoded here.
func AuthorizeURL(supabaseURL, clientID, challenge, redirectURI, state string) string {
	params := url.Values{
		"client_id":             {clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"openid email profile"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
	}
	return strings.TrimRight(supabaseURL, "/") + "/auth/v1/oauth/authorize?" + params.Encode()
}

// NewCallbackServer creates and starts a localhost HTTP server for OAuth
// callbacks. It tries each candidate port in order and binds the first that is
// free — every candidate must be a registered redirect_uri on the OAuth client,
// because the OAuth 2.1 server validates redirect_uri exactly (no DCR).
func NewCallbackServer(ports ...int) (*CallbackServer, error) {
	if len(ports) == 0 {
		return nil, fmt.Errorf("no callback port provided")
	}

	var lastErr error
	for _, port := range ports {
		listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			lastErr = err
			continue
		}

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

	return nil, fmt.Errorf("starting callback server on ports %v (is another login running?): %w", ports, lastErr)
}

// SetExpectedState binds the callback handler to an expected OAuth `state`.
// When set, handleCallback rejects callbacks whose state does not match.
func (cs *CallbackServer) SetExpectedState(state string) {
	cs.expectedState = state
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
	q := r.URL.Query()
	if cs.expectedState != "" && q.Get("state") != cs.expectedState {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><h2>Login failed</h2><p>State mismatch — the login response could not be verified. Please try again.</p><p>You can close this tab.</p></body></html>`)
		cs.errCh <- fmt.Errorf("auth callback: state mismatch (possible CSRF) — please retry login")
		return
	}

	code := q.Get("code")
	if code == "" {
		errMsg := q.Get("error_description")
		if errMsg == "" {
			errMsg = q.Get("error")
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

// oauthToken posts an OAuth 2.1 token request (form-encoded, RFC 6749) to the
// Supabase OAuth server's token endpoint and decodes the token response.
// supabaseURL + supabaseAnonKey come from the resolved Config.
func oauthToken(supabaseURL, supabaseAnonKey string, form url.Values, label string) (*PKCEResult, error) {
	endpoint := strings.TrimRight(supabaseURL, "/") + "/auth/v1/oauth/token"
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating %s request: %w", label, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("apikey", supabaseAnonKey)

	resp, err := supabaseHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var errResp map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("%s failed (%d): %v", label, resp.StatusCode, errResp)
	}

	var result PKCEResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parsing %s response: %w", label, err)
	}
	return &result, nil
}

// ExchangeCode exchanges an authorization code for tokens via the OAuth 2.1
// authorization_code grant + PKCE. clientID + redirectURI must match the values
// used in the authorization request (and the registered redirect_uri).
func ExchangeCode(supabaseURL, supabaseAnonKey, clientID, code, verifier, redirectURI string) (*PKCEResult, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"client_id":     {clientID},
		"redirect_uri":  {redirectURI},
	}
	return oauthToken(supabaseURL, supabaseAnonKey, form, "token exchange")
}

// RefreshAccessToken uses a refresh token to get a new access token via the
// OAuth 2.1 refresh_token grant.
func RefreshAccessToken(supabaseURL, supabaseAnonKey, clientID, refreshToken string) (*PKCEResult, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	result, err := oauthToken(supabaseURL, supabaseAnonKey, form, "token refresh")
	if err != nil {
		return nil, fmt.Errorf("%w — please re-login with: weside auth login", err)
	}
	return result, nil
}
