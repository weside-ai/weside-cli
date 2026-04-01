# weside CLI

The official command-line interface for [weside.ai](https://weside.ai) — chat with your AI Companions from the terminal.

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap weside-ai/tap
brew install weside
```

### Binary (GitHub Releases)

Download the latest binary from [Releases](https://github.com/weside-ai/weside-cli/releases).

### From Source

```bash
go install github.com/weside-ai/weside-cli@latest
```

## Quick Start

```bash
# Log in
weside auth login

# List your Companions
weside companions list

# Set a default Companion
weside companions select nox

# Chat
weside chat -m "Hey, how are you?"

# Stream the response
weside chat --stream -m "Tell me a story"
```

## Commands

| Command | Description |
|---------|-------------|
| `weside auth login` | Log in to weside.ai |
| `weside auth logout` | Log out |
| `weside auth whoami` | Show current user |
| `weside companions list` | List your Companions |
| `weside companions show <id>` | Show Companion details |
| `weside companions create` | Create a new Companion |
| `weside companions select <name>` | Set default Companion |
| `weside chat [companion] -m "msg"` | Send a message |
| `weside chat --stream` | Stream the response |
| `weside threads list` | List conversation threads |
| `weside threads show <id>` | Show thread messages |
| `weside threads delete <id>` | Delete a thread |
| `weside memories search "query"` | Search memories |
| `weside memories list` | List memories |
| `weside memories save` | Save a memory |
| `weside goals list` | List goals |
| `weside goals create` | Create a goal |
| `weside goals update <title>` | Update goal status |
| `weside provider show` | Show provider config |
| `weside provider presets` | List regional presets |
| `weside provider set <preset>` | Set provider preset |
| `weside provider byok <provider> <key>` | Bring Your Own Key |
| `weside tools discover` | Discover available tools |
| `weside config show` | Show CLI configuration |
| `weside config set KEY VALUE` | Set a config value |
| `weside version` | Print version info |

## Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Output as JSON (for scripting) |
| `--verbose` | Enable verbose output |
| `--api-url` | Custom API URL |
| `--no-color` | Disable color output |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `WESIDE_TOKEN` | Access token for CI/headless use |
| `WESIDE_API_URL` | Custom API base URL |
| `NO_COLOR` | Disable color output |

## Piping & Scripting

```bash
# Pipe message from stdin
echo "Hello!" | weside chat nox

# JSON output for scripting
weside companions list --json | jq '.[0].name'

# Use in CI with env token
WESIDE_TOKEN=xxx weside companions list --json
```

## Development

```bash
# Build
make build

# Test
make test

# Lint
make lint

# Format
make fmt

# Security check
make security
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
