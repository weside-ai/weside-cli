package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
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
	FetchedAt       string `json:"fetched_at,omitempty" mapstructure:"fetched_at,omitempty"`
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
// the caller can surface it under --verbose.
func Resolve(ctx context.Context, apiURL string) ResolveResult {
	if cfg := overrideConfig(); cfg != nil {
		return ResolveResult{Config: cfg, Source: SourceOverride}
	}
	if cfg, ok := loadCachedAuth(); ok {
		return ResolveResult{Config: cfg, Source: SourceCache}
	}
	cfg, err := Fetch(ctx, apiURL)
	if err == nil {
		_ = saveCachedAuth(cfg)
		return ResolveResult{Config: cfg, Source: SourceLive}
	}
	return ResolveResult{Config: defaultConfig(), Source: SourceFallback, FetchError: err}
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

	var cfg Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parsing well-known response: %w", err)
	}
	if cfg.SupabaseURL == "" || cfg.SupabaseAnonKey == "" || cfg.CallbackPort == 0 || cfg.MCPURL == "" {
		return nil, errors.New("well-known response is missing required fields")
	}
	cfg.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	return &cfg, nil
}

func overrideConfig() *Config {
	url := strings.TrimSpace(viper.GetString("supabase_url"))
	key := strings.TrimSpace(viper.GetString("supabase_anon_key"))
	if url == "" && key == "" {
		return nil
	}
	if url == "" {
		url = defaultSupabaseURL
	}
	if key == "" {
		key = defaultSupabaseAnonKey
	}
	return &Config{
		SupabaseURL:     url,
		SupabaseAnonKey: key,
		CallbackPort:    defaultCallbackPort,
		MCPURL:          defaultMCPURL,
	}
}

// LoadCachedAuth returns the cached auth-config from ~/.weside/config.yaml
// (via viper). The bool reports whether all required fields were present.
func LoadCachedAuth() (*Config, bool) {
	return loadCachedAuth()
}

// SaveCachedAuth persists the given config to ~/.weside/config.yaml under the
// `auth.*` block. Sets FetchedAt to now (UTC, RFC3339) if empty.
func SaveCachedAuth(cfg *Config) error {
	return saveCachedAuth(cfg)
}

func loadCachedAuth() (*Config, bool) {
	url := strings.TrimSpace(viper.GetString("auth.supabase_url"))
	key := strings.TrimSpace(viper.GetString("auth.supabase_anon_key"))
	port := viper.GetInt("auth.callback_port")
	mcp := strings.TrimSpace(viper.GetString("auth.mcp_url"))
	if url == "" || key == "" || port == 0 || mcp == "" {
		return nil, false
	}
	return &Config{
		SupabaseURL:     url,
		SupabaseAnonKey: key,
		CallbackPort:    port,
		MCPURL:          mcp,
		FetchedAt:       viper.GetString("auth.fetched_at"),
	}, true
}

func saveCachedAuth(cfg *Config) error {
	viper.Set("auth.supabase_url", cfg.SupabaseURL)
	viper.Set("auth.supabase_anon_key", cfg.SupabaseAnonKey)
	viper.Set("auth.callback_port", cfg.CallbackPort)
	viper.Set("auth.mcp_url", cfg.MCPURL)
	if cfg.FetchedAt == "" {
		cfg.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	viper.Set("auth.fetched_at", cfg.FetchedAt)
	return writeConfig()
}

// writeConfig persists the current viper state to ~/.weside/config.yaml.
// Defined as a var so tests can no-op the disk write without touching the user's home dir.
var writeConfig = func() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding home dir: %w", err)
	}
	dir := filepath.Join(home, ".weside")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	if err := viper.WriteConfigAs(filepath.Join(dir, "config.yaml")); err != nil {
		return fmt.Errorf("saving auth cache: %w", err)
	}
	return nil
}

func defaultConfig() *Config {
	return &Config{
		SupabaseURL:     defaultSupabaseURL,
		SupabaseAnonKey: defaultSupabaseAnonKey,
		CallbackPort:    defaultCallbackPort,
		MCPURL:          defaultMCPURL,
	}
}
