# Instruction audit weside-cli — 2026-10-02

Target claude-opus-5-5 / medium, Claude Code 2.1.288, we 7.3.0.

### Gate

```json
[
  {
    "severity": "error",
    "check": "instruction-file-lines",
    "path": "AGENTS.md",
    "message": "213 lines > 200 \u2014 move part-of-the-codebase detail into path-scoped rules"
  }
]
```

### /doctor prompt-audit

## Prompt-Audit: Claude-Code-Konfiguration für weside-cli

### Context

Ziel ist ein Audit der Instruktionsdateien, die in Sessions in `~/weside/weside-cli` geladen werden. Gesucht werden veraltete Muster, also Text für ältere Modelle, überholte Fakten und Dateien, die sich widersprechen. Das Ergebnis sind ein Report und ein vorgeschlagener Diff.

**Wenn du diesen Plan freigibst, wird kein Hunk angewendet.** Jeder Hunk ist ein Vorschlag, den du einzeln übernimmst. Hunks in `~/AGENTS.md` wirken auf alle Projekte in deinem Home-Verzeichnis. Hunks in `~/weside/AGENTS.md` wirken auf alle weside-Repos.

#### Annahmen (Step 0)

- **Target-Modell:** `claude-opus-5-5`. Das ist das Modell dieser Session. Keine geprüfte Datei pinnt ein eigenes Modell.
- **Scope, geprüft:**
  - `weside-cli/AGENTS.md`
  - `.claude/rules/git-workflow.md`
  - `.claude/rules/go-patterns.md` (path-scoped auf `**/*.go`)
  - `~/weside/AGENTS.md` (Ancestor)
  - `~/AGENTS.md` (Ancestor)
  - `~/.claude/CLAUDE.md`: 0 Byte, ohne Inhalt
  - `~/.claude/commands/use-account.md`
- **Scope, ohne Treffer im Dateisystem:**
  - kein `CLAUDE.md` und kein `CLAUDE.local.md` im Projekt oder in Ancestor-Pfaden
  - kein `.claude/CLAUDE.md` und kein `.claude/AGENTS.md`
  - keine Projekt-Skills, -Commands, -Agents oder -Output-Styles
  - unter `~/.claude` keine Rules, Skills, Agents oder Output-Styles
  - keine `@`-Imports
  - keine Managed Policy, `/etc/claude-code` existiert nicht
- **Plugins, nur als Report:** `we` 7.3.0, `codex` 1.0.6, `claude-security` 0.12.0, `frontend-design`, `sentry` 1.4.0. Das sind die Plugins, deren Skills und Agents in dieser Session erscheinen. Ob die übrigen installierten Plugins aktiviert sind, steht in Settings-Dateien, und die habe ich nicht gelesen. Deshalb behandle ich sie als nicht geladen.
- **Nicht gelesen, wie vorgegeben:** Settings-Dateien, `.mcp.json`, `~/.claude.json`.

### Zusammenfassung

Die drei wichtigsten Befunde:

1. `.claude/rules/git-workflow.md` beschreibt den Release-Prozess veraltet. Der npm-Publish-Workflow fehlt, und das Tag ist ein lightweight Tag. Die Regel lädt immer zusammen mit `AGENTS.md`, und `AGENTS.md` beschreibt den Prozess aktuell.
2. Der Footer von `AGENTS.md` nennt `Last Updated: 2026-09-25`. Git zeigt einen späteren Commit vom 2026-10-02.
3. Mehrere Stellen in drei Dateien sind als Historie oder relativ zu früheren Zuständen formuliert. Die aktuelle Regel steckt darin, aber eingebettet in Ticketnummern und alte Werte.

Befunde pro Gruppe:

| Gruppe | Befunde |
|---|---|
| 1 (Prompt-Text) | 2 |
| 2 (Konfigurationsdateien) | 8 |
| 3 (Tool-Beschreibungen) | nicht anwendbar |
| 4 (Request-Konfiguration) | nicht anwendbar, die Agent-Roster-Prüfung ergibt 0 Befunde |

Plugins: 2 Hinweise, nur als Report.

### Audit-Report

