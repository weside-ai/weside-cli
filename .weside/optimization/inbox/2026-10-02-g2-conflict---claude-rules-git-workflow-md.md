---
key: g2-conflict---claude-rules-git-workflow-md
pattern: G2-conflict
target: .claude/rules/git-workflow.md
target_repo: this
action: rewrite
confidence: High
source: audit
---

`:31-36` Release nur GoReleaser + lightweight Tag; `AGENTS.md` beschreibt Release + npm Publish, `git tag -a`, `npm view`. Ersatz: Verweis auf AGENTS.md.

## Evidence

- 2026-10-02 audit `audits/2026-10-02-weside-cli.md` #2: `:31-36` Release nur GoReleaser + lightweight Tag; `AGENTS.md` beschreibt Release + npm Pu
