# Chronicle Systems - Game System Content Packs

## What This Is

A collection of JSON data packs that provide reference content (spells, monsters,
items, classes, etc.) for tabletop RPG game systems. Consumed by Chronicle via
its system upload/install feature.

## What This Is NOT

- This is NOT the Chronicle application (that's the Chronicle repo)
- This is NOT the Foundry VTT module (that's chronicle-foundry-module)
- There is NO code to compile or build — just JSON data files
- This repo has NO dependency on Go, Node, or any runtime

## AI Documentation

| File | When to Read |
|------|-------------|
| `.ai/architecture.md` | System pack format spec, manifest schema, API integration |
| `.ai/conventions.md` | JSON authoring conventions and style guide |
| `.ai/creating-a-system.md` | Step-by-step guide for adding a new game system |

## Quick Reference

```bash
# Validate all systems
./scripts/validate.sh

# Validate a single system
./scripts/validate.sh systems/dnd5e

# Package systems into ZIPs
./scripts/package.sh
```

## Directory Structure

```
systems/<system-id>/
  manifest.json          # System metadata, categories, entity presets, field defs
  data/
    <category>.json      # Array of ReferenceItem objects (one file per category)
```

Each system is self-contained. The directory name MUST match the `id` in `manifest.json`.

## Key Conventions

- **JSON formatting:** 2-space indent, no trailing commas
- **Slugs:** `lowercase-hyphenated` (e.g., `dnd5e-character`, `acid-splash`)
- **Field keys:** `snake_case` (e.g., `hp_current`, `casting_time`)
- **Category slugs:** Plural, lowercase (e.g., `spells`, `creatures`, `ancestries`)
- **One file per category:** `data/spells.json` maps to the `spells` category
- **Source attribution:** Every item must have a `source` field (e.g., "SRD 5.1")

## Important Rules

1. **NEVER include copyrighted content** not covered by an open license
2. **ALWAYS set the `license` field** in manifest.json (OGL-1.0a, ORC, CC-BY-4.0, etc.)
3. **ALWAYS include `source`** on every reference item for attribution
4. **Manifest `id` must match the directory name** — `systems/dnd5e/manifest.json` must have `"id": "dnd5e"`
5. **`api_version` must be `"1"`** — this is the Chronicle system framework version
6. **Set `status` to `"available"`** for systems ready to use (or `"coming_soon"` for stubs)
7. **Foundry integration** requires `foundry_system_id` in manifest and `foundry_path` on character fields
8. Data files must parse as valid JSON arrays of ReferenceItem objects
9. Every category in `manifest.json` must have a corresponding `data/<slug>.json` file
