// Package config manages CLI configuration files.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// EnsureConfigDir creates ~/.weside/ if it doesn't exist.
func EnsureConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home dir: %w", err)
	}

	dir := filepath.Join(home, ".weside")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating config dir: %w", err)
	}

	return dir, nil
}

// GetDefaultCompanion returns the configured default companion name.
func GetDefaultCompanion() string {
	return viper.GetString("default_companion")
}

// SetDefaultCompanion saves the default companion (name + id) to disk and viper.
// Pass an empty id to leave default_companion_id untouched.
func SetDefaultCompanion(name, id string) error {
	updates := map[string]any{"default_companion": name}
	if id != "" {
		updates["default_companion_id"] = id
	}
	if err := PersistUpdates(updates); err != nil {
		return err
	}
	viper.Set("default_companion", name)
	if id != "" {
		viper.Set("default_companion_id", id)
	}
	return nil
}

// configFilePath returns the absolute path to ~/.weside/config.yaml.
func configFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home dir: %w", err)
	}
	return filepath.Join(home, ".weside", "config.yaml"), nil
}

// transientFlagKeys are top-level viper keys that come from CLI flags or env
// vars and must never be persisted. Older versions of the CLI used
// viper.WriteConfigAs which serialized them by accident; PersistUpdates
// strips them so legacy config files self-heal on the next write.
var transientFlagKeys = []string{
	"json",
	"verbose",
	"no_color",
	"supabase_url",
	"supabase_anon_key",
}

// loadConfigYAML reads ~/.weside/config.yaml as a YAML map.
// Returns an empty map if the file doesn't exist.
func loadConfigYAML() (map[string]any, error) {
	path, err := configFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// PersistUpdates merges `updates` into ~/.weside/config.yaml without touching
// unrelated keys. Nested map[string]any values are deep-merged so partial
// updates to a sub-block (e.g. just one auth.* field) preserve siblings.
//
// This is the only sanctioned way to write the config file. It deliberately
// avoids viper.WriteConfigAs, which would otherwise persist every flag-bound
// key that happened to be set on the current invocation (--api-url, --json,
// --verbose, …) and silently clobber values the user actually configured.
//
// Legacy flag-only keys already on disk (json, verbose, no_color, supabase_url,
// supabase_anon_key) are stripped on every write so old config files self-heal.
func PersistUpdates(updates map[string]any) error {
	existing, err := loadConfigYAML()
	if err != nil {
		return err
	}
	for _, k := range transientFlagKeys {
		delete(existing, k)
	}
	deepMerge(existing, updates)

	path, err := configFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming config: %w", err)
	}
	return nil
}

// PersistDottedKey turns "auth.supabase_url" + value into a nested map
// {"auth": {"supabase_url": value}} and writes it via PersistUpdates.
// Used by `weside config set <key> <value>` so a flag override on the
// same invocation does not leak into the persisted file.
func PersistDottedKey(dottedKey string, value any) error {
	parts := strings.Split(dottedKey, ".")
	root := map[string]any{}
	cur := root
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = value
			break
		}
		next := map[string]any{}
		cur[p] = next
		cur = next
	}
	return PersistUpdates(root)
}

// deepMerge recursively writes src into dst.
// If both src[k] and dst[k] are maps, recurses; otherwise dst[k] = src[k].
func deepMerge(dst, src map[string]any) {
	for k, sv := range src {
		if dv, ok := dst[k]; ok {
			if dvMap, dvOk := dv.(map[string]any); dvOk {
				if svMap, svOk := sv.(map[string]any); svOk {
					deepMerge(dvMap, svMap)
					continue
				}
			}
		}
		dst[k] = sv
	}
}
