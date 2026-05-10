// Package config manages CLI configuration files.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	"github.com/weside-ai/weside-cli/internal/config"
)

// withIsolatedHome redirects $HOME to a temp dir so config writes don't touch
// the user's real ~/.weside/config.yaml. Returns the temp config file path.
func withIsolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Cleanup(viper.Reset)
	viper.Reset()
	return filepath.Join(home, ".weside", "config.yaml")
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return out
}

func writeYAMLMap(t *testing.T, path string, m map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	withIsolatedHome(t)
	dir, err := config.EnsureConfigDir()
	if err != nil {
		t.Fatalf("EnsureConfigDir() error: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("config dir missing: %v", err)
	}
	if filepath.Base(dir) != ".weside" {
		t.Errorf("config dir = %q, want .weside", dir)
	}
}

func TestPersistUpdates_CreatesFile(t *testing.T) {
	path := withIsolatedHome(t)

	if err := config.PersistUpdates(map[string]any{"api_url": "https://example.test"}); err != nil {
		t.Fatalf("PersistUpdates: %v", err)
	}
	got := readYAMLMap(t, path)
	if got["api_url"] != "https://example.test" {
		t.Errorf("api_url = %v", got["api_url"])
	}
}

// TestPersistUpdates_PreservesSiblings is the core of the WA-1037 fix:
// SaveCachedAuth (or refresh-auth) must not clobber api_url / default_companion
// just because the caller passed a flag override on this invocation.
func TestPersistUpdates_PreservesSiblings(t *testing.T) {
	path := withIsolatedHome(t)
	writeYAMLMap(t, path, map[string]any{
		"api_url":           "http://localhost:8000",
		"default_companion": "TestCompanion",
	})

	if err := config.PersistUpdates(map[string]any{
		"auth": map[string]any{
			"supabase_url":      "https://stub.supabase.co",
			"supabase_anon_key": "stub-key",
		},
	}); err != nil {
		t.Fatalf("PersistUpdates: %v", err)
	}

	got := readYAMLMap(t, path)
	if got["api_url"] != "http://localhost:8000" {
		t.Errorf("api_url clobbered: got %v", got["api_url"])
	}
	if got["default_companion"] != "TestCompanion" {
		t.Errorf("default_companion clobbered: got %v", got["default_companion"])
	}
	auth, ok := got["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth block missing or wrong type: %v", got["auth"])
	}
	if auth["supabase_url"] != "https://stub.supabase.co" {
		t.Errorf("auth.supabase_url = %v", auth["supabase_url"])
	}
}

func TestPersistUpdates_DeepMergesNested(t *testing.T) {
	path := withIsolatedHome(t)
	writeYAMLMap(t, path, map[string]any{
		"auth": map[string]any{
			"supabase_url":  "https://existing.supabase.co",
			"callback_port": 18520,
		},
	})

	// Update only the anon-key — supabase_url + callback_port must survive.
	if err := config.PersistUpdates(map[string]any{
		"auth": map[string]any{
			"supabase_anon_key": "new-key",
		},
	}); err != nil {
		t.Fatalf("PersistUpdates: %v", err)
	}

	auth := readYAMLMap(t, path)["auth"].(map[string]any)
	if auth["supabase_url"] != "https://existing.supabase.co" {
		t.Errorf("auth.supabase_url clobbered: %v", auth["supabase_url"])
	}
	if auth["callback_port"] != 18520 {
		t.Errorf("auth.callback_port clobbered: %v", auth["callback_port"])
	}
	if auth["supabase_anon_key"] != "new-key" {
		t.Errorf("auth.supabase_anon_key not updated: %v", auth["supabase_anon_key"])
	}
}

// TestPersistUpdates_StripsLegacyFlagKeys covers the self-healing behavior:
// older CLI versions (≤ v0.5.0) used viper.WriteConfigAs which serialized
// flag-bound keys (json, verbose, supabase_url, …). On the next write,
// PersistUpdates removes them so the config file converges to a clean state.
func TestPersistUpdates_StripsLegacyFlagKeys(t *testing.T) {
	path := withIsolatedHome(t)
	writeYAMLMap(t, path, map[string]any{
		"api_url":           "http://localhost:8000",
		"json":              false,
		"verbose":           false,
		"no_color":          false,
		"supabase_url":      "",
		"supabase_anon_key": "",
	})

	if err := config.PersistUpdates(map[string]any{"default_companion": "Foo"}); err != nil {
		t.Fatalf("PersistUpdates: %v", err)
	}

	got := readYAMLMap(t, path)
	for _, k := range []string{"json", "verbose", "no_color", "supabase_url", "supabase_anon_key"} {
		if _, exists := got[k]; exists {
			t.Errorf("legacy key %q not stripped on rewrite", k)
		}
	}
	if got["api_url"] != "http://localhost:8000" {
		t.Errorf("api_url clobbered while stripping: %v", got["api_url"])
	}
	if got["default_companion"] != "Foo" {
		t.Errorf("default_companion = %v", got["default_companion"])
	}
}

func TestPersistDottedKey(t *testing.T) {
	path := withIsolatedHome(t)
	writeYAMLMap(t, path, map[string]any{"api_url": "http://localhost:8000"})

	if err := config.PersistDottedKey("auth.supabase_url", "https://x.supabase.co"); err != nil {
		t.Fatalf("PersistDottedKey: %v", err)
	}

	got := readYAMLMap(t, path)
	if got["api_url"] != "http://localhost:8000" {
		t.Errorf("api_url clobbered: %v", got["api_url"])
	}
	auth := got["auth"].(map[string]any)
	if auth["supabase_url"] != "https://x.supabase.co" {
		t.Errorf("auth.supabase_url = %v", auth["supabase_url"])
	}
}

func TestSetDefaultCompanion_PersistsBoth(t *testing.T) {
	path := withIsolatedHome(t)

	if err := config.SetDefaultCompanion("Nox", "53"); err != nil {
		t.Fatalf("SetDefaultCompanion: %v", err)
	}

	got := readYAMLMap(t, path)
	if got["default_companion"] != "Nox" {
		t.Errorf("default_companion = %v", got["default_companion"])
	}
	if got["default_companion_id"] != "53" {
		t.Errorf("default_companion_id = %v", got["default_companion_id"])
	}
	// Mirrored into viper for the rest of the process.
	if viper.GetString("default_companion") != "Nox" {
		t.Errorf("viper default_companion = %v", viper.GetString("default_companion"))
	}
}

func TestSetDefaultCompanion_EmptyIDLeavesItAlone(t *testing.T) {
	path := withIsolatedHome(t)
	writeYAMLMap(t, path, map[string]any{"default_companion_id": "old-id"})

	if err := config.SetDefaultCompanion("Nox", ""); err != nil {
		t.Fatalf("SetDefaultCompanion: %v", err)
	}

	got := readYAMLMap(t, path)
	if got["default_companion"] != "Nox" {
		t.Errorf("default_companion = %v", got["default_companion"])
	}
	if got["default_companion_id"] != "old-id" {
		t.Errorf("default_companion_id should be untouched, got %v", got["default_companion_id"])
	}
}
