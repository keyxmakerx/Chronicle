# AI Documentation Index

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Map of all AI documentation files. An AI reads this to know     -->
<!--          which file to consult for a given question.                     -->
<!-- Update: When a doc file is added, removed, or its purpose changes.       -->
<!-- ====================================================================== -->

This directory contains all context files for AI coding assistants working on Chronicle. These files exist so any AI session can pick up work without re-reading every source file in the project.

## How to use these files

1. **Every session:** Read `cordinator/decisions/2026-05-21-core-tenets.md` first (the binding tenets). Then `status.md` (this repo, thin index) for current cross-cutting state.
2. **When working on a plugin:** Read `internal/plugins/<name>/.ai.md` for that plugin's docs. Every plugin owns its own `.ai.md` per the convention in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)` + `2026-05-23-c-plugin-isolation-audit.md §2.3`.
3. **When coding:** Read `conventions.md` for patterns with code examples.
4. **When planning:** Read `todo.md` for the prioritized backlog.
5. **When making design choices:** Read `decisions.md` (this repo) and `cordinator/decisions/` (cross-repo binding decisions).
6. **When looking for session deliverables:** Read `cordinator/reports/chronicle/` — per the dispatch-workflow convention (`cordinator/decisions/2026-05-19-dispatch-workflow.md`), one report per dispatch lives there.

## File inventory

| File | Category | Purpose | Read when... |
|------|----------|---------|--------------|
| `status.md` | Dynamic | Thin index: cross-cutting project state + index of per-plugin `.ai.md` files | Every session start |
| `todo.md` | Dynamic | Prioritized task backlog with completion markers | Planning work |
| `architecture.md` | Semi-static | System design, three-tier extension model, request flow, dependency graph | Designing new features |
| `conventions.md` | Semi-static | Code patterns with concrete Go/Templ/SQL examples + CI guards + cross-plugin import discipline | Writing any code |
| `decisions.md` | Semi-static | Architecture Decision Records (ADRs) with rationale; complements `cordinator/decisions/` | Making or questioning design choices |
| `tech-stack.md` | Static | Technology versions, configs, "why this tech" notes | Setting up or debugging infrastructure |
| `data-model.md` | Semi-static | Database schema, tables, columns, indexes, relations | Writing queries or migrations |
| `api-routes.md` | Semi-static | Complete route table with handler mappings | Adding or modifying endpoints |
| `glossary.md` | Static | TTRPG and Chronicle-specific terminology | Understanding domain concepts |
| `troubleshooting.md` | Semi-static | Known gotchas and their solutions | Debugging non-obvious issues |
| `roadmap.md` | Semi-static | Competitive analysis, feature brainstorm, priority phases | Planning features |
| `audit.md` | Dynamic | Feature parity audit (March 2026) | Fixing quality issues |
| `phases.md` | Dynamic | Phase & sprint plan with execution order | Planning work |
| `obsidian-notes-plan.md` | Semi-static | Obsidian-style notes feature plan | Working on notes features |
| `plugin-development.md` | Semi-static | WASM plugin development guide | Building WASM extensions |
| `design-content-extensions.md` | Semi-static | Content-extension WASM design | WASM runtime work |
| `competitive-gap-analysis.md` | Semi-static | Feature gaps vs competitors | Planning feature parity |
| `security-hardening-plan.md` | Semi-static | Security roadmap | Security review work |
| `designs/wasm-plugin-system.md` | Semi-static | WASM runtime extensibility design | Deep WASM work |

## Category definitions

- **Static:** Rarely changes. Reference material established once.
- **Semi-static:** Changes when architecture evolves, new patterns are set, or new systems are added. Maybe once per sprint.
- **Dynamic:** Changes every session. Status and backlog tracking.

## Where else documentation lives

The `.ai/` tree is AI-process-facing (this index, status, conventions, decisions). Other documentation homes:

- **`cordinator/decisions/`** — cross-repo binding decisions (tenets, dispatch workflow, decision-routing). Every AI session reads these on bootstrap.
- **`cordinator/reports/chronicle/`** — per-dispatch status reports + audit reports. Canonical home for session deliverables.
- **`cordinator/dispatches/chronicle/`** — current and historical dispatch specs.
- **`docs/`** — operator-facing deployment + system docs (deployment.md, api/, bestiary/, system-package-rendering.md, system-plugin-marketplace.md).
- **`internal/plugins/<X>/.ai.md`** — per-plugin context (purpose, key files, routes, footguns, recent work). Each plugin owns its own `.ai.md`; 24 plugins covered.
- **`internal/widgets/<X>/.ai.md`** — per-widget context. 9 of 10 widgets covered (one still lacks its `.ai.md` — see backlog).
- **`internal/systems/<X>/.ai.md`** — per-system content-pack docs.
- **`tools/`** — CI guard scripts (plugin-isolation, templ-drift, decision-citations, wire-contract test).
- **Root `README.md`** — human-facing project overview.
- **Root `CLAUDE.md`** — the AI bootstrap entrypoint (this file is one level deeper; `CLAUDE.md` points here).

## Archive

`.ai/archive/` holds historical docs that have served their purpose:

- `status-2026-04-25-pre-shrink.md` — the 1198-line chronological session log that lived at `.ai/status.md` until 2026-05-23 (Chunk E moved it here; new `status.md` is a thin index per `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)`)
- `phase-d-plan.md` — Phase D sprint plan (Phase D shipped)
- `security-audit-2026-03-06.md` — the original security audit (superseded by `cordinator/reports/chronicle/2026-05-22-c-security-audit.md`)
- `plan.md` — Draw Steel system implementation plan (work shipped)

## Templates

The `templates/` subdirectory contains templates for creating new documentation:

- `module-ai.md.tmpl` — Copy this when creating a new system's `.ai.md` file
- `decision-record.md.tmpl` — Copy this format when adding a new ADR entry

## Extension-level documentation

Each plugin (`internal/plugins/<name>/`), system (`internal/systems/<name>/`), and widget (`internal/widgets/<name>/`) contains an `.ai.md` file describing its purpose, internal structure, dependencies, routes, business rules, and recent work. Per the plugin-isolation audit, this reached uniform coverage in NW-2.2 Chunk E (2026-05-25, then 22/22 + 9/9). Current: **24/24 plugins**, **9/10 widgets** (one widget still lacks its `.ai.md`).
