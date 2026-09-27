package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/weside-ai/weside-cli/internal/auth"
)

// TestFinishLogin_StoresTheVerifiedAAL2Tokens drives PKCE exchange → factor
// lookup → prompt → challenge + verify and asserts the stored tokens are the
// ones verify returned, not the aal1 exchange tokens.
func TestFinishLogin_StoresTheVerifiedAAL2Tokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/auth/v1/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"aal1-access","refresh_token":"aal1-refresh","expires_in":3600,"token_type":"bearer"}`))
		case "/auth/v1/user":
			_, _ = w.Write([]byte(`{"factors":[{"id":"f1","factor_type":"totp","status":"verified"}]}`))
		case "/auth/v1/factors/f1/challenge":
			_, _ = w.Write([]byte(`{"id":"ch1"}`))
		case "/auth/v1/factors/f1/verify":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Header.Get("Authorization") != "Bearer aal1-access" || body["challenge_id"] != "ch1" || body["code"] != "654321" {
				w.WriteHeader(http.StatusUnprocessableEntity)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"aal2-access","refresh_token":"aal2-refresh","expires_in":3600,"token_type":"bearer"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	storage := auth.NewStorage()
	storage.SetFilePath(filepath.Join(t.TempDir(), "credentials.json"))
	cfg := &auth.Config{SupabaseURL: srv.URL, SupabaseAnonKey: "anon", OAuthClientID: "cli"}

	err := finishLogin(cfg, "code", "verifier", "http://localhost:18520/callback",
		func(int) (string, error) { return "654321", nil }, storage)
	if err != nil {
		t.Fatalf("finishLogin: %v", err)
	}
	got, err := storage.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AccessToken != "aal2-access" || got.RefreshToken != "aal2-refresh" {
		t.Errorf("stored %q/%q, want the aal2 verify tokens", got.AccessToken, got.RefreshToken)
	}
}

// TestFinishLogin_WrongCodeTwiceStoresNothing: a failed code step leaves no
// aal1 session behind that the backend would refuse anyway.
func TestFinishLogin_WrongCodeTwiceStoresNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/v1/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"aal1-access","refresh_token":"aal1-refresh"}`))
		case "/auth/v1/user":
			_, _ = w.Write([]byte(`{"factors":[{"id":"f1","factor_type":"totp","status":"verified"}]}`))
		case "/auth/v1/factors/f1/challenge":
			_, _ = w.Write([]byte(`{"id":"ch1"}`))
		default:
			w.WriteHeader(http.StatusUnprocessableEntity)
		}
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "credentials.json")
	storage := auth.NewStorage()
	storage.SetFilePath(path)
	cfg := &auth.Config{SupabaseURL: srv.URL, SupabaseAnonKey: "anon", OAuthClientID: "cli"}

	if err := finishLogin(cfg, "code", "verifier", "http://localhost:18520/callback",
		func(int) (string, error) { return "000000", nil }, storage); err == nil {
		t.Fatal("want an error after two wrong codes")
	}
	if _, err := storage.Load(); err == nil {
		t.Error("tokens were stored after a failed code step")
	}
}
