# weside-cli

Go CLI for the weside.ai AI Companion Platform.

**Workspace:** `~/weside/CLAUDE.md` — Cross-repo Overview

---

## Essential Commands

```bash
make build              # Build binary (with version via ldflags)
make test               # Run tests + coverage report
make lint               # golangci-lint + gofumpt check
make fmt                # Auto-format all Go files
make security           # govulncheck vulnerability scan
make release-snapshot   # Test GoReleaser locally (no publish)
```

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Language | Go 1.23+ |
| CLI Framework | Cobra 1.10 + Viper 1.21 |
| HTTP Client | net/http (stdlib) |
| Auth | File-based token storage + WESIDE_TOKEN env |
| Testing | go test (stdlib) |
| Linting | golangci-lint v2 + gofumpt |
| Release | GoReleaser + GitHub Actions |

## Project Structure

```
weside-cli/
├── main.go                 # Entry point (calls cmd.Execute)
├── cmd/                    # Cobra commands (1 file per command group)
│   ├── root.go             # Root command + global flags + Viper init
│   ├── auth.go             # auth login/logout/whoami/token
│   ├── companions.go       # companions list/show/create/select
│   ├── chat.go             # chat (streaming SSE, stdin pipe)
│   ├── threads.go          # threads list/show/delete
│   ├── memories.go         # memories search/list
│   ├── goals.go            # goals list/update
│   ├── provider.go         # provider show/presets/set/byok
│   ├── tools.go            # tools discover (stub)
│   ├── config.go           # config show/set
│   └── version.go          # version (ldflags injected)
├── internal/               # Private packages (Go compiler enforced)
│   ├── api/client.go       # HTTP client (Get/Post/Put/Patch/Delete/DoRaw)
│   ├── auth/storage.go     # Token persistence (~/.weside/credentials.json)
│   ├── config/config.go    # Config dir + default companion
│   └── ui/output.go        # JSON, table, success/error output
├── Makefile                # Build targets
├── .golangci.yml           # Linter config (v2 format)
├── .goreleaser.yaml        # Release config (5 platforms + Homebrew)
└── .github/workflows/      # CI (lint, test, build, security) + Release
```

## Backend API

| Env | Base URL |
|-----|----------|
| **Prod** | `https://api.weside.ai/api/v1` |
| **Dev** | `http://localhost:8000/api/v1` |

Auth: Bearer JWT in `Authorization` header.
API Docs: `weside-core/apps/backend` (Swagger at `/docs`).

## Git Workflow

**Branch format:** `<type>/WA-XXX-short-description`

Types: `feat`, `fix`, `docs`, `ci`, `test`, `chore`, `refactor`

**Commit format:** Conventional Commits

```
<type>(<scope>): <subject>

WA-XXX
```

**Branch protection on main:** PR required, CI must pass (lint, test, build).

**Release & Install:**

Tag `v*` triggers GoReleaser → GitHub Releases + Homebrew Tap + npm.

```bash
# 1. Tag + push (triggers GoReleaser CI)
git tag v0.4.0 && git push origin v0.4.0

# 2. Verify release build
gh run list -R weside-ai/weside-cli --limit 1

# 3. Install on dev machine (release binary → ~/go/bin/weside)
gh release download v0.4.0 -R weside-ai/weside-cli -p "*linux_amd64*" -D /tmp/weside-release --clobber
tar -xzf /tmp/weside-release/weside-cli_*.tar.gz -C /tmp/weside-release/
cp /tmp/weside-release/weside-cli ~/go/bin/weside
rm -rf /tmp/weside-release
weside version  # verify
```

**Do NOT use `go install`** — it doesn't inject version ldflags (`weside version` shows "dev").

Users install via:
- **Homebrew:** `brew install weside-ai/tap/weside`
- **npm:** `npm install -g @weside-ai/cli`

## How to Add a New Command

1. Create `cmd/<noun>.go`
2. Define `var <noun>Cmd = &cobra.Command{...}`
3. Add subcommands: `var <noun>ListCmd = &cobra.Command{...}`
4. Register in `init()`: `rootCmd.AddCommand(<noun>Cmd)`
5. Use `newAuthenticatedClient()` for authenticated API calls
6. Parse API responses as `map[string]any` (API field names vary)
7. Support `--json` output: `if IsJSON() { ui.PrintJSON(result); return nil }`
8. Write tests in `cmd/<noun>_test.go`

## API Response Parsing Pattern

Backend responses use different key names per endpoint. Always use `map[string]any`:

```go
var result map[string]any
client.Get(ctx, "/companions", &result)
companions, _ := result["companions"].([]any)  // NOT "items"!
for _, item := range companions {
    c, _ := item.(map[string]any)
    name := fmt.Sprintf("%v", c["name"])
}
```

**Known response keys:**
- Companions: `{"companions": [...], "total": N}`
- Threads: `{"threads": [...], "total": N}`
- Memories: `{"memories": [...]}`
- Goals: `{"active": [...], "paused": [...], "completed": [...]}`
- Provider: `{"type": "...", "model_name": "...", "preset_display_name": "..."}`
- Presets: `{"groups": [{"region": "EUR", "presets": [...]}]}`
- Chat: `{"assistant_message": {"content": [{"type": "text", "text": "..."}]}}`

## Current Limitations

- **Auth:** PKCE (Google OAuth via browser), dev mode (`--dev`), and `WESIDE_TOKEN` env.
- **Tools:** `discover` attempts MCP call, `schema` and `exec` are stubs.
- **Output:** Plain text, no colors/styling (lipgloss/glamour not yet integrated).
- **Memories/Goals:** Read-only (creation happens through Companion conversations).

---

**Version:** 2.1
**Last Updated:** 2026-04-07
