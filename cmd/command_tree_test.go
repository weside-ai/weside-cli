package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// WA-2955: two subcommands of `companions` were both called `identity`. Cobra
// resolves a name to the first registered match, so `identity show|set` were
// unreachable. These tests drive the real tree through Execute.

// AC 4: no two siblings anywhere in the tree may share a name or alias.
func TestCommandTreeHasNoDuplicateNames(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		seen := map[string]string{}
		for _, child := range c.Commands() {
			for _, name := range append([]string{child.Name()}, child.Aliases...) {
				if prev, dup := seen[name]; dup {
					t.Errorf("%q has two subcommands answering to %q: %q and %q",
						c.CommandPath(), name, prev, child.Use)
				}
				seen[name] = child.Use
			}
			walk(child)
		}
	}
	walk(rootCmd)
}

// runRootErr executes the real tree and returns its output and error. Global
// state the run touches is restored.
func runRootErr(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	prevOut, prevErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() {
		rootCmd.SetOut(prevOut)
		rootCmd.SetErr(prevErr)
		rootCmd.SetArgs(nil)
	})
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return out.String(), err
}

// runRoot is runRootErr for a run that must succeed.
func runRoot(t *testing.T, args ...string) string {
	t.Helper()
	out, err := runRootErr(t, args...)
	if err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
	return out
}

// AC 1: the help of the one `identity` command lists its subcommands.
func TestCompanionsIdentityHelpListsShowAndSet(t *testing.T) {
	out := runRoot(t, "companions", "identity", "--help")
	t.Cleanup(func() { _ = companionsIdentityCmd.Flags().Set("help", "false") })

	for _, want := range []string{"show", "set", "--companion"} {
		if !strings.Contains(out, want) {
			t.Errorf("`companions identity --help` lacks %q:\n%s", want, out)
		}
	}
}

type recordedRequest struct {
	method, path string
	body         map[string]any
}

// identityPromptServer answers the v1 identity-prompt endpoints and records
// every request.
func identityPromptServer(t *testing.T) *[]recordedRequest {
	t.Helper()
	var reqs []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{method: r.Method, path: r.URL.Path}
		if data, _ := io.ReadAll(r.Body); len(data) > 0 {
			_ = json.Unmarshal(data, &rec.body)
		}
		reqs = append(reqs, rec)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":3,"body":"I am Nox.","created_by":"owner","updated_at":"now"}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("WESIDE_TOKEN", "test-token")
	viper.Set("api_url", srv.URL)
	t.Cleanup(func() { viper.Set("api_url", "") })
	return &reqs
}

// AC 3: `identity show <c>` reaches its RunE.
func TestIdentityShowReachesItsRunE(t *testing.T) {
	reqs := identityPromptServer(t)

	runRoot(t, "companions", "identity", "show", "42")

	if len(*reqs) != 1 || (*reqs)[0].method != http.MethodGet ||
		(*reqs)[0].path != "/api/v1/companions/42/identity-prompt" {
		t.Fatalf("requests = %+v, want one GET /api/v1/companions/42/identity-prompt", *reqs)
	}
}

// AC 3: `identity set <c> --file -` reaches its RunE and sends stdin.
func TestIdentitySetFromStdinReachesItsRunE(t *testing.T) {
	reqs := identityPromptServer(t)
	prevStdin := stdinReader
	stdinReader = strings.NewReader("I am Nox.")
	t.Cleanup(func() {
		stdinReader = prevStdin
		identityFile, identityBaseVer = "", 0
	})

	runRoot(t, "companions", "identity", "set", "42", "--file", "-")

	if len(*reqs) != 1 || (*reqs)[0].method != http.MethodPut ||
		(*reqs)[0].path != "/api/v1/companions/42/identity-prompt" {
		t.Fatalf("requests = %+v, want one PUT /api/v1/companions/42/identity-prompt", *reqs)
	}
	if got := (*reqs)[0].body["new_identity"]; got != "I am Nox." {
		t.Fatalf("new_identity = %v, want the stdin content", got)
	}
}

// AC 2 through the tree: without a subcommand, `identity` still loads the full
// identity via MCP and honours --companion.
func TestCompanionsIdentityWithoutSubcommandStillLoadsViaMCP(t *testing.T) {
	pins := mcpPinRecorder(t)
	withDefaultCompanion(t, "Nox")
	t.Cleanup(func() { identityCompanion = "" })

	runRoot(t, "companions", "identity", "--companion", "Elise")

	if len(*pins) != 1 || (*pins)[0] != "Elise" {
		t.Fatalf("X-Weside-Companion = %q, want [\"Elise\"]", *pins)
	}
}

// A mistyped subcommand is an error, not a silent MCP identity load.
func TestCompanionsIdentityRejectsUnknownSubcommand(t *testing.T) {
	_, err := runRootErr(t, "companions", "identity", "shw", "42")
	if err == nil || !strings.Contains(err.Error(), `unknown command "shw"`) {
		t.Fatalf("err = %v, want unknown command \"shw\"", err)
	}
}
