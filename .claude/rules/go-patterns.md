---
description: Go coding patterns and conventions for weside CLI
globs: ["**/*.go"]
---

# Go Patterns

## Error Handling

- Wrap errors with context: `fmt.Errorf("listing companions: %w", err)`
- Return errors, don't panic (panic = only for unrecoverable bugs)
- Sentinel errors for known cases: `var ErrNotFound = errors.New("not found")`
- 401 errors: clear message directing user to re-login

## Testing

- Table-driven tests (Go standard):
  ```go
  tests := []struct{ name, input, want string }{...}
  for _, tt := range tests { t.Run(tt.name, func(t *testing.T) {...}) }
  ```
- Mock HTTP via `httptest.NewServer` (stdlib)
- Test files next to code: `foo.go` → `foo_test.go`
- Use `t.TempDir()` for file-based tests
- Use `t.Setenv()` for env var tests

## API Client

- Base client: `internal/api/client.go`
- All methods return `(Result, error)` — never just error
- JSON decoding: `json.NewDecoder(resp.Body)` — not `io.ReadAll`
- Context propagation: all API calls take `ctx context.Context`
- Parse responses as `map[string]any` (field names vary per endpoint)
- Trailing slash matters: `/data-residency/` not `/data-residency`

## Command Structure

- One file per command group: `cmd/<noun>.go`
- Use `newAuthenticatedClient()` for API access
- Support `--json` output: `if IsJSON() { ui.PrintJSON(result); return nil }`
- Errors to stderr, data to stdout
- Register commands in `init()`: `rootCmd.AddCommand(<noun>Cmd)`

## Output

- `--json` flag → JSON to stdout, errors to stderr
- Without `--json` → `ui.PrintTable()` for lists, `fmt.Printf()` for details
- Use `truncate(s, maxLen)` for table cell content

## Commits

- Conventional Commits: `feat:`, `fix:`, `test:`, `ci:`, `docs:`, `chore:`
- Include `WA-XXX` Jira reference when applicable
- Co-Author line for Claude Code contributions
