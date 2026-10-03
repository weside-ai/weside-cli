---
description: Git workflow for weside CLI
---

# Git Workflow

## Branch Protection

`main` is protected. All changes require a Pull Request with passing CI.

## Branch Format

`<type>/WA-XXX-short-description` (without a ticket: `<type>/short-description`)

Types: `feat`, `fix`, `docs`, `ci`, `test`, `chore`, `refactor`

## Commit Format

Conventional Commits (enforced by pre-commit hook):

```
<type>(<scope>): <subject>

<body>

WA-XXX
```

`WA-XXX` rule: `AGENTS.md` § Git Workflow.

## Pre-commit Hooks

Installed hooks (via `.pre-commit-config.yaml`):
- `go-fumpt` — format check
- `go-build-repo-mod` — compile check
- `go-test-repo-mod` — run tests
- `trailing-whitespace`, `end-of-file-fixer`, `check-yaml`, `check-json`
- `conventional-pre-commit` — commit message format

## CI Pipeline

| Job | What | Required for merge |
|-----|------|-------------------|
| `lint` | golangci-lint v2 | Yes |
| `test` | go test -race + coverage | Yes |
| `build` | Cross-compile 5 platforms | Yes (linux/amd64) |
| `security` | govulncheck | **Yes** — blocking, no continue-on-error |
| `claude-review` | Claude Code Review (`claude-code-review.yml`) | Yes |

A vulnerability in a *called* code path fails the PR. Fix = bump the dependency
(`go get <mod>@<fixed>` + `go mod tidy`), then verify with `make security`.
