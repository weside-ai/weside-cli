package auth_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/weside-ai/weside-cli/internal/auth"
)

func TestGenerateVerifier(t *testing.T) {
	v1, err := auth.GenerateVerifier()
	if err != nil {
		t.Fatalf("GenerateVerifier() error: %v", err)
	}
	if v1 == "" {
		t.Error("GenerateVerifier() returned empty string")
	}

	// Should be unique
	v2, _ := auth.GenerateVerifier()
	if v1 == v2 {
		t.Error("two calls to GenerateVerifier() returned same value")
	}
}

func TestGenerateChallenge(t *testing.T) {
	verifier := "test-verifier-value"
	challenge := auth.GenerateChallenge(verifier)

	if challenge == "" {
		t.Error("GenerateChallenge() returned empty string")
	}

	// Same input should produce same output
	challenge2 := auth.GenerateChallenge(verifier)
	if challenge != challenge2 {
		t.Error("GenerateChallenge() not deterministic")
	}

	// Different input should produce different output
	challenge3 := auth.GenerateChallenge("different-verifier")
	if challenge == challenge3 {
		t.Error("different verifiers produced same challenge")
	}
}

func TestGenerateState(t *testing.T) {
	s1, err := auth.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}
	if s1 == "" {
		t.Error("GenerateState() returned empty string")
	}
	s2, _ := auth.GenerateState()
	if s1 == s2 {
		t.Error("two calls to GenerateState() returned same value")
	}
}

func TestAuthorizeURL(t *testing.T) {
	const supabaseURL = "https://example.supabase.co"
	url := auth.AuthorizeURL(supabaseURL, "cli-client-id", "test-challenge", "http://localhost:18520/callback", "state-xyz")

	if url == "" {
		t.Fatal("AuthorizeURL() returned empty string")
	}

	// OAuth 2.1 authorization-server flow (not the social /authorize path):
	// must carry client_id + response_type=code and target /oauth/authorize.
	tests := []struct {
		name     string
		contains string
	}{
		{"uses provided supabase host", "example.supabase.co"},
		{"has oauth authorize path", "/auth/v1/oauth/authorize"},
		{"has client_id", "client_id=cli-client-id"},
		{"has response_type code", "response_type=code"},
		{"has redirect_uri", "redirect_uri="},
		{"has challenge", "code_challenge=test-challenge"},
		{"has S256 method", "code_challenge_method=S256"},
		{"has state", "state=state-xyz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !contains(url, tt.contains) {
				t.Errorf("URL %q missing %q", url, tt.contains)
			}
		})
	}

	// Must NOT use the social-login provider param (the whole point of the
	// change — provider choice happens on the weside login page).
	if contains(url, "provider=") {
		t.Errorf("URL %q should NOT contain provider= (social-login path)", url)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestExchangeCode_HappyPath stubs the Supabase OAuth 2.1 token endpoint and
// asserts ExchangeCode posts the authorization_code grant (form-encoded) to the
// resolved supabaseURL with the resolved anon-key, client_id and redirect_uri.
func TestExchangeCode_HappyPath(t *testing.T) {
	const wantAnonKey = "test-anon-key"
	var (
		gotPath   string
		gotMethod string
		gotAPIKey string
		gotCT     string
		gotForm   map[string]string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("apikey")
		gotCT = r.Header.Get("Content-Type")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		_ = r.ParseForm()
		gotForm = map[string]string{}
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "stub-access",
			"refresh_token": "stub-refresh",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer srv.Close()

	// Pass srv.URL with a trailing slash to verify TrimRight in the implementation.
	res, err := auth.ExchangeCode(srv.URL+"/", wantAnonKey, "cli-client-id", "auth-code-xyz", "verifier-abc", "http://localhost:18520/callback")
	if err != nil {
		t.Fatalf("ExchangeCode error: %v", err)
	}

	if res.AccessToken != "stub-access" || res.RefreshToken != "stub-refresh" {
		t.Errorf("got tokens %+v", res)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/auth/v1/oauth/token" {
		t.Errorf("path = %s, want /auth/v1/oauth/token", gotPath)
	}
	if gotCT != "application/x-www-form-urlencoded" {
		t.Errorf("content-type = %q, want form-urlencoded", gotCT)
	}
	if gotAPIKey != wantAnonKey {
		t.Errorf("apikey header = %q, want %q", gotAPIKey, wantAnonKey)
	}
	if gotForm["grant_type"] != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", gotForm["grant_type"])
	}
	if gotForm["code"] != "auth-code-xyz" || gotForm["code_verifier"] != "verifier-abc" {
		t.Errorf("form = %+v, want code+code_verifier", gotForm)
	}
	if gotForm["client_id"] != "cli-client-id" || gotForm["redirect_uri"] != "http://localhost:18520/callback" {
		t.Errorf("form = %+v, want client_id+redirect_uri", gotForm)
	}
}

func TestExchangeCode_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	defer srv.Close()

	if _, err := auth.ExchangeCode(srv.URL, "anon", "client", "code", "verifier", "http://localhost:18520/callback"); err == nil {
		t.Error("ExchangeCode should error on non-2xx response")
	}
}

