// Package config manages CLI configuration files.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
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

// SetDefaultCompanion saves the default companion name.
func SetDefaultCompanion(name string) error {
	viper.Set("default_companion", name)
	return writeConfig()
}

func writeConfig() error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.yaml")
	return viper.WriteConfigAs(configPath)
}
