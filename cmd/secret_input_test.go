package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func TestSecretStdinCommands(t *testing.T) {
	for _, tc := range []struct {
		name    string
		command *cobra.Command
		flags   []string
		args    []string
		field   string
	}{
		{"test stored", providerByokTestCmd, []string{"--provider", "openai", "--model", "test"}, nil, ""},
		{"discover stored", providerByokDiscoverCmd, []string{"--provider", "openai"}, nil, ""},
		{"byok", providerByokCmd, []string{"--key-stdin"}, []string{"openai"}, "api_key"},
		{"byok argv", providerByokCmd, nil, []string{"openai", " secret-value "}, "api_key"},
		{"sandbox argv", sandboxSecretsPutCmd, []string{"--value", " secret-value "}, []string{"test-key"}, "value"},
		{"test argv", providerByokTestCmd, []string{"--provider", "openai", "--model", "test", "--key", " secret-value "}, nil, "api_key"},
		{"discover argv", providerByokDiscoverCmd, []string{"--provider", "openai", "--key", " secret-value "}, nil, "api_key"},
		{"sandbox", sandboxSecretsPutCmd, []string{"--value-stdin"}, []string{"test-key"}, "value"},
		{"test", providerByokTestCmd, []string{"--provider", "openai", "--model", "test", "--key-stdin"}, nil, "api_key"},
		{"discover", providerByokDiscoverCmd, []string{"--provider", "openai", "--key-stdin"}, nil, "api_key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer srv.Close()
			t.Setenv("WESIDE_TOKEN", "test-token")
			viper.Set("api_url", srv.URL)
			t.Cleanup(func() { viper.Set("api_url", "") })
			tc.command.SetIn(strings.NewReader(" secret-value \r\n"))
			t.Cleanup(func() {
				tc.command.SetIn(nil)
				for _, flag := range []string{"key-stdin", "value-stdin", "provider", "model", "key", "value"} {
					if f := tc.command.Flags().Lookup(flag); f != nil {
						_ = f.Value.Set(f.DefValue)
						f.Changed = false
					}
				}
			})
			if err := tc.command.ParseFlags(tc.flags); err != nil {
				t.Fatal(err)
			}
			if tc.command.Args != nil {
				if err := tc.command.Args(tc.command, tc.args); err != nil {
					t.Fatal(err)
				}
			}
			out := captureStdout(t, func() error { return tc.command.RunE(tc.command, tc.args) })
			if tc.field == "" {
				if _, present := body["api_key"]; present {
					t.Fatal("omitted key must reuse stored key")
				}
			} else if body[tc.field] != " secret-value " {
				t.Fatalf("secret not delivered exactly (except one line ending): %#v", body)
			}
			if strings.Contains(out, "secret-value") {
				t.Fatal("secret echoed")
			}
		})
	}
}

func TestSecretStdinRejectsConflictsAndEmptyInput(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		supplied bool
	}{
		{"conflicting argv", "stdin-secret", true},
		{"empty", "", false},
		{"empty line", "\r\n", false},
		{"oversized", strings.Repeat("x", 1024*1024+1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := &cobra.Command{}
			addSecretStdinFlag(command, "key-stdin")
			if err := command.Flags().Set("key-stdin", "true"); err != nil {
				t.Fatal(err)
			}
			command.SetIn(strings.NewReader(tc.input))
			if _, _, err := secretInput(command, "key-stdin", "argv-secret", tc.supplied); err == nil {
				t.Fatal("unsafe input combination accepted")
			} else if strings.Contains(err.Error(), "stdin-secret") || strings.Contains(err.Error(), "argv-secret") {
				t.Fatal("error echoed secret")
			}
		})
	}
}