// TestRefreshAccessToken_HappyPath covers the deferred AC-6 path: the
// function has no callers today (auto-refresh on 401 is a follow-up), but
// its signature was just changed to take dynamic supabaseURL + anon-key —
// we want a regression net so a future AC-6 implementer doesn't trip on a
// silently-broken URL composition or apikey header.
func TestRefreshAccessToken_HappyPath(t *testing.T) {
	const wantAnonKey = "test-anon-key"
	var (
		gotPath   string
		gotMethod string
		gotAPIKey string
		gotForm   map[string]string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("apikey")
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		_ = r.ParseForm()
		gotForm = map[string]string{}
		for k := range r.PostForm {
			gotForm[k] = r.PostForm.Get(k)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "fresh-access",
			"refresh_token": "fresh-refresh",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer srv.Close()

	res, err := auth.RefreshAccessToken(srv.URL+"/", wantAnonKey, "cli-client-id", "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken error: %v", err)
	}
	if res.AccessToken != "fresh-access" || res.RefreshToken != "fresh-refresh" {
		t.Errorf("got tokens %+v", res)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/auth/v1/oauth/token" {
		t.Errorf("path = %s, want /auth/v1/oauth/token", gotPath)
	}
	if gotForm["grant_type"] != "refresh_token" {
		t.Errorf("grant_type = %q, want refresh_token", gotForm["grant_type"])
	}
	if gotAPIKey != wantAnonKey {
		t.Errorf("apikey header = %q, want %q", gotAPIKey, wantAnonKey)
	}
	if gotForm["refresh_token"] != "old-refresh-token" || gotForm["client_id"] != "cli-client-id" {
		t.Errorf("form = %+v, want refresh_token+client_id", gotForm)
	}
}

func TestRefreshAccessToken_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := auth.RefreshAccessToken(srv.URL, "anon", "client", "expired"); err == nil {
		t.Error("RefreshAccessToken should error on non-2xx response")
	}
}

// TestCallbackServer_StateMismatchRejected exercises the CSRF guard: a callback
// whose `state` does not match the expected value must surface an error from
// WaitForCode rather than yielding a code.
func TestCallbackServer_StateMismatchRejected(t *testing.T) {
	srv, err := auth.NewCallbackServer(18546)
	if err != nil {
		t.Skipf("port 18546 unavailable: %v", err)
	}
	srv.SetExpectedState("good-state")

	go func() {
		resp, gerr := http.Get(srv.RedirectURI() + "?code=abc&state=evil")
		if gerr == nil {
			_ = resp.Body.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := srv.WaitForCode(ctx); err == nil {
		t.Fatal("WaitForCode should reject a state mismatch")
	}
}

// TestCallbackServer_ValidStateReturnsCode is the happy-path counterpart: a
// matching state yields the authorization code.
func TestCallbackServer_ValidStateReturnsCode(t *testing.T) {
	srv, err := auth.NewCallbackServer(18547)
	if err != nil {
		t.Skipf("port 18547 unavailable: %v", err)
	}
	srv.SetExpectedState("good-state")

	go func() {
		resp, gerr := http.Get(srv.RedirectURI() + "?code=the-code&state=good-state")
		if gerr == nil {
			_ = resp.Body.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, err := srv.WaitForCode(ctx)
	if err != nil {
		t.Fatalf("WaitForCode error: %v", err)
	}
	if code != "the-code" {
		t.Errorf("code = %q, want the-code", code)
	}
}

// TestNewCallbackServer_PortFallback verifies the multi-port retry: when the
// primary port is occupied, the server binds the next candidate and reports the
// fallback port in RedirectURI (which must match a registered redirect_uri).
func TestNewCallbackServer_PortFallback(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:18548")
	if err != nil {
		t.Skipf("port 18548 unavailable: %v", err)
	}
	defer func() { _ = l.Close() }()

	srv, err := auth.NewCallbackServer(18548, 18549)
	if err != nil {
		t.Fatalf("NewCallbackServer error: %v", err)
	}
	if !strings.HasSuffix(srv.RedirectURI(), ":18549/callback") {
		t.Errorf("RedirectURI = %q, want fallback port 18549", srv.RedirectURI())
	}

	// Let the fallback server shut down cleanly (WaitForCode shuts it down).
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _ = srv.WaitForCode(ctx)
}
