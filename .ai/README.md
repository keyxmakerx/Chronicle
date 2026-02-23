# AI Documentation Index

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Map of all AI documentation files. An AI reads this to know     -->
<!--          which file to consult for a given question.                     -->
<!-- Update: When a doc file is added, removed, or its purpose changes.       -->
<!-- ====================================================================== -->

This directory contains all context files for AI coding assistants working on
Chronicle. These files exist so any AI session can pick up work without
re-reading every source file in the project.

## How to Use These Files

1. **Every session:** Read `status.md` first. It tells you what was done last,
   what is in progress, and what to do next.
2. **When planning:** Read `todo.md` for the prioritized backlog.
3. **When coding:** Read `conventions.md` for patterns with code examples.
4. **When working on a module:** Read `internal/modules/<name>/.ai.md` for that
   module's specific docs.
5. **When making design choices:** Read `decisions.md` to see what has already
   been decided and why.

## File Inventory

| File | Category | Purpose | Read When... |
|------|----------|---------|--------------|
| `status.md` | Dynamic | Current state, last session recap, next priorities | Every session start |
| `todo.md` | Dynamic | Prioritized task backlog with completion markers | Planning work |
| `architecture.md` | Semi-static | System design, module map, request flow, dependency graph | Designing new features |
| `conventions.md` | Semi-static | Code patterns with concrete Go/Templ/SQL examples | Writing any code |
| `decisions.md` | Semi-static | Architecture Decision Records (ADRs) with rationale | Making or questioning design choices |
| `tech-stack.md` | Static | Technology versions, configs, "why this tech" notes | Setting up or debugging infrastructure |
| `data-model.md` | Semi-static | Database schema, tables, columns, indexes, relations | Writing queries or migrations |
| `api-routes.md` | Semi-static | Complete route table with handler mappings | Adding or modifying endpoints |
| `glossary.md` | Static | TTRPG and Chronicle-specific terminology | Understanding domain concepts |
| `troubleshooting.md` | Semi-static | Known gotchas and their solutions | Debugging non-obvious issues |
| `roadmap.md` | Semi-static | Competitive analysis (WorldAnvil/Kanka/LegendKeeper), feature brainstorm organized by tier (Core/Plugin/Module/Widget/External), priority phases, Foundry VTT integration plans | Planning features, understanding competitive landscape, checking priorities |

## Category Definitions

- **Static:** Rarely changes. Reference material established once.
- **Semi-static:** Changes when architecture evolves, new patterns are set, or
  new modules are added. Maybe once per sprint.
- **Dynamic:** Changes every session. Status and backlog tracking.

## Templates

The `templates/` subdirectory contains templates for creating new documentation:

- `module-ai.md.tmpl` -- Copy this when creating a new module's `.ai.md` file
- `decision-record.md.tmpl` -- Copy this format when adding a new ADR entry

## Module-Level Documentation

Each module at `internal/modules/<name>/` contains an `.ai.md` file describing
that module's purpose, internal structure, dependencies, routes, business rules,
and current implementation state. These are the files to read when you are about
to work on a specific module.
