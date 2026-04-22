---
description: Git workflow and release process for weside CLI
---

# Git Workflow

## Branch Protection

`main` is protected. All changes require a Pull Request with passing CI.

Exception: Initial setup commits (repo bootstrap) may go direct to main.

## Branch Format

`<type>/WA-XXX-short-description`

Types: `feat`, `fix`, `docs`, `ci`, `test`, `chore`, `refactor`

## Commit Format

Conventional Commits (enforced by pre-commit hook):

```
<type>(<scope>): <subject>

<body>

WA-XXX
```

## Release Process

1. Merge all changes to `main`
2. `git tag vX.Y.Z && git push origin vX.Y.Z`
3. GoReleaser builds binaries + updates Homebrew tap automatically
4. Verify: `gh release view vX.Y.Z -R weside-ai/weside-cli`

## Pre-commit Hooks

Installed hooks (via `.pre-commit-config.yaml`):
- `gofumpt` — format check
- `go-build-repo-mod` — compile check
- `go-test-repo-mod` — run tests
- `trailing-whitespace` + `end-of-file-fixer`
- `conventional-pre-commit` — commit message format

## CI Pipeline

| Job | What | Required for merge |
|-----|------|-------------------|
| `lint` | golangci-lint v2 | Yes |
| `test` | go test -race + coverage | Yes |
| `build` | Cross-compile 5 platforms | Yes (linux/amd64) |
| `security` | govulncheck | No (continue-on-error) |
