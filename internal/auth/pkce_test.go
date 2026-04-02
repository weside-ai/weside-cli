package auth_test

import (
	"testing"

	"github.com/weside-ai/weside-cli/internal/auth"
)

func TestGenerateVerifier(t *testing.T) {
	v1, err := auth.GenerateVerifier()
	if err != nil {
		t.Fatalf("GenerateVerifier() error: %v", err)
	}
	if v1 == "" {
		t.Error("GenerateVerifier() returned empty string")
	}

	// Should be unique
	v2, _ := auth.GenerateVerifier()
	if v1 == v2 {
		t.Error("two calls to GenerateVerifier() returned same value")
	}
}

func TestGenerateChallenge(t *testing.T) {
	verifier := "test-verifier-value"
	challenge := auth.GenerateChallenge(verifier)

	if challenge == "" {
		t.Error("GenerateChallenge() returned empty string")
	}

	// Same input should produce same output
	challenge2 := auth.GenerateChallenge(verifier)
	if challenge != challenge2 {
		t.Error("GenerateChallenge() not deterministic")
	}

	// Different input should produce different output
	challenge3 := auth.GenerateChallenge("different-verifier")
	if challenge == challenge3 {
		t.Error("different verifiers produced same challenge")
	}
}

func TestAuthorizeURL(t *testing.T) {
	url := auth.AuthorizeURL("test-challenge", "http://localhost:12345/callback")

	if url == "" {
		t.Fatal("AuthorizeURL() returned empty string")
	}

	tests := []struct {
		name     string
		contains string
	}{
		{"has supabase host", "supabase.co"},
		{"has authorize path", "/auth/v1/authorize"},
		{"has response_type", "response_type=code"},
		{"has challenge", "code_challenge=test-challenge"},
		{"has S256 method", "code_challenge_method=S256"},
		{"has redirect_uri", "redirect_uri="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !contains(url, tt.contains) {
				t.Errorf("URL %q missing %q", url, tt.contains)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
