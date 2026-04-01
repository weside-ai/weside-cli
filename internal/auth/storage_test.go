package auth_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/weside-ai/weside-cli/internal/auth"
)

func TestStorageSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	storage := &auth.Storage{}
	storage.SetFilePath(filepath.Join(dir, "creds.json"))

	tokens := &auth.Tokens{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
	}

	if err := storage.Save(tokens); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if loaded.AccessToken != tokens.AccessToken {
		t.Errorf("AccessToken = %q, want %q", loaded.AccessToken, tokens.AccessToken)
	}
	if loaded.RefreshToken != tokens.RefreshToken {
		t.Errorf("RefreshToken = %q, want %q", loaded.RefreshToken, tokens.RefreshToken)
	}
}

func TestStorageLoadNotLoggedIn(t *testing.T) {
	dir := t.TempDir()
	storage := &auth.Storage{}
	storage.SetFilePath(filepath.Join(dir, "nonexistent.json"))

	_, err := storage.Load()
	if err == nil {
		t.Fatal("Load() expected error for missing file, got nil")
	}
}

func TestStorageDelete(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "creds.json")
	storage := &auth.Storage{}
	storage.SetFilePath(fp)

	// Save then delete
	_ = storage.Save(&auth.Tokens{AccessToken: "x"})
	if err := storage.Delete(); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("file should not exist after Delete()")
	}
}

func TestStorageDeleteNonExistent(t *testing.T) {
	dir := t.TempDir()
	storage := &auth.Storage{}
	storage.SetFilePath(filepath.Join(dir, "nope.json"))

	// Should not error
	if err := storage.Delete(); err != nil {
		t.Errorf("Delete() on nonexistent file: %v", err)
	}
}

func TestGetTokenFromEnv(t *testing.T) {
	want := "test-env-value-abc"
	t.Setenv("WESIDE_TOKEN", want)

	token, err := auth.GetToken()
	if err != nil {
		t.Fatalf("GetToken() error: %v", err)
	}
	if token != want {
		t.Errorf("GetToken() = %q, want %q", token, want)
	}
}

func TestLoadEmptyAccessToken(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "creds.json")
	storage := &auth.Storage{}
	storage.SetFilePath(fp)

	// Save empty token
	_ = storage.Save(&auth.Tokens{AccessToken: ""})

	_, err := storage.Load()
	if err == nil {
		t.Fatal("Load() expected error for empty access token, got nil")
	}
}
