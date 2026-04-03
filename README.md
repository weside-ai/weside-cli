# weside CLI

The official command-line interface for [weside.ai](https://weside.ai) — chat with your AI Companions from the terminal.

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap weside-ai/tap
brew install weside-cli
```

### Shell Script (Linux/macOS)

```bash
curl -fsSL https://raw.githubusercontent.com/weside-ai/weside-cli/main/scripts/install.sh | sh
```

### Binary (GitHub Releases)

Download the latest binary from [Releases](https://github.com/weside-ai/weside-cli/releases) for your platform:

- `linux/amd64`, `linux/arm64`
- `darwin/amd64` (Intel Mac), `darwin/arm64` (Apple Silicon)
- `windows/amd64`

### From Source

```bash
go install github.com/weside-ai/weside-cli@latest
```

## Quick Start

```bash
# Log in (dev mode for local development)
weside auth login --dev

# List your Companions
weside companions list

# Set a default Companion
weside companions select nox

# Chat
weside chat -m "Hey, how are you?"

# Stream the response
weside chat --stream -m "Tell me a story"
```

> **Note:** Production login (`weside auth login` without `--dev`) is not yet implemented. Use `--dev` for local development or set `WESIDE_TOKEN` for CI/headless use.

## Commands

### Authentication

| Command | Description |
|---------|-------------|
| `weside auth login --dev` | Log in (dev mode, local backend) |
| `weside auth logout` | Log out and remove stored credentials |
| `weside auth whoami` | Show current authenticated user |
| `weside auth token` | Print access token to stdout (for scripting) |

### Companions

| Command | Description |
|---------|-------------|
| `weside companions list` | List your Companions |
| `weside companions show <id>` | Show Companion details |
| `weside companions create --name "X"` | Create a new Companion |
| `weside companions select <name>` | Set default Companion for chat |

### Chat

| Command | Description |
|---------|-------------|
| `weside chat -m "message"` | Send a message (uses default Companion) |
| `weside chat nox -m "message"` | Send to specific Companion |
| `weside chat --stream -m "msg"` | Stream the response live |
| `weside chat --new -m "msg"` | Start a new thread |
| `weside chat -t <thread_id> -m "msg"` | Continue specific thread |
| `weside chat -f file.txt` | Send message from file |
| `echo "Hi" \| weside chat` | Pipe message from stdin |

### Threads

| Command | Description |
|---------|-------------|
| `weside threads list` | List conversation threads |
| `weside threads show <id>` | Show messages in a thread |
| `weside threads delete <id>` | Delete a thread |

### Memories & Goals

| Command | Description |
|---------|-------------|
| `weside memories search "query"` | Search memories semantically |
| `weside memories list` | List all memories |
| `weside goals list` | List goals (active, paused, completed) |
| `weside goals update <title> --status completed` | Update goal status |

> **Note:** Memory and goal creation happens through conversations with your Companion, not via CLI commands.

### Provider & Data Residency

| Command | Description |
|---------|-------------|
| `weside provider show` | Show current provider configuration |
| `weside provider presets` | List available regional presets |
| `weside provider set <id>` | Set provider preset (numeric ID) |
| `weside provider byok <provider> <key>` | Bring Your Own Key |

### Tools (MCP)

| Command | Description |
|---------|-------------|
| `weside tools discover` | Discover available tool categories |
| `weside tools exec <name> '<json>'` | Execute a tool with JSON arguments |

> **MCP Integration:** The same tools are available as MCP server tools in the [we Plugin](https://github.com/weside-ai/claude-code-plugin) for Claude Code. CLI and MCP share the same API surface — see [API Concepts](#api-concepts) below.

### Configuration

| Command | Description |
|---------|-------------|
| `weside config show` | Show current CLI configuration |
| `weside config set <key> <value>` | Set a configuration value |
| `weside version` | Print version and build info |

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON (for scripting) |
| `--verbose` | Enable verbose output |
| `--api-url` | Custom API URL (default: https://api.weside.ai) |
| `--no-color` | Disable color output |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `WESIDE_TOKEN` | Access token for CI/headless use (skips login) |
| `WESIDE_API_URL` | Custom API base URL |
| `NO_COLOR` | Disable color output (standard) |

## Scripting & Piping

```bash
# Pipe message from stdin
echo "Hello!" | weside chat nox

# JSON output for scripting
weside companions list --json | jq '.companions[0].name'

# Use in CI with env token
WESIDE_TOKEN=xxx weside companions list --json

# Print token for other tools
curl -H "Authorization: Bearer $(weside auth token)" https://api.weside.ai/api/v1/auth/me
```

## Configuration

Config file: `~/.weside/config.yaml`

```yaml
api_url: https://api.weside.ai
default_companion: nox
default_companion_id: "53"
```

Credentials: `~/.weside/credentials.json` (600 permissions)

## API Concepts

The CLI and the [weside MCP server](https://github.com/weside-ai/claude-code-plugin) expose the same weside.ai API. This section describes the shared concepts.

### Companions

A Companion is a persistent AI persona with its own personality, memory, and goals. Each user can have multiple Companions.

| Field | Description |
|-------|-------------|
| `id` | Unique identifier |
| `name` | Display name (used for `companions select`) |
| `personality` | Personality description text |
| `created_at` | Creation timestamp |

### Memories

Companions accumulate memories through conversations. Memories are vector-embedded for semantic search.

| Field | Description |
|-------|-------------|
| `type` | `fact`, `preference`, `experience`, or `reflection` |
| `title` | Short title |
| `content` | Memory content |
| `tags` | Comma-separated tags (optional) |

**Search** is semantic (vector similarity), not keyword-based. "how does auth work" finds memories about authentication even if they don't contain the word "auth".

### Goals

Goals represent what a Companion is working towards. They can be managed by the user or set collaboratively during conversations.

| Field | Description |
|-------|-------------|
| `title` | Goal title (used as identifier for updates) |
| `content` | Goal description and details |
| `status` | `active`, `paused`, or `completed` |
| `due_date` | Target date (YYYY-MM-DD, optional) |
| `tags` | Comma-separated tags (optional) |

### Provider & Data Residency

Controls which LLM provider and region your Companion uses. Presets group providers by geographic region (EU, US, Asia). BYOK (Bring Your Own Key) allows using your own API keys.

### Tools

Tools are actions a Companion can perform (e.g., web search, file operations). Available tools depend on the Companion's configuration and connected MCP servers.

**CLI:** `weside tools discover` / `weside tools exec <name> '<json>'`
**MCP:** `discover_tools()` / `execute_tool(name, arguments)`

---

## Development

### Prerequisites

- Go 1.23+
- golangci-lint v2
- gofumpt
- pre-commit

### Setup

```bash
git clone https://github.com/weside-ai/weside-cli.git
cd weside-cli
pre-commit install --hook-type pre-commit --hook-type commit-msg
make build
```

### Commands

```bash
make build              # Build binary with version info
make test               # Run tests with coverage
make lint               # golangci-lint + gofumpt check
make fmt                # Auto-format code
make security           # govulncheck vulnerability scan
make release-snapshot   # Test GoReleaser locally
```

### Adding a New Command

1. Create `cmd/<noun>.go` with Cobra command
2. Register in `init()` with `rootCmd.AddCommand()`
3. Use `newAuthenticatedClient()` for API calls
4. Support `--json` via `IsJSON()` + `ui.PrintJSON()`
5. Write tests in `cmd/<noun>_test.go`

### Release Process

```bash
git tag v0.2.0
git push origin v0.2.0
# GoReleaser builds binaries + updates Homebrew tap automatically
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
