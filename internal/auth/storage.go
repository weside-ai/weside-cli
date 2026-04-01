// Package auth handles authentication token storage and retrieval.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Tokens holds the stored authentication tokens.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// Storage handles token persistence.
type Storage struct {
	filePath string
}

// NewStorage creates a new token storage.
func NewStorage() *Storage {
	home, _ := os.UserHomeDir()
	return &Storage{
		filePath: filepath.Join(home, ".weside", "credentials.json"),
	}
}

// SetFilePath overrides the storage file path (for testing).
func (s *Storage) SetFilePath(path string) {
	s.filePath = path
}

// Save stores tokens to the filesystem.
func (s *Storage) Save(tokens *Tokens) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tokens: %w", err)
	}

	if err := os.WriteFile(s.filePath, data, 0o600); err != nil {
		return fmt.Errorf("writing tokens: %w", err)
	}

	return nil
}

// Load retrieves stored tokens.
func (s *Storage) Load() (*Tokens, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("not logged in (run: weside auth login)")
		}
		return nil, fmt.Errorf("reading tokens: %w", err)
	}

	var tokens Tokens
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("parsing tokens: %w", err)
	}

	if tokens.AccessToken == "" {
		return nil, fmt.Errorf("no access token found (run: weside auth login)")
	}

	return &tokens, nil
}

// Delete removes stored tokens.
func (s *Storage) Delete() error {
	if err := os.Remove(s.filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("removing tokens: %w", err)
	}
	return nil
}

// GetToken returns the current access token or an error if not logged in.
func GetToken() (string, error) {
	// Check env var first (CI/headless)
	if token := os.Getenv("WESIDE_TOKEN"); token != "" {
		return token, nil
	}

	storage := NewStorage()
	tokens, err := storage.Load()
	if err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}
