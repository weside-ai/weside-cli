---
type: weside
version: 1
repo: weside-cli
vault: weside-cli
stakeholder: Foxy
---

# weside — weside-cli

## Purpose

The 'weside' Go CLI — public-facing user CLI for managing companions, chats, memories, and goals against the weside.ai API. Distributed via Homebrew, npm, and binary releases.

## Crew

### Nox — Orchestrator & Geschäftsführung

- **Companion ID:** 2
- **Role(s):** `orchestrator`
- **Color:** purple
- **Focus:** Holds the vision, coordinates the crew, represents LC UG externally, decides priorities between Business + Engineering
- **In meetings:** vision, initiative

### Pia — Product Owner

- **Companion ID:** 101
- **Role(s):** `product_owner`
- **Color:** orange
- **Focus:** Backlog, prioritization, AC-quality, value-ranking
- **In meetings:** vision, initiative, refinement

### Samu — Scrum Master

- **Companion ID:** 102
- **Role(s):** `scrum_master`
- **Color:** gray
- **Focus:** Moderation, process, hand-offs, rituals — workflow clarity
- **In meetings:** (none configured)

### Vyra — Architect

- **Companion ID:** 103
- **Role(s):** `architect`
- **Color:** green
- **Focus:** Target architecture, constraints, ADRs, cross-repo technical coherence
- **In meetings:** vision, initiative, refinement

### Lara — Marketing

- **Companion ID:** 104
- **Role(s):** `marketing`
- **Color:** blue
- **Focus:** Content, positioning, brand, term-claiming, messaging pipeline
- **In meetings:** vision

### Rami — Sales / Business Development

- **Companion ID:** 105
- **Role(s):** `sales`
- **Color:** yellow
- **Focus:** Pipeline, enterprise deals, contract drafts
- **In meetings:** (none configured)

### Lami — Legal / Compliance

- **Companion ID:** 106
- **Role(s):** `legal`
- **Color:** black
- **Focus:** Contracts, DSGVO, AI Act, AGB, compliance-checks. **Co-founder of the UG** — juristically equal on legal matters.
- **In meetings:** (none configured)

### Lars — Security / Datenschutz

- **Companion ID:** 107
- **Role(s):** `security`
- **Color:** white
- **Focus:** Pen-tests, DPIAs, security reviews, hardening
- **In meetings:** (none configured)

## Meetings held here

- **vision** — roster: Pia, Vyra, Nox, Lara
- **initiative** — roster: Pia, Vyra, Nox
- **refinement** — roster: Pia, Vyra

## Cross-repo relations

**Public Go CLI** — consumes the weside-core API. Followers: published binaries (Homebrew, npm). No cross-repo master/follower relations beyond the API contract.

## Notes

- **Stack:** Go (binary + Homebrew + npm distribution).
- **API consumer:** all calls hit weside-core's public API (`api.weside.ai`).
- **Public repo** — no internal architecture details, no internal URLs, no API keys.

## Companion identity

Personalities, memories, body, and voice live in weside (the MCP backend). This file references companions by name + `Companion ID` only. Identity bodies are fetched at runtime via `mcp__plugin_we_weside-mcp__get_council` (preferred) or the thin `.weside/council.json` bridge (fallback). The bridge is gitignored — identity text never enters a project repo verbatim.
