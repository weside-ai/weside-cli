package cmd

import "testing"

func TestVersionCommand(t *testing.T) {
	rootCmd.SetArgs([]string{"version"})

	// Should not error
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version command failed: %v", err)
	}
}

func TestVersionCommandJSON(t *testing.T) {
	rootCmd.SetArgs([]string{"version", "--json"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("version --json failed: %v", err)
	}
}
