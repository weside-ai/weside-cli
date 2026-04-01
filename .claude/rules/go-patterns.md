---
description: Go coding patterns and conventions for weside CLI
globs: ["**/*.go"]
---

# Go Patterns

## Error Handling
- Always wrap errors with context: `fmt.Errorf("listing companions: %w", err)`
- Return errors, don't panic (panic = only for unrecoverable bugs)
- Sentinel errors: `var ErrNotFound = errors.New("not found")`

## Testing
- Table-driven tests:
  ```go
  tests := []struct{ name string; input string; want string }{...}
  for _, tt := range tests { t.Run(tt.name, func(t *testing.T) {...}) }
  ```
- Use testify/assert for readable assertions
- Mock HTTP via httptest.NewServer (stdlib)

## API Client
- Base client in internal/api/client.go
- All methods return (Result, error) — never just error
- JSON decoding via json.NewDecoder(resp.Body) — not ioutil.ReadAll
- Context propagation: all API calls take ctx context.Context

## Output
- --json flag → JSON to stdout, errors to stderr
- Without --json → lipgloss-styled output
- Use ui.Output interface for Strategy Pattern
