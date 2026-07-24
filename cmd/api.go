package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/weside-ai/weside-cli/internal/api"
)

var (
	apiV2   bool
	apiJSON bool
	apiBody string
)

var apiCmd = &cobra.Command{
	Use:   "api <METHOD> <path> [--body …] [--v2] [--json]",
	Short: "Raw authenticated API call (debug)",
	Long: `Make an authenticated call against the weside API. Useful for verifying
endpoints before wrapping them in a command, or debugging a response shape.

METHOD: GET|POST|PUT|PATCH|DELETE
path:   relative to the API base, with leading slash (e.g. /companions/10)
--v2:   use the /api/v2 base instead of /api/v1
--body: inline JSON, @file to read a file, or - to read stdin
--json: raw body on stdout (no status line, no pretty-print) for piping into jq

Examples:
  weside api GET /companions/10
  weside api GET /rooms --v2 --json | jq .rooms
  weside api POST /companions --body '{"name":"x","personality":"y"}'
  weside api PATCH /companions/10 --body @patch.json`,
	Args: cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		method := strings.ToUpper(args[0])
		path := args[1]
		switch method {
		case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		default:
			return fmt.Errorf("unsupported method %q (use GET|POST|PUT|PATCH|DELETE)", method)
		}

		client, err := newAuthClient(apiV2)
		if err != nil {
			return err
		}

		var body any
		if apiBody != "" && method != http.MethodGet && method != http.MethodDelete {
			raw, err := readAPICmdBody(apiBody)
			if err != nil {
				return err
			}
			if len(raw) > 0 {
				var parsed any
				if json.Unmarshal(raw, &parsed) != nil {
					// not JSON; send as a JSON string literal
					parsed = string(raw)
				}
				body = parsed
			}
		}

		resp, err := client.DoRaw(context.Background(), method, path, body)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		respBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if apiJSON {
			// Scripting mode: raw body only, no pretty-print, no status line.
			_, _ = os.Stdout.Write(respBytes)
			if len(respBytes) > 0 && respBytes[len(respBytes)-1] != '\n' {
				fmt.Println()
			}
			return nil
		}

		fmt.Fprintf(os.Stderr, "HTTP %d %s\n", resp.StatusCode, http.StatusText(resp.StatusCode))
		ct := resp.Header.Get("Content-Type")
		if strings.Contains(ct, "application/json") {
			var pretty bytes.Buffer
			if json.Indent(&pretty, respBytes, "", "  ") == nil {
				fmt.Println(pretty.String())
			} else {
				_, _ = os.Stdout.Write(respBytes)
			}
		} else {
			_, _ = os.Stdout.Write(respBytes)
		}
		return nil
	},
}

func newAuthClient(v2 bool) (*api.Client, error) {
	if v2 {
		return newAuthenticatedClientV2()
	}
	return newAuthenticatedClient()
}

// readAPICmdBody resolves --body: inline JSON, @file, or - for stdin.
func readAPICmdBody(spec string) ([]byte, error) {
	switch {
	case spec == "-":
		return io.ReadAll(stdinReader)
	case strings.HasPrefix(spec, "@"):
		return os.ReadFile(strings.TrimPrefix(spec, "@"))
	default:
		return []byte(spec), nil
	}
}

func init() {
	apiCmd.Flags().BoolVar(&apiV2, "v2", false, "use the /api/v2 base")
	apiCmd.Flags().BoolVar(&apiJSON, "json", false, "raw body on stdout, for piping")
	apiCmd.Flags().StringVar(&apiBody, "body", "", "request body: inline JSON, @file, or - for stdin")
	rootCmd.AddCommand(apiCmd)
}
