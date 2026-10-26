package auth_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/weside-ai/weside-cli/internal/auth"
)

// fakeJWT builds an unsigned JWT whose payload carries exp — enough for
// GetToken, which reads exp without verifying the signature.
func fakeJWT(t *testing.T, exp time.Time, nonce string) string {
	t.Helper()
	enc := base64.RawURLEncoding
	header := enc.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload, err := json.Marshal(map[string]any{"exp": exp.Unix(), "sub": "user-1", "n": nonce})
	if err != nil {
		t.Fatal(err)
	}
	return header + "." + enc.EncodeToString(payload) + ".sig"
}

// fakeTokenServer is a Supabase OAuth token endpoint with refresh-token
// rotation and reuse detection: each refresh token is valid exactly once, a
// replayed one answers invalid_grant (what burns a chain at Supabase).
type fakeTokenServer struct {
	t        *testing.T
	srv      *httptest.Server
	mu       sync.Mutex
	valid    string
	issued   int
	requests atomic.Int32
	delay    time.Duration
	fail     bool
}

func newFakeTokenServer(t *testing.T, validRefresh string) *fakeTokenServer {
	t.Helper()
	f := &fakeTokenServer{t: t, valid: validRefresh, delay: 300 * time.Millisecond}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeTokenServer) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/auth/v1/oauth/token" {
		http.NotFound(w, r)
		return
	}
	f.requests.Add(1)
	time.Sleep(f.delay)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.fail || r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != f.valid {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"Invalid Refresh Token"}`))
		return
	}
	f.issued++
	f.valid = fmt.Sprintf("rotated-refresh-%d", f.issued)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  fakeJWT(f.t, time.Now().Add(time.Hour), fmt.Sprint(f.issued)),
		"refresh_token": f.valid,
		"expires_in":    3600,
		"token_type":    "bearer",
	})
}

// setupHome points HOME at a temp dir, writes credentials.json there and
// routes auth-config resolution to the fake server via the override.
func setupHome(t *testing.T, supabaseURL string, tokens auth.Tokens) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("WESIDE_TOKEN", "")
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("supabase_url", supabaseURL)
	viper.Set("supabase_anon_key", "test-anon-key")
	if err := auth.NewStorage().Save(&tokens); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(home, ".weside", "credentials.json")
}

func readTokens(t *testing.T, path string) auth.Tokens {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var tok auth.Tokens
	if err := json.Unmarshal(data, &tok); err != nil {
		t.Fatal(err)
	}
	return tok
}

func expOf(t *testing.T, token string) time.Time {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	return time.Unix(claims.Exp, 0)
}

func TestGetTokenRefreshesExpiredToken(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	fake.delay = 0
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	path := setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})

	token, err := auth.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token == expired || !expOf(t, token).After(time.Now()) {
		t.Fatalf("GetToken() returned a token that is not fresh")
	}
	if n := fake.requests.Load(); n != 1 {
		t.Fatalf("refresh requests = %d, want 1", n)
	}
	stored := readTokens(t, path)
	if stored.AccessToken != token || stored.RefreshToken != "rotated-refresh-1" {
		t.Fatalf("stored pair not rotated (refresh=%q)", stored.RefreshToken)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %v (err %v), want 0600", info.Mode().Perm(), err)
	}
}

func TestGetTokenRefreshesWithinLeeway(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	fake.delay = 0
	soon := fakeJWT(t, time.Now().Add(30*time.Second), "soon")
	setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: soon, RefreshToken: "refresh-0"})

	token, err := auth.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token == soon || fake.requests.Load() != 1 {
		t.Fatalf("token expiring in 30 s was not refreshed (requests=%d)", fake.requests.Load())
	}
}

func TestGetTokenFreshTokenNoRefresh(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	fresh := fakeJWT(t, time.Now().Add(time.Hour), "fresh")
	setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: fresh, RefreshToken: "refresh-0"})

	token, err := auth.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != fresh || fake.requests.Load() != 0 {
		t.Fatalf("fresh token changed or refreshed (requests=%d)", fake.requests.Load())
	}
}

func TestGetTokenEnvShortCircuitsRefresh(t *testing.T) {
	const envValue = "env-value-unchanged"
	fake := newFakeTokenServer(t, "refresh-0")
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})
	t.Setenv("WESIDE_TOKEN", envValue)

	token, err := auth.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != envValue || fake.requests.Load() != 0 {
		t.Fatalf("WESIDE_TOKEN not returned unchanged (requests=%d)", fake.requests.Load())
	}
}

func TestGetTokenRefreshFailureLeavesFileUntouched(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	fake.delay = 0
	fake.fail = true
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	path := setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	token, err := auth.GetToken()
	if err == nil {
		t.Fatalf("GetToken() expected an error on refresh failure, got a token")
	}
	if token != "" {
		t.Fatalf("GetToken() returned a token alongside its error")
	}
	if !strings.Contains(err.Error(), "weside auth login") {
		t.Fatalf("error does not ask for re-login: %v", err)
	}
	if strings.Contains(err.Error(), "refresh-0") || strings.Contains(err.Error(), expired) {
		t.Fatalf("error message carries a token")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("credentials file changed after a failed refresh")
	}
}

func TestGetTokenExpiredWithoutRefreshToken(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired})

	if _, err := auth.GetToken(); err == nil || !strings.Contains(err.Error(), "weside auth login") {
		t.Fatalf("GetToken() = %v, want a re-login error", err)
	}
	if fake.requests.Load() != 0 {
		t.Fatalf("refresh sent without a refresh token")
	}
}

// AC 7: concurrent callers on one credentials file send exactly one refresh.
// Without the lock both would send refresh-0; the fake answers the second with
// invalid_grant, as Supabase's reuse detection would.
func TestGetTokenConcurrentGoroutinesRefreshOnce(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	path := setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})

	const n = 4
	var wg sync.WaitGroup
	tokens := make([]string, n)
	errs := make([]error, n)
	start := make(chan struct{})
	for i := range n {
		wg.Go(func() {
			<-start
			tokens[i], errs[i] = auth.GetToken()
		})
	}
	close(start)
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		if !expOf(t, tokens[i]).After(time.Now()) {
			t.Fatalf("caller %d got a stale token", i)
		}
	}
	if got := fake.requests.Load(); got != 1 {
		t.Fatalf("refresh requests = %d, want exactly 1", got)
	}
	if stored := readTokens(t, path); stored.RefreshToken != "rotated-refresh-1" {
		t.Fatalf("stored refresh token = %q, want rotated-refresh-1", stored.RefreshToken)
	}
}

// helperEnv selects the subprocess mode of TestHelperGetTokenProcess.
const helperEnv = "WESIDE_TEST_GETTOKEN_HELPER"

// TestHelperGetTokenProcess is not a test on its own: the two-process test
// re-executes the test binary into it, so each process holds its own lock fd.
func TestHelperGetTokenProcess(t *testing.T) {
	if os.Getenv(helperEnv) != "1" {
		t.Skip("subprocess helper")
	}
	viper.Set("supabase_url", os.Getenv("HELPER_SUPABASE_URL"))
	viper.Set("supabase_anon_key", "test-anon-key")
	token, err := auth.GetToken()
	if err != nil {
		fmt.Fprintln(os.Stderr, "helper error:", err)
		os.Exit(3)
	}
	fmt.Print("TOKEN=" + token)
	os.Exit(0)
}

func TestGetTokenTwoProcessesRefreshOnce(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	path := setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})
	home := filepath.Dir(filepath.Dir(path))

	type result struct {
		out []byte
		err error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		cmd := exec.Command(os.Args[0], //nolint:gosec // G702: re-executes this test binary
			"-test.run=^TestHelperGetTokenProcess$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			helperEnv+"=1",
			"HOME="+home,
			"USERPROFILE="+home,
			"WESIDE_TOKEN=",
			"HELPER_SUPABASE_URL="+fake.srv.URL,
		)
		wg.Go(func() {
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil {
				err = fmt.Errorf("%w: %s", err, stderr.String())
			}
			results[i] = result{out, err}
		})
	}
	wg.Wait()

	for i, r := range results {
		if r.err != nil {
			t.Fatalf("process %d: %v", i, r.err)
		}
		idx := bytes.Index(r.out, []byte("TOKEN="))
		if idx < 0 {
			t.Fatalf("process %d printed no token", i)
		}
		tok := strings.Fields(string(r.out[idx+len("TOKEN="):]))[0]
		if !expOf(t, tok).After(time.Now()) {
			t.Fatalf("process %d got a stale token", i)
		}
	}
	if got := fake.requests.Load(); got != 1 {
		t.Fatalf("refresh requests across two processes = %d, want exactly 1", got)
	}
	if stored := readTokens(t, path); stored.RefreshToken != "rotated-refresh-1" {
		t.Fatalf("stored refresh token = %q, want rotated-refresh-1", stored.RefreshToken)
	}
}

func TestGetTokenPartialOverrideRefusesRefresh(t *testing.T) {
	fake := newFakeTokenServer(t, "refresh-0")
	expired := fakeJWT(t, time.Now().Add(-time.Minute), "old")
	setupHome(t, fake.srv.URL, auth.Tokens{AccessToken: expired, RefreshToken: "refresh-0"})
	viper.Set("supabase_anon_key", "")

	if _, err := auth.GetToken(); !errors.Is(err, auth.ErrPartialOverride) {
		t.Fatalf("GetToken() = %v, want ErrPartialOverride", err)
	}
	if fake.requests.Load() != 0 {
		t.Fatalf("refresh sent against a half-overridden auth config")
	}
}
