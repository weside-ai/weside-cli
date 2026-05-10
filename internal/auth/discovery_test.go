package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"

	"github.com/weside-ai/weside-cli/internal/auth"
)

// resetAuthState clears every viper key auth.Resolve might consult and stubs
// the cache writer so tests don't touch the real ~/.weside/config.yaml.
func resetAuthState(t *testing.T) {
	t.Helper()
	viper.Reset()
	auth.SetCacheWriterForTests(func() error { return nil })
	t.Cleanup(func() {
		viper.Reset()
		auth.ResetCacheWriterForTests()
	})
}

func goodWellKnownHandler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/weside-auth" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"version": 1,
			"supabase_url": "https://stub.supabase.co",
			"supabase_anon_key": "stub-anon-key",
			"callback_port": 19999,
			"mcp_url": "https://api.example.test/mcp/"
		}`))
	}
}

func TestResolve_LiveFetch(t *testing.T) {
	resetAuthState(t)
	srv := httptest.NewServer(goodWellKnownHandler(t))
	defer srv.Close()

	res := auth.Resolve(context.Background(), srv.URL)
	if res.Source != auth.SourceLive {
		t.Fatalf("source = %q, want %q (err=%v)", res.Source, auth.SourceLive, res.FetchError)
	}
	if res.Config.SupabaseURL != "https://stub.supabase.co" {
		t.Errorf("SupabaseURL = %q", res.Config.SupabaseURL)
	}
	if res.Config.CallbackPort != 19999 {
		t.Errorf("CallbackPort = %d", res.Config.CallbackPort)
	}
	if res.Config.MCPURL != "https://api.example.test/mcp/" {
		t.Errorf("MCPURL = %q", res.Config.MCPURL)
	}
	if res.Config.FetchedAt == "" {
		t.Error("FetchedAt should be set after live fetch")
	}
	// Live fetch must populate the in-memory viper cache.
	if got := viper.GetString("auth.supabase_url"); got != "https://stub.supabase.co" {
		t.Errorf("cache supabase_url = %q after live fetch", got)
	}
}

func TestResolve_MalformedJSONFallsBack(t *testing.T) {
	resetAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer srv.Close()

	res := auth.Resolve(context.Background(), srv.URL)
	if res.Source != auth.SourceFallback {
		t.Fatalf("source = %q, want %q", res.Source, auth.SourceFallback)
	}
	if res.FetchError == nil {
		t.Error("FetchError should be set on malformed JSON")
	}
	if res.Config == nil || res.Config.SupabaseURL == "" {
		t.Error("fallback config should be non-empty")
	}
}

func TestResolve_NetworkErrorFallsBack(t *testing.T) {
	resetAuthState(t)
	// Port 1 is reserved + closed in practice → connect refused fast.
	res := auth.Resolve(context.Background(), "http://127.0.0.1:1")
	if res.Source != auth.SourceFallback {
		t.Fatalf("source = %q, want %q", res.Source, auth.SourceFallback)
	}
	if res.FetchError == nil {
		t.Error("FetchError should be set on network failure")
	}
}

func TestResolve_PartialResponseFallsBack(t *testing.T) {
	resetAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Missing mcp_url → must fall back per validation.
		_, _ = w.Write([]byte(`{
			"version": 1,
			"supabase_url": "https://stub.supabase.co",
			"supabase_anon_key": "stub",
			"callback_port": 18520
		}`))
	}))
	defer srv.Close()

	res := auth.Resolve(context.Background(), srv.URL)
	if res.Source != auth.SourceFallback {
		t.Fatalf("source = %q, want %q (err=%v)", res.Source, auth.SourceFallback, res.FetchError)
	}
}

func TestResolve_CacheHitShortCircuitsFetch(t *testing.T) {
	resetAuthState(t)

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		goodWellKnownHandler(t).ServeHTTP(w, r)
	}))
	defer srv.Close()

	viper.Set("auth.supabase_url", "https://cached.supabase.co")
	viper.Set("auth.supabase_anon_key", "cached-key")
	viper.Set("auth.callback_port", 28520)
	viper.Set("auth.mcp_url", "https://cached.example/mcp/")
	viper.Set("auth.fetched_at", time.Now().UTC().Format(time.RFC3339))

	res := auth.Resolve(context.Background(), srv.URL)
	if res.Source != auth.SourceCache {
		t.Fatalf("source = %q, want %q", res.Source, auth.SourceCache)
	}
	if res.Config.SupabaseURL != "https://cached.supabase.co" {
		t.Errorf("SupabaseURL = %q", res.Config.SupabaseURL)
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("well-known endpoint hit %d times — cache should short-circuit fetch", got)
	}
}

func TestResolve_OverrideBeatsCache(t *testing.T) {
	resetAuthState(t)

	viper.Set("auth.supabase_url", "https://cached.supabase.co")
	viper.Set("auth.supabase_anon_key", "cached-key")
	viper.Set("auth.callback_port", 28520)
	viper.Set("auth.mcp_url", "https://cached.example/mcp/")

	viper.Set("supabase_url", "https://override.supabase.co")
	viper.Set("supabase_anon_key", "override-key")

	res := auth.Resolve(context.Background(), "http://127.0.0.1:1")
	if res.Source != auth.SourceOverride {
		t.Fatalf("source = %q, want %q", res.Source, auth.SourceOverride)
	}
	if res.Config.SupabaseURL != "https://override.supabase.co" {
		t.Errorf("SupabaseURL = %q", res.Config.SupabaseURL)
	}
	if res.Config.SupabaseAnonKey != "override-key" {
		t.Errorf("SupabaseAnonKey = %q", res.Config.SupabaseAnonKey)
	}
	// Override path uses default callback port + mcp URL (not overridable).
	if res.Config.CallbackPort != 18520 {
		t.Errorf("CallbackPort = %d, want default 18520", res.Config.CallbackPort)
	}
}

func TestResolve_PartialOverrideFallsBackWithDiagnosis(t *testing.T) {
	resetAuthState(t)
	// Only URL set — anon-key missing. Must NOT mix with default anon-key.
	viper.Set("supabase_url", "https://my-selfhosted.example")

	res := auth.Resolve(context.Background(), "http://127.0.0.1:1")
	if res.Source != auth.SourceFallback {
		t.Fatalf("source = %q, want %q (partial override must not silently mix defaults)", res.Source, auth.SourceFallback)
	}
	if res.FetchError == nil {
		t.Fatal("FetchError should describe the partial-override mistake")
	}
	if !strings.Contains(res.FetchError.Error(), "supabase-url") || !strings.Contains(res.FetchError.Error(), "supabase-anon-key") {
		t.Errorf("FetchError = %v, want to mention both flags", res.FetchError)
	}
	// cmd/auth.go uses errors.Is to decide whether to print the unconditional
	// stderr warning — make sure the sentinel matches.
	if !errors.Is(res.FetchError, auth.ErrPartialOverride) {
		t.Errorf("FetchError = %v, want errors.Is to match auth.ErrPartialOverride", res.FetchError)
	}
}

func TestFetch_EmptyAPIURL(t *testing.T) {
	resetAuthState(t)
	if _, err := auth.Fetch(context.Background(), ""); err == nil {
		t.Error("Fetch with empty apiURL should error")
	}
}

func TestFetch_Non2xx(t *testing.T) {
	resetAuthState(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	if _, err := auth.Fetch(context.Background(), srv.URL); err == nil {
		t.Error("Fetch should error on non-2xx response")
	}
}
