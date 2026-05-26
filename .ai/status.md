# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC — thin index, not a session log                        -->
<!-- Purpose: Cross-cutting project state + index of per-plugin .ai.md files. -->
<!-- Update: When release status / branch state / cross-cutting work changes. -->
<!-- ====================================================================== -->

## For humans

### What this file is

A thin index. It documents Chronicle's current high-level state (release version, active phase, cross-cutting items) and points at where per-plugin status lives — each plugin owns its own `.ai.md` per the convention in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)` + `2026-05-23-c-plugin-isolation-audit.md §2.3`.

### What this file is NOT

It is no longer a chronological session-recap log. Pre-2026-05-23 session recaps (51 numbered entries spanning ~135 KB) live in `.ai/archive/status-2026-04-25-pre-shrink.md`. Going forward, per-session deliverables are tracked by the dispatch workflow (`cordinator/decisions/2026-05-19-dispatch-workflow.md`): one report per dispatch in `cordinator/reports/chronicle/`, plus the PR itself.

If you're an AI session looking for "what shipped last week", read the Cordinator working branch (`claude/setup-working-memory-vROh3`) and grep `reports/chronicle/2026-05-*` by date. If you're looking for plugin-specific architecture, footguns, or recent work on plugin X, read `internal/plugins/<X>/.ai.md`.

## For AI sessions

### Current release + branch state

- **Release line:** 0.0.1 (Release Readiness completed 2026-04-25 — backup scripts + mariadb-client in image + deployment runbook)
- **Active phase:** Phase 4 (post-hygiene + post-security + plugin-isolation arc). See `cordinator/plans/2026-05-21-master-plan.md` for the phase definition.
- **Coordinator working branch (cordinator artifacts):** `claude/setup-working-memory-vROh3`
- **Last cross-cutting decision:** `cordinator/decisions/2026-05-23-plugin-registration.md` (NW-2.2 Chunk A — the lightweight plugin registration model)

### Bootstrap reads for a new session

In order:

