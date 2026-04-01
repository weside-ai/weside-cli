// Package config manages CLI configuration files.
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/weside-ai/weside-cli/internal/config"
)

func TestEnsureConfigDir(t *testing.T) {
	dir, err := config.EnsureConfigDir()
	if err != nil {
		t.Fatalf("EnsureConfigDir() error: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("config dir does not exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("config dir is not a directory")
	}

	// Should end with .weside
	if filepath.Base(dir) != ".weside" {
		t.Errorf("config dir = %q, want to end with .weside", dir)
	}
}
