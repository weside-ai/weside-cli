# weside-cli

Go CLI for the weside.ai AI Companion Platform.

**Workspace:** `~/weside/AGENTS.md` — Cross-repo Overview

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
| Language | Go 1.25+ (`go 1.25.12`, toolchain pinned in `go.mod`) |
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
│   ├── rooms.go            # rooms list/show/mute/unmute/activity/delete (v2)
│   ├── rooms_debug.go      # rooms trace/participants/tool-call/cancel/undo/regenerate/context-break/rename/group/dm/events (v2)
│   ├── rooms_confirmations.go # rooms confirmations <id> — the Gefallen asks a room carries + their durable status
│   ├── stage.go            # stage list/delete — artifacts a companion rendered (v2)
│   ├── search.go           # search <query> — memories, notes, files (v2, grouped)
│   ├── invite.go           # invite mint/rotate/show/accept
│   ├── ablage.go           # ablage search <query> — vault entries + plain-storage files by name
│   ├── stickers.go         # stickers packs list/show/upload/publish/export/send
│   ├── stickers_workshop.go # stickers workshop slots/generate/reroll/keep/discard/style
│   ├── secret_input.go     # secret input: argv, opt-in stdin (never echoed), stored-key reuse
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
│   ├── tools.go            # tools discover/exec (MCP)
│   ├── config.go           # config show/set/refresh-auth
│   ├── completion.go       # shell completion
│   └── version.go          # version (ldflags injected)
├── internal/               # Private packages (Go compiler enforced)
│   ├── api/client.go       # HTTP client (Get/Post/Put/Patch/Delete, PostStream/PutStream/PostMultipart, DoRaw/DoRawNoTimeout/Subscribe)
│   ├── auth/               # storage.go (token persistence, ~/.weside/credentials.json), pkce.go (OAuth + refresh grant), mfa.go (TOTP step), discovery.go (auth config)
│   ├── mcp/client.go       # MCP client (tools discover/exec)
│   ├── config/config.go    # Config dir + default companion
│   └── ui/                 # output.go (JSON/table), markdown.go (glamour, TTY-only), terminal.go (SafeText: escape terminal controls in remote text)
├── Makefile                # Build targets
├── .golangci.yml           # Linter config (v2 format)
├── .goreleaser.yaml        # Release config (6 platforms + Homebrew)
└── .github/workflows/      # CI (lint, test, build, security), Claude Code Review, Release, npm Publish
```

## Backend API

| Env | Base URL |
|-----|----------|
| **Prod** | `https://api.weside.ai/api/v1` |
| **Dev** | `http://localhost:8000/api/v1` |

Auth: Bearer JWT in `Authorization` header.
API Docs: `weside-core/apps/backend` (Swagger at `/docs`).

## Git Workflow

**Branch format:** `<type>/WA-XXX-short-description` (without a ticket: `<type>/short-description`)

Types: `feat`, `fix`, `docs`, `ci`, `test`, `chore`, `refactor`

**Commit format:** Conventional Commits

```
<type>(<scope>): <subject>

WA-XXX
```

`WA-XXX` is mandatory once a ticket exists; bot commits and small `docs:`/`chore:` commits without a ticket omit it.

**Branch protection on main:** PR required; required checks `lint`, `test`, `build (linux, amd64)`, `security`, `claude-review`.

> `security` (govulncheck) is **blocking** — there is no `continue-on-error` in `.github/workflows/ci.yml`. A vulnerability in a *called* code path fails the PR, so a dependency bump is part of the fix.

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

Adding a command and the known response keys: `.claude/rules/go-patterns.md` (loads with `**/*.go`).

## Current Limitations