1. `cordinator/decisions/2026-05-21-core-tenets.md` — binding tenets every session honors
2. `cordinator/decisions/2026-05-19-dispatch-workflow.md` — how dispatches + status reports flow
3. `cordinator/decisions/2026-05-23-decision-routing.md` — backend-vs-frontend question routing
4. This file (you're here) — high-level state + plugin index
5. `.ai/conventions.md` — code patterns
6. `.ai/architecture.md` — three-tier extension model + request flow
7. The relevant plugin's `.ai.md` if your work is plugin-scoped (see index below)

### Plugin .ai.md index (the canonical per-plugin docs)

#### Plugins (22)

| Plugin | `.ai.md` |
|---|---|
| addons | [internal/plugins/addons/.ai.md](../internal/plugins/addons/.ai.md) |
| admin | [internal/plugins/admin/.ai.md](../internal/plugins/admin/.ai.md) |
| armory | [internal/plugins/armory/.ai.md](../internal/plugins/armory/.ai.md) |
| audit | [internal/plugins/audit/.ai.md](../internal/plugins/audit/.ai.md) |
| auth | [internal/plugins/auth/.ai.md](../internal/plugins/auth/.ai.md) |
| backup | [internal/plugins/backup/.ai.md](../internal/plugins/backup/.ai.md) |
| bestiary | [internal/plugins/bestiary/.ai.md](../internal/plugins/bestiary/.ai.md) |
| calendar | [internal/plugins/calendar/.ai.md](../internal/plugins/calendar/.ai.md) |
| campaigns | [internal/plugins/campaigns/.ai.md](../internal/plugins/campaigns/.ai.md) |
| designlab | [internal/plugins/designlab/.ai.md](../internal/plugins/designlab/.ai.md) |
| entities | [internal/plugins/entities/.ai.md](../internal/plugins/entities/.ai.md) |
| foundry_vtt | [internal/plugins/foundry_vtt/.ai.md](../internal/plugins/foundry_vtt/.ai.md) |
| maps | [internal/plugins/maps/.ai.md](../internal/plugins/maps/.ai.md) |
| media | [internal/plugins/media/.ai.md](../internal/plugins/media/.ai.md) |
| npcs | [internal/plugins/npcs/.ai.md](../internal/plugins/npcs/.ai.md) |
| packages | [internal/plugins/packages/.ai.md](../internal/plugins/packages/.ai.md) |
| restore | [internal/plugins/restore/.ai.md](../internal/plugins/restore/.ai.md) |
| sessions | [internal/plugins/sessions/.ai.md](../internal/plugins/sessions/.ai.md) |
| settings | [internal/plugins/settings/.ai.md](../internal/plugins/settings/.ai.md) |
| smtp | [internal/plugins/smtp/.ai.md](../internal/plugins/smtp/.ai.md) |
| syncapi | [internal/plugins/syncapi/.ai.md](../internal/plugins/syncapi/.ai.md) |
| timeline | [internal/plugins/timeline/.ai.md](../internal/plugins/timeline/.ai.md) |

#### Widgets (9)

| Widget | `.ai.md` |
|---|---|
| attributes | [internal/widgets/attributes/.ai.md](../internal/widgets/attributes/.ai.md) |
| editor | [internal/widgets/editor/.ai.md](../internal/widgets/editor/.ai.md) |
| entity_notes | [internal/widgets/entity_notes/.ai.md](../internal/widgets/entity_notes/.ai.md) |
| mentions | [internal/widgets/mentions/.ai.md](../internal/widgets/mentions/.ai.md) |
| notes | [internal/widgets/notes/.ai.md](../internal/widgets/notes/.ai.md) |
| posts | [internal/widgets/posts/.ai.md](../internal/widgets/posts/.ai.md) |
| relations | [internal/widgets/relations/.ai.md](../internal/widgets/relations/.ai.md) |
| tags | [internal/widgets/tags/.ai.md](../internal/widgets/tags/.ai.md) |
| title | [internal/widgets/title/.ai.md](../internal/widgets/title/.ai.md) |

### Cross-cutting state (not plugin-scoped)

#### Active arc: NW-2.2 plugin-isolation refactor

Per `cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md` §3 (7 chunks A-G):

| Chunk | What | Status |
|---|---|---|
| A | Lightweight PluginRegistration registry | ✅ shipped PR #334 |
| B | Magic-string consolidation | ✅ shipped PR #332 |
| C | Cross-plugin import discipline docs | ✅ shipped PR #333 |
| D | Plugin-specific UI back into owning plugin | open |
| E | Per-plugin .ai.md split + status shrink | (this chunk) |
| F | Per-plugin static-asset ownership | open — unblocked by A |
| G | Packages plugin rendering refactor | open |

#### Other open work

- C-SEC Chunk 2 (Phase 2B wire-contract + rate-limit pin) — Wave 2 residual
- C-SEC Chunk 7 (AST sanitize invariant test) — Wave 2 residual
- Plugin Host interface design pass — deferred from Chunk A; tracked in `cordinator/decisions/2026-05-23-plugin-registration.md`

### Archive

`.ai/archive/` holds historical docs that have served their purpose:

- `status-2026-04-25-pre-shrink.md` — the 1198-line chronological session log that lived at `.ai/status.md` until 2026-05-23. Pre-Phase-4 session recaps live here.
- `phase-d-plan.md` — Phase D sprint plan (Phase D shipped)
- `security-audit-2026-03-06.md` — the original security audit (superseded by `cordinator/reports/chronicle/2026-05-22-c-security-audit.md`)
- `plan.md` — Draw Steel system implementation plan (work shipped)

### IMPORTANT RULES (mirrored from CLAUDE.md)

Per `cordinator/decisions/2026-05-19-dispatch-workflow.md`:

1. Session-work deliverables → committed PR on the target repo + a Cordinator status report (`reports/chronicle/YYYY-MM-DD-<dispatch>.md` on `claude/setup-working-memory-vROh3`).
2. Plugin-scoped status updates → append to the owning plugin's `.ai.md` "Recent Work" section. Don't bloat this file.
3. Cross-cutting decisions → new file in `cordinator/decisions/` + cite from code.
4. This file's "Cross-cutting state" section gets updated when an arc advances or a release ships.
