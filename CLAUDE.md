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
| Language | Go 1.25+ (`go 1.25.12`, toolchain 1.26.x) |
| CLI Framework | Cobra 1.10 + Viper 1.21 |
| HTTP Client | net/http (stdlib) |
| Auth | File-based token storage + WESIDE_TOKEN env |
| Output styling | lipgloss v2 (tables) + glamour v2 (markdown, TTY-only) |
| Testing | go test (stdlib) |
| Linting | golangci-lint v2 + gofumpt |
| Release | GoReleaser + GitHub Actions (+ npm Trusted Publishing) |

## Project Structure

```
weside-cli/
├── main.go                 # Entry point (calls cmd.Execute)
├── cmd/                    # Cobra commands (1 file per command group)
│   ├── root.go             # Root command + global flags + Viper init
│   ├── auth.go             # auth login/logout/whoami/token
│   ├── companions.go       # companions list/show/create/select/update/delete
│   ├── api.go              # api <METHOD> <path> — raw authenticated passthrough (debug)
│   ├── chat.go             # chat (v2 rooms SSE: resolve DM room, subscribe, send)
│   ├── rooms.go            # rooms list/show/delete (v2)
│   ├── rooms_debug.go      # rooms trace/participants/tool-call/cancel/undo/context-break/rename/group/dm/events/invites (v2)
│   ├── skills.go           # companions skills list/available/install/set/uninstall
│   ├── prompts.go          # companions resume/prompts versions/show/restore, identity show/set, tools list/set
│   ├── triggers.go         # triggers list/toggle/set/delete
│   ├── memories.go         # memories search/list/save
│   ├── memories_edit.go    # memories get/delete/update/edit
│   ├── goals.go            # goals list/update/save
│   ├── goals_edit.go       # goals edit/reorder
│   ├── files.go            # files tree/quota/delete
│   ├── notes.go            # notes list/get/search + notes-repo + notes-pat
│   ├── account.go          # me usage, user-config, sandbox-secrets
│   ├── account_extras.go   # provider byok-test/byok-discover, config system
│   ├── p3_ops.go           # referrals, circles, plans, billing, channels, experts, safety
│   ├── p3_companion.go     # evolution, reminders, mentor-sessions, subscriptions, me-account, integrations
│   ├── provider.go         # provider show/presets/set/byok
│   ├── tools.go            # tools discover (stub)
│   ├── config.go           # config show/set/refresh-auth
│   ├── completion.go       # shell completion
│   └── version.go          # version (ldflags injected)
├── internal/               # Private packages (Go compiler enforced)
│   ├── api/client.go       # HTTP client (Get/Post/Put/Patch/Delete/DoRaw/DoRawNoTimeout/Subscribe)
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

**Branch protection on main:** PR required, CI must pass (lint, test, build, **security**).

> `security` (govulncheck) is **blocking** — there is no `continue-on-error` in `.github/workflows/ci.yml`. A vulnerability in a *called* code path fails the PR, so a dependency bump is part of the fix. (v1.0.0 was blocked by GO-2026-5970 in `golang.org/x/text`, reachable via `ui.PrintError`.)

**Release & Install:**

A `v*` tag triggers **two independent workflows**: `Release` (GoReleaser → GitHub Release binaries + Homebrew tap) and `npm Publish` (`.github/workflows/npm-publish.yml` → npmjs). The npm job derives the package version from the tag (`npm version ${TAG#v}`) and publishes via **OIDC Trusted Publishing** — no NPM_TOKEN. Do not set `registry-url` in `setup-node` there; it writes an `.npmrc` placeholder that breaks the OIDC fallback.

```bash
# 1. Tag + push (triggers BOTH workflows)
git tag -a v1.1.0 -m "…" && git push origin v1.1.0

# 2. Verify both runs
gh run list -R weside-ai/weside-cli --limit 2   # "Release" + "npm Publish"
gh release view v1.1.0 -R weside-ai/weside-cli  # expect 7 assets
npm view weside-cli version                     # expect the new version

# 3. Install on dev machine (release binary → ~/go/bin/weside)
gh release download v1.1.0 -R weside-ai/weside-cli -p "*linux_amd64*" -D /tmp/weside-release --clobber
tar -xzf /tmp/weside-release/weside-cli_*.tar.gz -C /tmp/weside-release/
cp /tmp/weside-release/weside ~/go/bin/weside   # binary inside the archive is `weside`
rm -rf /tmp/weside-release
weside version  # verify
```

**Do NOT use `go install`** — it doesn't inject version ldflags (`weside version` shows "dev").

`npm/package.json` carries its own `version` field, but the workflow overwrites it from the tag — bumping it by hand is unnecessary and drifts.

Users install via:
- **Homebrew:** `brew install weside-ai/tap/weside-cli`
- **npm:** `npm install -g weside-cli` (or `npx weside-cli@latest`)

## How to Add a New Command

1. Create `cmd/<noun>.go`
2. Define `var <noun>Cmd = &cobra.Command{...}`
3. Add subcommands: `var <noun>ListCmd = &cobra.Command{...}`
4. Register in `init()`: `rootCmd.AddCommand(<noun>Cmd)`
5. Use `newAuthenticatedClient()` for authenticated API calls (v1 surface), `newAuthenticatedClientV2()` for the `/api/v2/*` surface (chat, rooms)
6. Parse API responses as `map[string]any` (API field names vary)
7. Support `--json` output: `if IsJSON() { ui.PrintJSON(result); return nil }`
8. Write tests in `cmd/<noun>_test.go` (`httptest.NewServer`, see `cmd/chat_test.go`)

Tips:
- **Probe the endpoint first** with `weside api GET /some/path --json` — cheaper than guessing the response shape from the schema.
- **Long-lived/SSE calls** take `cmd.Context()` (never `context.Background()`) so Ctrl-C cancels them, and use `client.Subscribe` / `DoRawNoTimeout` to escape the 30 s request timeout.
- **Side-effecting commands** (cancel, undo, context-break, deactivate) are gated behind `--confirm`.
- **Companion resolution:** `resolveCompanion(flagValue)` accepts an id or name and falls back to the selected companion when empty — prefer it over reading `default_companion_id` directly.

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
- Memories: `{"memories": [...]}`
- Goals: `{"active": [...], "paused": [...], "completed": [...]}`
- Provider: `{"type": "...", "model_name": "...", "preset_display_name": "..."}`
- Presets: `{"groups": [{"region": "EUR", "presets": [...]}]}`
- Rooms (v2): `{"rooms": [...], "total": N}`; room timeline `{"messages": [...], "next_cursor": ..., "prev_cursor": ...}`; each message `{"role": "user|assistant|mentor|system", "content": [{"type":"text","text":"..."}]}`
- Chat (v2): the reply arrives over the room SSE stream as `room_message_delta`/`room_message_complete` events; the complete frame is `{"message": {"role": "assistant", "content": [{"type": "text", "text": "..."}]}}`

## Current Limitations

- **Auth:** OAuth 2.1 Authorization-Code + PKCE via browser (same flow as the weside MCP client), dev mode (`--dev`), and `WESIDE_TOKEN` env. `weside auth login` opens the Supabase OAuth-2.1 authorization endpoint (`/auth/v1/oauth/authorize`) with the registered public CLI client — the user lands on the **weside login page and chooses their sign-in method** (Google / Apple / e-mail), not a hardcoded provider. Tokens exchanged at `/auth/v1/oauth/token` (`authorization_code` grant), stored in `~/.weside/credentials.json`. Login binds an OAuth `state` (CSRF) and the callback server tries ports 18520→18522 (all registered redirect_uris on the client; the OAuth-2.1 server validates redirect_uri exactly, no DCR). `18520` is also the well-known `callback_port` field (see Auth-config discovery below) — the base port is a platform contract, not a magic number.
  - **Server-side requirement:** each Supabase project's redirect allowlist must carry `http://localhost:18520/callback`, or `weside auth login` cannot complete — **all three** ports — `18520`, `18521`, `18522` — must be listed, because the callback server falls back through them when the base port is occupied and the OAuth-2.1 server validates `redirect_uri` exactly. Confirmed present on both staging (`yauruvmadvvdravrlixu`) and production (`pqykrwpmhjqjhpsnjxbd`) as of 2026-07-25 (`weside-core/docs/ops/runbook-wa998-supabase-url-cutover.md`). The WA-998 cutover initially carried only `18520`, which would have broken any fallback login; the two missing ports were added the same day once this file's own contract was checked against the live config.
  - **First-time authorization** (no remembered `oauth_consents` row for this client + user) bounces through weside's own consent screen, `mobile.weside.ai/oauth/mcp-consent` — the same screen MCP clients (Claude Code, Cursor) use, not a CLI-specific page. That route's effective URL is controlled by the Supabase-side `oauth_server_authorization_path` setting (a path, resolved against Site URL); see the runbook above for an incident where a stale value briefly broke this for all Supabase-OAuth-2.1 clients including this CLI.
  - **Auth-config discovery:** `internal/auth/discovery.go` resolves Supabase URL + anon-key + callback port + MCP URL + OAuth client_id via `Resolve()`. Precedence: `--supabase-url`/`--supabase-anon-key` flags (must be set together) → `WESIDE_SUPABASE_URL` / `WESIDE_SUPABASE_ANON_KEY` env (must be set together) → `~/.weside/config.yaml` `auth.*` cache → live GET `<api_url>/.well-known/weside-auth` (5s timeout, response cached) → hardcoded fallback constants in `discovery.go`. `oauth_client_id` is an **optional** well-known field (older backends omit it → hardcoded default `91aa6153-…`, a public PKCE client, non-sensitive). Run `weside config refresh-auth` to force-refresh the cache. AC-6 (auto-refresh on 401) is deferred — there is no existing CLI refresh flow to wrap.
- **Chat (v2, WA-1548):** Room-based. `weside chat <companion> -m "…"` resolves the companion's DM room (`POST /api/v2/rooms/dm/{companion_id}`), opens the room SSE subscription (`GET /api/v2/rooms/{room_id}/events`), and only then sends (`POST /api/v2/rooms/{room_id}/messages`). The reply arrives over the stream as `room_message_delta` (live with `--stream`) / `room_message_complete` (fallback when no deltas). A `client_message_id` idempotency key is sent on every POST. Threads no longer exist — the room is the conversation.
  - **Event correlation matters:** a room can have concurrent/queued turns, so `sendChat` records the `active_turns` from the `connected` frame, captures its own turn's `server_message_id` from `room_message_start`, and ignores deltas/completions from any other turn. It also terminates on `room_turn_ended` (cancelled/failed/timed_out) — without that the CLI hangs forever on those outcomes.
- **Rooms (v2):** `rooms list/show/delete` plus the debug surface in `cmd/rooms_debug.go`: `rooms trace <id>` (checkpoint trace), `rooms participants <id>`, `rooms tool-call <id> <tcid>`, `rooms cancel <id> --confirm`, `rooms undo <id> --confirm`, `rooms context-break <id> --confirm`, `rooms rename <id> [title] [--clear]`, `rooms group --companions …`, `rooms dm <companion_id>`, `rooms events <id> [--since] [--raw]` (live SSE mitschnitt), `rooms invites list/create/revoke`. All on `/api/v2/rooms/*`; side-effecting commands gated by `--confirm`. `rooms show` pages via `--cursor`/`--after`/`--limit`; `rooms trace --full` prints untruncated tool output.
- **Ops & account (v1):** `files tree/quota/delete`, `me usage [--month] [--daily]`, `user-config get/set/delete`, `sandbox-secrets list/presets/put/delete` (masked), `config system [key]`, `notes list/get/search`, `notes-repo status/repair`, `notes-pat list/mint/revoke`, `provider byok-test/byok-discover`.
- **Lower-frequency (v1/v2):** `referrals list/create/revoke/stats`, `circles list/create/delete`, `plans show/me`, `billing usage/purchase-eligibility`, `channels list/set-active`, `experts list/befriend`, `evolution current/start/dismiss/presets`, `reminders list/dismiss`, `mentor-sessions <companion>`, `subscriptions list/toggle`, `me-account profile/profile-set/locale/sliding-window/export/deactivate`, `integrations list/catalog/disconnect/reconcile`, `safety block/unblock`. List tables are best-effort (first array in the response); `--json` gives the exact wire shape.
- **Tools:** `discover` attempts MCP call, `schema` and `exec` are stubs.
- **Output:** lipgloss v2 for tables, glamour v2 for markdown rendering (TTY-only — piped output stays plain). `--json` always emits the raw wire shape.
- **Debug tooling:** `api <METHOD> <path> [--body <json|@file|->] [--v2] [--json]` — raw authenticated passthrough, the fastest way to verify an endpoint before wrapping it. `auth token --decode` prints the JWT claims (sub/exp/email; no signature check). `rooms events <id> [--since <cursor>] [--raw]` streams every SSE frame unfiltered. `rooms show --cursor/--after/--limit` pages the timeline; `rooms trace --full` skips output truncation.
- **Memories/Goals:** `memories search/list/save` (v1 + MCP) plus `memories get/delete/update/edit` (metadata + content versioning). `goals list/update(by title)/save` plus `goals edit/reorder`. New write commands take `--companion` (defaults to the selected companion).
- **Companions:** `list`, `show`, `create`, `select`, `identity`, `update`, `delete` plus `companions skills list/available/install/set/uninstall`, `companions resume`, `companions prompts versions/show/restore`, `companions identity show/set`, `companions tools list/set`. Media upload is a Follow-up.
- **Triggers:** `triggers list/toggle/set/delete <companion>` — debug why a trigger fires or not.

---

**Version:** 2.5
**Last Updated:** 2026-07-25 (CLI v1.0.0; documented the Supabase-side redirect-allowlist + mcp-consent requirements for `weside auth login`)
