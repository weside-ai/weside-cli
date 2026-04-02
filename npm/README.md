# weside-cli

The official CLI for [weside.ai](https://weside.ai) — chat with your AI Companions from the terminal.

This npm package downloads the pre-built Go binary for your platform.

## Installation

```bash
npx weside-cli version
```

Or install globally:

```bash
npm install -g weside-cli
weside version
```

## Other install methods

```bash
# Homebrew (macOS/Linux)
brew tap weside-ai/tap && brew install weside-cli

# Shell script
curl -fsSL https://raw.githubusercontent.com/weside-ai/weside-cli/main/scripts/install.sh | sh

# From source
go install github.com/weside-ai/weside-cli@latest
```

## Quick Start

```bash
weside auth login
weside companions list
weside companions select nox
weside chat -m "Hey!"
weside chat --stream -m "Tell me a story"
```

## Documentation

Full docs: [github.com/weside-ai/weside-cli](https://github.com/weside-ai/weside-cli)

## License

Apache 2.0