| # | Ort | Evidenz | Pattern | Warum veraltet | Konfidenz | Aktion |
|---|---|---|---|---|---|---|
| 1 | `AGENTS.md:212-213` | `**Version:** 2.7` / `**Last Updated:** 2026-09-25 (added Security section)` | G2 Volatile specifics | `git log -1 -- AGENTS.md` liefert 2026-10-02 (f37491e). Git führt dieses Datum ohnehin. | Hoch | `remove` (nur Vorschlag, G2) |
| 2 | `.claude/rules/git-workflow.md:31-36` | `git tag vX.Y.Z && git push`, `GoReleaser builds binaries + updates Homebrew tap` | G2 Widerspruch zwischen Dateien | Laut blame stammt die Stelle vom 2026-04-02. `AGENTS.md` ist neuer (ab 2026-07-25) und beschreibt zwei Workflows (`Release` und `npm Publish`), ein `git tag -a` und eine Prüfung per `npm view`. Die Regel hat keine `paths:`-Frontmatter und lädt immer mit `AGENTS.md`. | Hoch | `rewrite` auf einen Verweis (nur Vorschlag, G2) |
| 3 | `AGENTS.md:110` | `(v1.0.0 was blocked by GO-2026-5970 in golang.org/x/text, reachable via ui.PrintError.)` | G2 History narrative | Die Regel davor („a vulnerability in a called code path fails the PR") trägt die Aussage schon. Die Klammer erzählt einen Vorfall nach. | Mittel | `remove` |
| 4 | `AGENTS.md:188` | `Threads no longer exist — the room is the conversation.` | G1d migration-relative phrasing | „no longer" beschreibt einen Diff zu einem Stand, den das Modell nie gesehen hat. | Mittel | `rewrite` |
| 5 | `AGENTS.md:191` | `— WA-2145 removed this endpoint's cursor along with the app's Verlauf screen, its only pager.` | G2 History narrative | Die Regel lautet: kein Cursor, nie eine Page. Die Ticket-Geschichte trägt nichts dazu bei. | Mittel | `rewrite` |
| 6 | `AGENTS.md:192` | `Replaces the v2 rooms invites … subtree, whose endpoints WA-2235 deletes.` | G1d migration-relative / G2 History | Der Satz beschreibt einen Diff zu einem entfernten Kommando-Baum. | Mittel | `remove` |
| 7 | `~/AGENTS.md:28` (alle Projekte) | `(Port seit 2026-09-14, vorher 22 — …` | G1d migration-relative / G2 Time-sensitive | Der alte Port ist ein Nachweis ohne Nutzen und lädt dazu ein, Port 22 zu versuchen. | Mittel | `rewrite` |
| 8 | `~/AGENTS.md:13` (alle Projekte) | `Drei Teile, zusammen ≤5 Sätze:` | G1f numerische Obergrenze | Laut Guide werden numerische Längengrenzen qualitativ formuliert. Es handelt sich aber um deine eigene Präferenz für Antworten an dich. Du kannst den Hunk deshalb gut begründet ablehnen. | Mittel | `rewrite` |
| 9 | `~/weside/AGENTS.md:26` (alle weside-Repos) | `Staging gilt seit 2026-09-24 als prod-grade` | G2 Time-sensitive | Das Datum hat für die Regel keine Funktion. | Niedrig | `flag` |
| 10 | `.claude/rules/go-patterns.md:51-55` vs. `git-workflow.md:15-29` | `Conventional Commits: feat:, fix:, test:, ci:, docs:, chore:` / `Include WA-XXX … when applicable` | G2 Widerspruch | In go-patterns fehlt `refactor`. Das Ticket ist dort optional, im Template von git-workflow ist es Pflicht. Laut blame stammen beide Stellen aus demselben Commit (b53ea10), die Historie kann sie also nicht ordnen. Du entscheidest, ob WA-XXX Pflicht ist. Danach fällt der Abschnitt in go-patterns entweder weg, oder er wird angeglichen. | Niedrig | `flag` |
| 11 | `.claude/rules/git-workflow.md:7` | `Exception: Initial setup commits (repo bootstrap) may go direct to main.` | G2 Recency trap | Das Repo ist längst gebootstrapt. Die Ausnahme greift nie mehr, lockert aber eine Schutzregel. | Niedrig | `flag` (lockert eine Prohibition) |
| 12 | `~/AGENTS.md:19,29` / `~/weside/AGENTS.md:29` | `(Foxy, 2026-09-30)`, `(Foxy, 2026-09-23)` | G2 History narrative | Diese Attributionen dokumentieren, wer entschieden hat. Laut Sicherheitsabschnitt entscheidet Foxy, die Angabe ist also Kontext. | Niedrig | `flag` |

**Geprüft und sauber.** Diese Stellen fasst der Diff nicht an:

- Alle Pfade und Funktionen, die `AGENTS.md` und `go-patterns.md` nennen, existieren. Geprüft habe ich die `cmd/*`- und `internal/*`-Dateien, `newAuthenticatedClient`/`V2`, `resolveCompanion`, `IsJSON`, `PrintJSON`, `PrintTable`, `truncate`, `finishLogin`, `sendChat`, die Client-Methoden `Subscribe`, `DoRaw*`, `PostStream`, `PutStream` und `PostMultipart`, den Slug `mfa-session-binding-required` sowie `/data-residency/`.
- Die Make-Targets stimmen.
- „6 platforms" stimmt für GoReleaser (3×2), „5 platforms" für die CI-Matrix (ohne windows/arm64). „7 assets" stimmt ebenfalls.
- Laut CI ist `security` blockierend. Die Aussage „AC-6 nicht implementiert" ist korrekt.
- `~/.claude/commands/use-account.md` ist ein exaktes Skript für eine fragile Credential-Operation. Das fällt unter Keep-Liste 3 und bleibt.
- Die Sicherheitsabschnitte und die Dev-Token-Regel sind Policy und Prohibitions mit Begründung. Sie bleiben.
- Die Git-Workflow-Inhalte, die doppelt in `AGENTS.md` und der Rule stehen, stimmen außerhalb des Release-Teils überein. Sie bleiben (Keep-Liste 8).

**Plugins (nur Report, keine Edits):**

- `codex/skills/codex-result-handling/SKILL.md:19` enthält `CRITICAL: After presenting review findings, STOP. Do not make any code changes.` Das ist G1a Pressure language. Ein schlichtes „After presenting findings, stop; do not edit code unless asked" würde reichen.
- `sentry/skills/sentry-instrument/references/sdks/*/ai-monitoring.md` nutzt in Codebeispielen ausgemusterte Modell-IDs wie `claude-3-5-sonnet-20241022` und `claude-sonnet-4-20250514`. Das ist Beispielcode, keine Modell-Instruktion. Wer die Beispiele kopiert, übernimmt aber veraltete IDs.
- `we`, `claude-security` und `frontend-design` liefern keine Treffer auf Signale. Großbuchstaben-Emphase kommt kaum vor, Scratchpad- oder Thinking-Gerüste gibt es keine. `we:dev-high` und `we:dev-medium` unterscheiden sich im Effort und sind deshalb nicht redundant.

### Vorgeschlagener Diff

Jeder Befund hat einen eigenen Hunk. Die Hunks für #1 und #2 sind laut G2 ausdrücklich nur Vorschläge.

#### #1 `AGENTS.md`
```diff
@@ -209,6 +209,3 @@
 (separate, private repo) — not here.
-
----
-
-**Version:** 2.7
-**Last Updated:** 2026-09-25 (added Security section)
```

#### #2 `.claude/rules/git-workflow.md`
```diff
 ## Release Process

-1. Merge all changes to `main`
-2. `git tag vX.Y.Z && git push origin vX.Y.Z`
-3. GoReleaser builds binaries + updates Homebrew tap automatically
-4. Verify: `gh release view vX.Y.Z -R weside-ai/weside-cli`
+See `AGENTS.md` § Release & Install: a `v*` tag triggers both the `Release`
+(GoReleaser + Homebrew) and the `npm Publish` workflow; verify both.
```

#### #3 `AGENTS.md:110`
```diff
-… so a dependency bump is part of the fix. (v1.0.0 was blocked by GO-2026-5970 in `golang.org/x/text`, reachable via `ui.PrintError`.)
+… so a dependency bump is part of the fix.
```

#### #4 `AGENTS.md:188`
```diff
-… A `client_message_id` idempotency key is sent on every POST. Threads no longer exist — the room is the conversation.
+… A `client_message_id` idempotency key is sent on every POST. The room is the conversation; there is no thread concept.
```

#### #5 `AGENTS.md:191`
```diff
-A newest-first slice bounded by `--limit`, **never a page** — WA-2145 removed this endpoint's cursor along with the app's Verlauf screen, its only pager. `--scope last_turn` …
+A newest-first slice bounded by `--limit`, **never a page** — the endpoint has no cursor. `--scope last_turn` …
```

#### #6 `AGENTS.md:192`
```diff
-… so the error cannot tell you which. Replaces the v2 `rooms invites …` subtree, whose endpoints WA-2235 deletes.
+… so the error cannot tell you which.
```

#### #7 `~/AGENTS.md:28` (wirkt auf alle Projekte)
```diff
-`ssh -p <port> <user>@<host>` (Port seit <Datum>, vorher <alter Port> — Direktpfad nur bei Bedarf:
+`ssh -p <port> <user>@<host>` (Direktpfad nur bei Bedarf:
```

#### #8 `~/AGENTS.md:13` (wirkt auf alle Projekte)
```diff
-„status" / „wo stehen wir" will Fortschritt, keine Doku. Drei Teile, zusammen ≤5 Sätze:
+„status" / „wo stehen wir" will Fortschritt, keine Doku. Drei Teile, knapp genug, um sie auf einen Blick zu erfassen:
```

### Verification (Step 7)

- Für #1 bis #7: Du liest jede betroffene Zeile nach dem Edit noch einmal. Mit `grep -n 'WA-2145\|GO-2026-5970\|vorher 22\|Last Updated' AGENTS.md ~/AGENTS.md` prüfst du, dass keine Reste übrig sind. Für #2 bestätigst du, dass `AGENTS.md` § Release & Install weiterhin beide Workflows beschreibt.
- Für #8, die einzige Verhaltensänderung: Du stellst in einer neuen Session zwei- oder dreimal „status" und vergleichst die Länge der Antworten mit dem bisherigen Stand. Werden die Antworten länger, setzt du die Zahl wieder ein.
- Keine Code- oder Testdatei hängt an diesen Texten. Das zeigt ein `grep` auf die entfernten Strings in `cmd/` und `internal/`.
