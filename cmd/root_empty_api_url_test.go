package cmd

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The guard lives in Execute's exit path and in the PersistentPreRun walk, and
// it reads process-global state (flag Changed bit, env), so each case runs the
// real Execute in a child process of this test binary: that is the only way to
// see the exit code and keep the global flag state from leaking between cases.
const emptyAPIURLChildEnv = "WESIDE_TEST_EMPTY_API_URL_ARGS"

func TestEmptyAPIURLChild(t *testing.T) {
	raw, ok := os.LookupEnv(emptyAPIURLChildEnv)
	if !ok {
		t.Skip("child process helper")
	}
	os.Args = append([]string{"weside"}, strings.Split(raw, "\x1f")...)
	rootCmd.SetArgs(os.Args[1:])
	Execute()
	os.Exit(0)
}

func runEmptyAPIURLChild(t *testing.T, env []string, args ...string) (code int, output string) {
	t.Helper()
	c := exec.Command(os.Args[0], "-test.run=^TestEmptyAPIURLChild$") //nolint:gosec // re-exec of this test binary, fixed args
	// A fresh HOME: no stored login and no config file, so even a guard that
	// failed open could not reach a real account.
	c.Env = append([]string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH"), emptyAPIURLChildEnv + "=" + strings.Join(args, "\x1f")}, env...)
	out, err := c.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running child: %v", err)
	}
	return code, string(out)
}

func TestEmptyAPIURLIsRefused(t *testing.T) {
	const flagMsg = "--api-url is set but empty; refusing to fall back to the stored login"
	const envMsg = "WESIDE_API_URL is set but empty; refusing to fall back to the stored login"
	cases := []struct {
		name string
		env  []string
		args []string
		want string
	}{
		{"flag empty", nil, []string{"version", "--api-url", ""}, flagMsg},
		{"flag empty equals form", nil, []string{"--api-url=", "version"}, flagMsg},
		{"flag whitespace", nil, []string{"version", "--api-url", "  "}, flagMsg},
		{"env empty", []string{"WESIDE_API_URL="}, []string{"version"}, envMsg},
		{"env whitespace", []string{"WESIDE_API_URL= "}, []string{"version"}, envMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := runEmptyAPIURLChild(t, tc.env, tc.args...)
			if code == 0 {
				t.Errorf("exit code 0; want non-zero. output:\n%s", out)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output lacks %q; got:\n%s", tc.want, out)
			}
		})
	}
}

// The fallback stays for an absent flag, and an explicit flag wins over an
// empty env var.
func TestAbsentOrSetAPIURLStillRuns(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		args []string
	}{
		{"no flag no env", nil, []string{"version"}},
		{"flag set, env empty", []string{"WESIDE_API_URL="}, []string{"version", "--api-url", "http://localhost:8000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := runEmptyAPIURLChild(t, tc.env, tc.args...)
			if code != 0 {
				t.Errorf("exit code %d; want 0. output:\n%s", code, out)
			}
		})
	}
}
