# weside-cli

Go CLI for the weside.ai AI Companion Platform.

**Workspace:** `~/weside/CLAUDE.md` — Cross-repo Overview

## Commands

```bash
make build              # Build binary
make test               # Run tests + coverage
make lint               # Lint + format check
make fmt                # Auto-format code
make security           # Vulnerability check
make sync-spec          # Sync OpenAPI spec from weside-core + regenerate types
make generate           # Regenerate Go types from local OpenAPI spec
make release-snapshot   # GoReleaser snapshot (local test)
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.23+ |
| CLI Framework | Cobra + Viper |
| HTTP Client | net/http (stdlib) |
| Auth | Supabase PKCE + go-keyring |
| Output | lipgloss (styled), glamour (markdown) |
| Testing | go test (stdlib) + testify |
| Linting | golangci-lint + gofumpt |
| Release | GoReleaser + GitHub Actions |

## Backend API

- **Prod:** https://api.weside.ai/api/v1
- **Dev:** http://localhost:8000/api/v1
- **Auth:** Bearer JWT in Authorization header
- **Docs:** weside-core/apps/backend (OpenAPI at /docs)

## Conventions

- `internal/` = private packages (Go compiler enforced)
- Tests neben Code: `foo.go` → `foo_test.go`
- Table-Driven Tests (Go Standard-Pattern)
- Error wrapping: `fmt.Errorf("doing X: %w", err)`
- No global state — pass dependencies via structs

## Shared Types

Go types are generated from the same OpenAPI spec as TypeScript types:

```
weside-core: generate-openapi.py → weside-client-openapi.json
  ├── openapi-ts → TypeScript (packages/shared-types/)
  └── oapi-codegen → Go (internal/api/types.gen.go)
```

Sync: `make sync-spec` (copies spec from weside-core + regenerates)

---

**Version:** 1.0
**Last Updated:** 2026-04-01