- **Auth:** OAuth 2.1 Authorization-Code + PKCE via browser (same flow as the weside MCP client), dev mode (`--dev`), and `WESIDE_TOKEN` env. `weside auth login` opens the Supabase OAuth-2.1 authorization endpoint (`/auth/v1/oauth/authorize`) with the registered public CLI client — the user lands on the **weside login page and chooses their sign-in method** (Google / Apple / e-mail), not a hardcoded provider. Tokens exchanged at `/auth/v1/oauth/token` (`authorization_code` grant), stored in `~/.weside/credentials.json`. Login binds an OAuth `state` (CSRF) and the callback server tries ports 18520→18522 (all registered redirect_uris on the client; the OAuth-2.1 server validates redirect_uri exactly, no DCR). `18520` is also the well-known `callback_port` field (see Auth-config discovery below) — the base port is a platform contract, not a magic number.
  - **Server-side requirement:** each Supabase project's redirect allowlist must carry `http://localhost:18520/callback`, or `weside auth login` cannot complete — **all three** ports — `18520`, `18521`, `18522` — must be listed, because the callback server falls back through them when the base port is occupied and the OAuth-2.1 server validates `redirect_uri` exactly. Confirmed present on both staging (`yauruvmadvvdravrlixu`) and production (`pqykrwpmhjqjhpsnjxbd`) as of 2026-07-25 (`weside-core/docs/ops/runbook-wa998-supabase-url-cutover.md`).
  - **Two-factor step (WA-2309):** a Supabase OAuth-server session is always `aal1`. After the code exchange, `finishLogin` (`cmd/auth.go`) reads `GET /auth/v1/user` → `factors[]`; with a verified `totp` factor it prompts for the code (hidden, stderr, `charmbracelet/x/term`) and runs `POST /auth/v1/factors/{id}/challenge` + `/verify` with the session's own access token (`internal/auth/mfa.go`). The returned `aal2` tokens are stored; a refresh through `/auth/v1/oauth/token` keeps `aal2`. The backend's 403 for an `aal1` OAuth session of a factor user has the problem type `mfa-session-confirmation-required` (formerly `mfa-session-binding-required`); `api.Error` and the MCP client match both slugs and print "run `weside auth login` again", never the URL.
  - **First-time authorization** (no remembered `oauth_consents` row for this client + user) bounces through weside's own consent screen, `mobile.weside.ai/oauth/mcp-consent` — the same screen MCP clients (Claude Code, Cursor) use, not a CLI-specific page. That route's effective URL is controlled by the Supabase-side `oauth_server_authorization_path` setting (a path, resolved against Site URL); see the runbook above for an incident where a stale value briefly broke this for all Supabase-OAuth-2.1 clients including this CLI.
  - **Auth-config discovery:** `internal/auth/discovery.go` resolves Supabase URL + anon-key + callback port + MCP URL + OAuth client_id via `Resolve()`. Precedence: `--supabase-url`/`--supabase-anon-key` flags (must be set together) → `WESIDE_SUPABASE_URL` / `WESIDE_SUPABASE_ANON_KEY` env (must be set together) → `~/.weside/config.yaml` `auth.*` cache → live GET `<api_url>/.well-known/weside-auth` (5s timeout, response cached) → hardcoded fallback constants in `discovery.go`. `oauth_client_id` is an **optional** well-known field (older backends omit it → hardcoded default `91aa6153-…`, a public PKCE client, non-sensitive). Run `weside config refresh-auth` to force-refresh the cache. AC-6 (auto-refresh on 401) is not implemented: `auth.RefreshAccessToken` (`internal/auth/pkce.go`) exists, but no command calls it yet.
- **Chat (v2, WA-1548):** Room-based. `weside chat <companion> -m "…"` resolves the companion's DM room (`POST /api/v2/rooms/dm/{companion_id}`), opens the room SSE subscription (`GET /api/v2/rooms/{room_id}/events`), and only then sends (`POST /api/v2/rooms/{room_id}/messages`). The reply arrives over the stream as `room_message_delta` (live with `--stream`) / `room_message_complete` (fallback when no deltas). A `client_message_id` idempotency key is sent on every POST.
  - **Event correlation matters:** a room can have concurrent/queued turns, so `sendChat` records the `active_turns` from the `connected` frame, captures its own turn's `server_message_id` from `room_message_start`, and ignores deltas/completions from any other turn. It also terminates on `room_turn_ended` (cancelled/failed/timed_out) — without that the CLI hangs forever on those outcomes.
