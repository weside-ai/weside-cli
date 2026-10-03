---
description: Go coding patterns and conventions for weside CLI
paths:
  - "**/*.go"
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
- Mock HTTP via `httptest.NewServer` (stdlib); `cmd/chat_test.go` is the model
- Test files next to code: `foo.go` → `foo_test.go`
- Use `t.TempDir()` for file-based tests
- Use `t.Setenv()` for env var tests

## API Client

- Base client: `internal/api/client.go`
- `Get/Post/Put/Patch/Delete` decode into a `result any` out-parameter and return `error`; `DoRaw`/`Subscribe` return `(*http.Response, error)`
- JSON decoding: `json.NewDecoder(resp.Body)` — not `io.ReadAll`
- Context propagation: all API calls take `ctx context.Context`
- Parse responses as `map[string]any` (field names vary per endpoint)
- Trailing slash matters: `/data-residency/` not `/data-residency`

## Command Structure

- One file per command group: `cmd/<noun>.go`
- Use `newAuthenticatedClient()` for the v1 API, `newAuthenticatedClientV2()` for `/api/v2/*`
- Support `--json` output: `if IsJSON() { ui.PrintJSON(result); return nil }`
- Errors to stderr, data to stdout
- Register commands in `init()`: `rootCmd.AddCommand(<noun>Cmd)`

## Adding a command

- Probe the endpoint first: `weside api GET /some/path --json` shows the real response shape.
- Long-lived/SSE calls take `cmd.Context()`, never `context.Background()`, so Ctrl-C cancels them; `client.Subscribe` / `DoRawNoTimeout` escape the 30 s request timeout.
- Side-effecting commands (cancel, undo, context-break, deactivate) are gated behind `--confirm`.
- `resolveCompanion(flagValue)` takes an id or a name and falls back to the selected companion; use it instead of reading `default_companion_id`.

## Response keys

Keys differ per endpoint (`result["companions"]`, not `"items"`):

- Companions: `{"companions": [...], "total": N}`
- Memories: `{"memories": [...]}`
- Goals: `{"active": [...], "paused": [...], "completed": [...]}`
- Provider: `{"type": "...", "model_name": "...", "preset_display_name": "..."}`
- Presets: `{"groups": [{"region": "EUR", "presets": [...]}]}`
- Rooms (v2): `{"rooms": [...], "total": N}`; timeline `{"messages": [...], "next_cursor": ..., "prev_cursor": ...}`; message `{"role": "user|assistant|mentor|system", "content": [{"type":"text","text":"..."}]}`
- Chat (v2): the reply arrives over the room SSE stream as `room_message_delta` / `room_message_complete`; the complete frame is `{"message": {"role": "assistant", "content": [{"type": "text", "text": "..."}]}}`

## Output

- `--json` flag → JSON to stdout, errors to stderr
- Without `--json` → `ui.PrintTable()` for lists, `fmt.Printf()` for details
- Use `truncate(s, maxLen)` for table cell content
