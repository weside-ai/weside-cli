---
last_optimize: 2026-10-03
---

# Optimization charter

## Goal

Keep the always-loaded instructions (`AGENTS.md` + `.claude/rules/git-workflow.md`) true to the repo and under the budget gate.

## Decisions

- 2026-10-03: `AGENTS.md` stays ≤ 200 lines; Go-editing detail (adding a command, response keys) lives in the path-scoped `.claude/rules/go-patterns.md`.
- 2026-10-03: `WA-XXX` is mandatory in a commit once a ticket exists; bot commits and small `docs:`/`chore:` commits without a ticket omit it.
- 2026-10-03: `security` (govulncheck) becomes a required status check on `main`; the docs follow once the setting is live.

## Findings

- Always-loaded 270 → 222 lines after the 2026-10-03 run; budget gate green.

## Next steps

- Review applied rows on 2026-11-02.