- **Rooms (v2):** `rooms list/show/mute/unmute/delete` plus the debug surface in `cmd/rooms_debug.go`: `rooms trace <id>` (checkpoint trace), `rooms participants <id>`, `rooms tool-call <id> <tcid>`, `rooms cancel <id> --confirm`, `rooms undo <id> --confirm`, `rooms context-break <id> --confirm`, `rooms rename <id> [title] [--clear]`, `rooms group --companions …`, `rooms dm <companion_id>`, `rooms events <id> [--since] [--raw]` (live SSE mitschnitt), `rooms confirmations <id> [--limit]` (the Gefallen asks in the timeline plus the status the server holds for each — the API exposes an ask by id only, so the ids are parsed out of the messages). All on `/api/v2/rooms/*`; destructive commands are gated by `--confirm`. `rooms list --json` preserves the caller-owned `muted` field. `rooms show` pages via `--cursor`/`--after`/`--limit`; `rooms trace --full` prints untruncated tool output.
  - **`rooms activity <id>`** reads the durable activity feed (tool-audit rows — tool calls, memory saves, note writes), scoped to your own companions. A newest-first slice bounded by `--limit`, **never a page**. `--scope last_turn` narrows to the newest turn per own companion. (`rooms show --cursor` is the MESSAGE timeline and is unaffected.)
- **Invite (v1, WA-2235):** `invite mint [--room <id>]`, `invite rotate [--room <id>]`, `invite show <code>`, `invite accept <code>` — the ONE invite code for the app and for a room, on `/api/v1/invites/*`. `--room` picks the room scope (owner only); no flag is app scope, sent as an absent key, never `room_id: 0`. `accept` is the half that needs the *second* identity, which is why it is a verb. Unknown, expired, revoked and spent codes all answer the same 404 by design, so the error cannot tell you which.
- **Stage & search (v2):** `stage list [--room] [--cursor] [--limit]` / `stage delete <artifact_id>` — artifacts a companion rendered belong to the user, so they outlive the room they were born in. `search <query> [--limit]` queries memories, notes and files; results stay **grouped per engine** (vector distance and Postgres FTS ranks are not on one scale), and each group carries its own availability so "nothing there" is distinguishable from "it did not run".
- **Ops & account (v1):** `files tree/quota/delete`, `me usage [--month] [--daily]`, `user-config get/set/delete`, `sandbox-secrets list/presets/put/delete` (masked), `config system [key]`, `notes list/get/search`, `notes-repo status/repair`, `notes-pat list/mint/revoke`, `provider byok-test/byok-discover`.
- **Lower-frequency (v1/v2):** `referrals list/create/revoke/stats`, `circles list/create/delete`, `plans show/me`, `billing usage/purchase-eligibility`, `channels list/set-active`, `experts list/befriend`, `evolution current/start/dismiss/presets`, `reminders list/dismiss`, `mentor-sessions <companion>`, `subscriptions list/toggle`, `me-account profile/profile-set/locale/sliding-window/export/deactivate`, `integrations list/catalog/disconnect/reconcile`, `safety block/unblock`. List tables are best-effort (first array in the response); `--json` gives the exact wire shape.
- **Tools:** `tools discover` and `tools exec <name> [json-args]` call the weside MCP server.
- **Output:** lipgloss v2 for tables, glamour v2 for markdown rendering (TTY-only — piped output stays plain). `--json` always emits the raw wire shape.
- **Debug tooling:** `api <METHOD> <path> [--body <json|@file|->] [--v2] [--json]` — raw authenticated passthrough, the fastest way to verify an endpoint before wrapping it. `auth token --decode` prints the JWT claims (sub/exp/email; no signature check). `rooms events <id> [--since <cursor>] [--raw]` streams every SSE frame unfiltered. `rooms show --cursor/--after/--limit` pages the timeline; `rooms trace --full` skips output truncation.
- **Memories/Goals:** `memories search/list/save` (v1 + MCP) plus `memories get/delete/update/edit` (metadata + content versioning). `goals list/update(by title)/save` plus `goals edit/reorder`. New write commands take `--companion` (defaults to the selected companion).
- **Companions:** `list`, `show`, `create`, `select`, `identity`, `update`, `delete` plus `companions skills list/available/install/set/uninstall`, `companions resume`, `companions prompts versions/show/restore`, `companions identity show/set`, `companions tools list/set`. Companion media upload is a Follow-up (multipart upload exists for `stickers`).
- **Triggers:** `triggers list/toggle/set/delete <companion>` — debug why a trigger fires or not.

## Security

Never commit tokens, credentials, or contents of `~/.weside/credentials.json`; never print a
token or secret value into a log, test fixture, or commit message. If a task needs backend or
infrastructure secrets handling, that lives in `weside-infrastructure/docs/security/SECRETS.md`
(separate, private repo) — not here.
