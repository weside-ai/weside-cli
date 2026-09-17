package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// executeRoot drives the REAL command tree through cobra's Execute, because the
// behaviour under test lives in `ExecuteC` and in the PersistentPreRun walk —
// calling a RunE directly skips both and would pass no matter what root says.
//
// rootCmd is a package global and the hook MUTATES the command it runs (that is
// how it works), so every field this touches is restored: a leaked
// `SilenceUsage: true` would leave the second test passing on the first test's
// side effect, which is the shape of a gate that cannot fail.
func executeRoot(t *testing.T, args ...string) string {
	t.Helper()

	var out bytes.Buffer
	prevOut, prevErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)

	child, _, err := rootCmd.Find(args)
	if err != nil {
		t.Fatalf("finding %v: %v", args, err)
	}
	prevSilence := child.SilenceUsage

	t.Cleanup(func() {
		child.SilenceUsage = prevSilence
		rootCmd.SetOut(prevOut)
		rootCmd.SetErr(prevErr)
		rootCmd.SetArgs(nil)
	})

	rootCmd.SetArgs(args)
	if err := rootCmd.Execute(); err == nil {
		t.Fatalf("%v succeeded; this test needs it to fail", args)
	}
	return out.String()
}

// A wrong argument COUNT is what usage is for: cobra validates args before any
// PersistentPreRun runs, so the block survives the hook.
func TestArgumentErrorStillPrintsUsage(t *testing.T) {
	out := executeRoot(t, "api", "GET") // ExactArgs(2), one given

	if !strings.Contains(out, "Usage:") {
		t.Errorf("an argument error printed no usage block; got:\n%s", out)
	}
}

// An error raised from RunE onwards is about the world, not the syntax. The
// driver needs no network and no credentials: `api` rejects an unknown method
// itself, three lines into RunE.
func TestRuntimeErrorPrintsNoUsageBlock(t *testing.T) {
	out := executeRoot(t, "api", "FETCH", "/rooms")

	if strings.Contains(out, "Usage:") {
		t.Errorf("a runtime error printed the usage block; got:\n%s", out)
	}
	if strings.Contains(out, "--supabase-anon-key") {
		t.Errorf("a runtime error printed the global flag table; got:\n%s", out)
	}
}
