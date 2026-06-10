package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/weside-ai/weside-cli/internal/config"
)

// Config holds the backend-derived auth/discovery values used during PKCE login.
//
// Source of truth at runtime is the resolver (Resolve / Fetch); the hardcoded
// constants in this file are last-resort fallbacks for offline first-runs.
type Config struct {
	SupabaseURL     string `json:"supabase_url"     mapstructure:"supabase_url"`
	SupabaseAnonKey string `json:"supabase_anon_key" mapstructure:"supabase_anon_key"`
	CallbackPort    int    `json:"callback_port"    mapstructure:"callback_port"`
	MCPURL          string `json:"mcp_url"          mapstructure:"mcp_url"`
	// OAuthClientID is the registered public/PKCE OAuth client the CLI uses for
	// the OAuth 2.1 login flow. Optional in the well-known response (older
	// backends omit it) — callers fall back to defaultOAuthClientID.
	OAuthClientID string `json:"oauth_client_id,omitempty" mapstructure:"oauth_client_id,omitempty"`
	FetchedAt     string `json:"fetched_at,omitempty" mapstructure:"fetched_at,omitempty"`
}

// ResolveSource identifies which precedence level produced a Config.
type ResolveSource string

// Source labels for ResolveResult — see Resolve for the precedence chain.
const (
	SourceOverride ResolveSource = "override"
	SourceCache    ResolveSource = "cache"
	SourceLive     ResolveSource = "live"
	SourceFallback ResolveSource = "fallback"
)

// ResolveResult bundles the resolved config with provenance metadata.
type ResolveResult struct {
	Config     *Config
	Source     ResolveSource
	FetchError error
}

const (
	defaultSupabaseURL     = "https://pqykrwpmhjqjhpsnjxbd.supabase.co"
	defaultSupabaseAnonKey = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6InBxeWtyd3BtaGpxamhwc25qeGJkIiwicm9sZSI6ImFub24iLCJpYXQiOjE3Njk5ODU3NDksImV4cCI6MjA4NTU2MTc0OX0.ADx_HD7O-xNMx-j4MDrhaJbRO71R-hJO6yTcf5wFWUA"
	defaultCallbackPort    = 18520
	defaultMCPURL          = "https://api.weside.ai/mcp/"
	// defaultOAuthClientID is the public PKCE OAuth client registered for the
	// CLI in the Supabase dashboard (project pqykrwpmhjqjhpsnjxbd, OAuth Server).
	// Public by spec — a PKCE client has no client_secret, so this value is not
	// sensitive (mirrors how supabase_url/anon_key are already shipped here).
	// Registered redirect_uris: http://localhost:18520-18522/callback.
	defaultOAuthClientID = "91aa6153-6a70-4e91-8c7c-bd89de775ad8"
)

var discoveryHTTPClient = &http.Client{Timeout: 5 * time.Second}

// Resolve picks a Config using a fixed precedence chain:
//  1. Override — `--supabase-url`/`--supabase-anon-key` flags or
//     `WESIDE_SUPABASE_URL` / `WESIDE_SUPABASE_ANON_KEY` env vars.
//  2. Cache  — `auth.*` block in ~/.weside/config.yaml (must be complete).
//  3. Live   — single GET to `<apiURL>/.well-known/weside-auth` (5s timeout).
//     On success the result is written back to the cache.
//  4. Fallback — hardcoded defaults in this file.
//
// Resolve never returns nil — Source==SourceFallback indicates that the live
// fetch was attempted and failed; FetchError carries the underlying error so
// the caller can surface it under --verbose. A partial override (only one of
// supabase_url / supabase_anon_key set) is reported via FetchError on the
// fallback result so the caller can show the user a precise diagnosis.
func Resolve(ctx context.Context, apiURL string) ResolveResult {
	cfg, err := overrideConfig()
	if err != nil {
		return ResolveResult{Config: defaultConfig(), Source: SourceFallback, FetchError: err}
	}
	if cfg != nil {
		return ResolveResult{Config: cfg, Source: SourceOverride}
	}
	if cached, ok := loadCachedAuth(); ok {
		warnIfCacheStale(cached)
		return ResolveResult{Config: cached, Source: SourceCache}
	}
	live, fetchErr := Fetch(ctx, apiURL)
	if fetchErr == nil {
		if saveErr := SaveCachedAuth(live); saveErr != nil {
			fmt.Fprintf(os.Stderr, "auth-config: warning: could not persist cache: %v\n", saveErr)
		}
		return ResolveResult{Config: live, Source: SourceLive}
	}
	return ResolveResult{Config: defaultConfig(), Source: SourceFallback, FetchError: fetchErr}
}

// cacheStalenessThreshold defines when loadCachedAuth surfaces a hint that
// the cached auth-config is old enough to be worth re-fetching. Pure UX
// nudge — Resolve never invalidates the cache itself; that stays the user's
// call via `weside config refresh-auth`.
const cacheStalenessThreshold = 30 * 24 * time.Hour

// warnIfCacheStale prints a one-line stderr hint when the cached auth-config
// is older than cacheStalenessThreshold (or when fetched_at is unparseable).
func warnIfCacheStale(cfg *Config) {
	if cfg.FetchedAt == "" {
		return
	}
	ts, err := time.Parse(time.RFC3339, cfg.FetchedAt)
	if err != nil {
		return
	}
	if age := time.Since(ts); age > cacheStalenessThreshold {
		days := int(age / (24 * time.Hour))
		fmt.Fprintf(os.Stderr, "auth-config: cache is %d days old — run `weside config refresh-auth` if login fails\n", days)
	}
}

