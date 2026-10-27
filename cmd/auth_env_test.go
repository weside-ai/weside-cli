package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/weside-ai/weside-cli/internal/auth"
)

// envHome points HOME at a temp dir with a prod login at the path every
// release before per-environment storage wrote it, and returns that path and
// its bytes.
func envHome(t *testing.T) (prodPath string, prod []byte) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WESIDE_TOKEN", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
	prodPath = filepath.Join(home, ".weside", "credentials.json")
	if err := os.MkdirAll(filepath.Dir(prodPath), 0o700); err != nil {
		t.Fatal(err)
	}
	prod = []byte(`{"access_token": "prod-access", "refresh_token": "prod-refresh"}`)
	if err := os.WriteFile(prodPath, prod, 0o600); err != nil {
		t.Fatal(err)
	}
	return prodPath, prod
}

// devBackend serves /dev/auth/token like a local backend.
func devBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/dev/auth/token" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "dev-access"})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A dev login against a local backend leaves the prod login byte-identical,
// and prod's token stays the one GetToken returns for prod.
func TestDevLoginLeavesProdCredentialsUntouched(t *testing.T) {
	prodPath, prodBefore := envHome(t)
	srv := devBackend(t)

	viper.Set("api_url", srv.URL)
	if err := loginDev(); err != nil {
		t.Fatalf("loginDev: %v", err)
	}

	after, err := os.ReadFile(prodPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, prodBefore) {
		t.Fatalf("prod credentials changed by a dev login:\n%s", after)
	}

	devToken, err := auth.GetToken()
	if err != nil || devToken != "dev-access" {
		t.Fatalf("GetToken for dev = %q, %v; want dev-access", devToken, err)
	}

	viper.Set("api_url", "")
	prodToken, err := auth.GetToken()
	if err != nil || prodToken != "prod-access" {
		t.Fatalf("GetToken for prod = %q, %v; want prod-access", prodToken, err)
	}
}

// Logging out of dev keeps the prod login.
func TestDevLogoutKeepsProdLogin(t *testing.T) {
	prodPath, prodBefore := envHome(t)
	srv := devBackend(t)
	viper.Set("api_url", srv.URL)
	if err := loginDev(); err != nil {
		t.Fatalf("loginDev: %v", err)
	}
	if err := authLogoutCmd.RunE(authLogoutCmd, nil); err != nil {
		t.Fatalf("logout: %v", err)
	}
	after, err := os.ReadFile(prodPath)
	if err != nil {
		t.Fatalf("prod credentials gone after a dev logout: %v", err)
	}
	if !bytes.Equal(after, prodBefore) {
		t.Fatal("prod credentials changed by a dev logout")
	}
}

// A dev login aimed at production is refused before anything is stored.
func TestDevLoginRefusesProduction(t *testing.T) {
	prodPath, prodBefore := envHome(t)
	if err := loginDev(); err == nil {
		t.Fatal("dev login against production succeeded")
	}
	after, err := os.ReadFile(prodPath)
	if err != nil || !bytes.Equal(after, prodBefore) {
		t.Fatal("prod credentials changed by a refused dev login")
	}
}
