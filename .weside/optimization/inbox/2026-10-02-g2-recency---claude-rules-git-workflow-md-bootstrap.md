---
key: g2-recency---claude-rules-git-workflow-md-bootstrap
pattern: G2-recency
target: .claude/rules/git-workflow.md#bootstrap
target_repo: this
action: flag
confidence: Low
source: audit
---

`:7` Bootstrap-Ausnahme direkt auf main greift nie mehr, lockert eine Schutzregel.

## Evidence

- 2026-10-02 audit `audits/2026-10-02-weside-cli.md` #11: `:7` Bootstrap-Ausnahme direkt auf main greift nie mehr, lockert eine Schutzregel.