// Fetch performs a single live GET against `<apiURL>/.well-known/weside-auth`.
// Returns an error on transport failure, non-2xx response, malformed JSON, or
// missing required fields. Used by Resolve and by `weside config refresh-auth`.
func Fetch(ctx context.Context, apiURL string) (*Config, error) {
	apiURL = strings.TrimSpace(apiURL)
	if apiURL == "" {
		return nil, errors.New("api URL is empty")
	}
	endpoint := strings.TrimRight(apiURL, "/") + "/.well-known/weside-auth"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building well-known request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := discoveryHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("well-known returned status %d", resp.StatusCode)
	}

	// Cap response body to 64 KiB — a misconfigured/malicious well-known endpoint
	// must not be able to OOM the CLI by streaming a multi-MB JSON payload.
	const maxWellKnownBodyBytes = 64 * 1024
	var cfg Config
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxWellKnownBodyBytes)).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing well-known response: %w", err)
	}
	if cfg.SupabaseURL == "" || cfg.SupabaseAnonKey == "" || cfg.CallbackPort == 0 || cfg.MCPURL == "" {
		return nil, errors.New("well-known response is missing required fields")
	}
	// oauth_client_id is optional — older backends omit it. Fall back to the
	// hardcoded default so login keeps working against them.
	if cfg.OAuthClientID == "" {
		cfg.OAuthClientID = defaultOAuthClientID
	}
	cfg.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return &cfg, nil
}

// ErrPartialOverride is returned by overrideConfig (via Resolve.FetchError)
// when only one of supabase_url / supabase_anon_key is set. Exported so
// cmd/auth.go can errors.Is-match against it and print an unconditional
// warning to stderr — partial overrides always indicate a misconfiguration
// and must not silently mix a user-supplied URL with the prod-default key.
var ErrPartialOverride = errors.New("--supabase-url and --supabase-anon-key (or WESIDE_SUPABASE_URL/WESIDE_SUPABASE_ANON_KEY) must be set together")

func overrideConfig() (*Config, error) {
	url := strings.TrimSpace(viper.GetString("supabase_url"))
	key := strings.TrimSpace(viper.GetString("supabase_anon_key"))
	switch {
	case url == "" && key == "":
		return nil, nil
	case url == "" || key == "":
		return nil, ErrPartialOverride
	}
	return &Config{
		SupabaseURL:     url,
		SupabaseAnonKey: key,
		CallbackPort:    defaultCallbackPort,
		MCPURL:          defaultMCPURL,
		OAuthClientID:   defaultOAuthClientID,
	}, nil
}

func loadCachedAuth() (*Config, bool) {
	url := strings.TrimSpace(viper.GetString("auth.supabase_url"))
	key := strings.TrimSpace(viper.GetString("auth.supabase_anon_key"))
	port := viper.GetInt("auth.callback_port")
	mcp := strings.TrimSpace(viper.GetString("auth.mcp_url"))
	if url == "" || key == "" || port == 0 || mcp == "" {
		return nil, false
	}
	clientID := strings.TrimSpace(viper.GetString("auth.oauth_client_id"))
	if clientID == "" {
		clientID = defaultOAuthClientID
	}
	return &Config{
		SupabaseURL:     url,
		SupabaseAnonKey: key,
		CallbackPort:    port,
		MCPURL:          mcp,
		OAuthClientID:   clientID,
		FetchedAt:       viper.GetString("auth.fetched_at"),
	}, true
}

// SaveCachedAuth persists cfg to ~/.weside/config.yaml under the `auth.*` block.
// Sets FetchedAt to now (UTC, RFC3339) if empty. Used by Resolve on a successful
// live fetch and by `weside config refresh-auth`.
//
// Routes through config.PersistUpdates rather than viper.WriteConfigAs so that
// flag values from the current invocation (--api-url, --supabase-url, …) are
// not silently persisted alongside the auth cache.
func SaveCachedAuth(cfg *Config) error {
	if cfg.FetchedAt == "" {
		cfg.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	authBlock := map[string]any{
		"supabase_url":      cfg.SupabaseURL,
		"supabase_anon_key": cfg.SupabaseAnonKey,
		"callback_port":     cfg.CallbackPort,
		"mcp_url":           cfg.MCPURL,
		"oauth_client_id":   cfg.OAuthClientID,
		"fetched_at":        cfg.FetchedAt,
	}
	if err := config.PersistUpdates(map[string]any{"auth": authBlock}); err != nil {
		return err
	}
	// Mirror into viper so the rest of the current process sees the new cache
	// without needing a re-read of the file.
	viper.Set("auth.supabase_url", cfg.SupabaseURL)
	viper.Set("auth.supabase_anon_key", cfg.SupabaseAnonKey)
	viper.Set("auth.callback_port", cfg.CallbackPort)
	viper.Set("auth.mcp_url", cfg.MCPURL)
	viper.Set("auth.oauth_client_id", cfg.OAuthClientID)
	viper.Set("auth.fetched_at", cfg.FetchedAt)
	return nil
}

func defaultConfig() *Config {
	return &Config{
		SupabaseURL:     defaultSupabaseURL,
		SupabaseAnonKey: defaultSupabaseAnonKey,
		CallbackPort:    defaultCallbackPort,
		MCPURL:          defaultMCPURL,
		OAuthClientID:   defaultOAuthClientID,
	}
}
