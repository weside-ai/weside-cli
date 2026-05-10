package auth_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestAuthorizeURL(t *testing.T) {
	const supabaseURL = "https://example.supabase.co"
	url := auth.AuthorizeURL(supabaseURL, "test-challenge", "http://localhost:12345/callback", "google")

	if url == "" {
		t.Fatal("AuthorizeURL() returned empty string")
	}

	tests := []struct {
		name     string
		contains string
	}{
		{"uses provided supabase host", "example.supabase.co"},
		{"has authorize path", "/auth/v1/authorize"},
		{"has challenge", "code_challenge=test-challenge"},
		{"has S256 method", "code_challenge_method=S256"},
		{"has redirect_to", "redirect_to="},
		{"has provider", "provider=google"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !contains(url, tt.contains) {
				t.Errorf("URL %q missing %q", url, tt.contains)
			}
		})
	}

	// Social login flow must NOT include client_id or response_type
	forbidden := []struct {
		name     string
		contains string
	}{
		{"no client_id", "client_id="},
		{"no response_type", "response_type="},
	}
	for _, tt := range forbidden {
		t.Run(tt.name, func(t *testing.T) {
			if contains(url, tt.contains) {
				t.Errorf("URL %q should NOT contain %q", url, tt.contains)
			}
		})
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

// TestExchangeCode_HappyPath stubs Supabase's PKCE token endpoint and asserts
// ExchangeCode posts to the resolved supabaseURL with the resolved anon-key.
// Catches regressions in the URL-composition path that was just refactored to
// take supabaseURL as a parameter.
func TestExchangeCode_HappyPath(t *testing.T) {
	const wantAnonKey = "test-anon-key"
	var (
		gotPath   string
		gotMethod string
		gotAPIKey string
		gotBody   map[string]string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAPIKey = r.Header.Get("apikey")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

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
	res, err := auth.ExchangeCode(srv.URL+"/", wantAnonKey, "auth-code-xyz", "verifier-abc")
	if err != nil {
		t.Fatalf("ExchangeCode error: %v", err)
	}

	if res.AccessToken != "stub-access" || res.RefreshToken != "stub-refresh" {
		t.Errorf("got tokens %+v", res)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if !strings.HasPrefix(gotPath, "/auth/v1/token") {
		t.Errorf("path = %s, want /auth/v1/token...", gotPath)
	}
	if !strings.Contains(gotPath, "grant_type=pkce") {
		t.Errorf("path = %s, want grant_type=pkce", gotPath)
	}
	if gotAPIKey != wantAnonKey {
		t.Errorf("apikey header = %q, want %q", gotAPIKey, wantAnonKey)
	}
	if gotBody["auth_code"] != "auth-code-xyz" || gotBody["code_verifier"] != "verifier-abc" {
		t.Errorf("body = %+v, want auth_code+code_verifier", gotBody)
	}
}

func TestExchangeCode_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid_grant"}`))
	}))
	defer srv.Close()

	if _, err := auth.ExchangeCode(srv.URL, "anon", "code", "verifier"); err == nil {
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
		gotBody   map[string]string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAPIKey = r.Header.Get("apikey")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"access_token": "fresh-access",
			"refresh_token": "fresh-refresh",
			"expires_in": 3600,
			"token_type": "Bearer"
		}`))
	}))
	defer srv.Close()

	res, err := auth.RefreshAccessToken(srv.URL+"/", wantAnonKey, "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken error: %v", err)
	}
	if res.AccessToken != "fresh-access" || res.RefreshToken != "fresh-refresh" {
		t.Errorf("got tokens %+v", res)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if !strings.HasPrefix(gotPath, "/auth/v1/token") {
		t.Errorf("path = %s, want /auth/v1/token...", gotPath)
	}
	if !strings.Contains(gotPath, "grant_type=refresh_token") {
		t.Errorf("path = %s, want grant_type=refresh_token", gotPath)
	}
	if gotAPIKey != wantAnonKey {
		t.Errorf("apikey header = %q, want %q", gotAPIKey, wantAnonKey)
	}
	if gotBody["refresh_token"] != "old-refresh-token" {
		t.Errorf("body = %+v, want refresh_token", gotBody)
	}
}

func TestRefreshAccessToken_Non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := auth.RefreshAccessToken(srv.URL, "anon", "expired"); err == nil {
		t.Error("RefreshAccessToken should error on non-2xx response")
	}
}
