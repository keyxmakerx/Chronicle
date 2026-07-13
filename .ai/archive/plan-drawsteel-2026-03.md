# Draw Steel System Module — Full Implementation Plan

## Overview

Make the Draw Steel game system a complete, self-contained, "containerized" system module
for Chronicle — including website reference content, character/entity presets with full
Foundry VTT field mappings, and all necessary infrastructure fixes. Everything should be
dynamic and modular: a system manifest drives all behavior, and the Foundry module reads
field definitions from the API. No hard-coded adapters.

---

## Phase 1: Infrastructure Fixes (Bugs & Architecture)

### 1A. CORS — Admin-managed origin whitelist

**Problem:** `AllowedOrigins` in `app.go:137` is hardcoded to only `Config.BaseURL`. Foundry
VTT (on a different port/origin) gets CORS-blocked.

**Solution:** Store allowed CORS origins in the `site_settings` table (key:
`cors.allowed_origins`). Admin can manage via the admin panel. The CORS middleware reads
from DB on startup and reloads on change.

**Files to modify:**
- `internal/plugins/settings/model.go` — Add `KeyCORSAllowedOrigins` constant
- `internal/plugins/settings/service.go` — Add `GetCORSOrigins()` / `UpdateCORSOrigins()` methods
- `internal/plugins/settings/repository.go` — SQL for reading/writing the setting
- `internal/plugins/settings/handler.go` — HTTP handler for CORS settings form
- `internal/plugins/settings/routes.go` — Register new routes
- `internal/plugins/admin/handler.go` — Add API settings page handler
- `internal/plugins/admin/api_settings.templ` — **New file**: Admin API settings page with
  CORS origin whitelist management (add/remove origins)
- `internal/middleware/cors.go` — Accept a `OriginProvider` function that returns current
  origins list (called per-request, cached briefly)
- `internal/app/app.go` — Wire up CORS middleware with DB-backed origin provider that
  includes `Config.BaseURL` + DB origins

### 1B. Foundry actor type — make it dynamic via manifest

**Problem:** `actor-sync.mjs` hardcodes `actor.type !== 'character'` in multiple places.
Draw Steel uses actor type `'hero'`, not `'character'`. This must be dynamic.

**Solution:** Add `foundry_actor_type` field to manifest `EntityPresetDef` and export it
via the character-fields API. The generic adapter and actor-sync use it instead of
hardcoding `'character'`.

**Files to modify:**
- `internal/systems/manifest.go`:
  - Add `FoundryActorType string` to `EntityPresetDef` struct (`json:"foundry_actor_type,omitempty"`)
  - Add `FoundryActorType string` to `CharacterFieldsResponse` struct
  - Include it in `CharacterFieldsForAPI()` and `ItemFieldsForAPI()` output
- `internal/systems/drawsteel/manifest.json` — Add `"foundry_actor_type": "hero"` to
  the character preset
- `internal/systems/dnd5e/manifest.json` — Add `"foundry_actor_type": "character"` to
  character preset (explicit)
- `internal/systems/pathfinder2e/manifest.json` — Add `"foundry_actor_type": "character"`
- `foundry-module/scripts/adapters/generic-adapter.mjs` — Read `foundry_actor_type` from
  API response, expose it on the returned adapter object, default to `'character'`
- `foundry-module/scripts/actor-sync.mjs` — Replace all `actor.type !== 'character'`
  checks with `actor.type !== this._adapter.actorType`, where `actorType` comes from the
  adapter. Also fix `Actor.create()` to use the correct type. Fallback to `'character'`.

### 1C. SYSTEM_MAP_FALLBACK key fix

**Problem:** `sync-manager.mjs` line 21 has `drawsteel: 'drawsteel'` but the Foundry
system ID is `'draw-steel'` (hyphenated), not `'drawsteel'`.

**Fix:** Change the key to `'draw-steel': 'drawsteel'`.

**File:** `foundry-module/scripts/sync-manager.mjs` line 21

---

## Phase 2: Draw Steel Manifest Expansion

### 2A. Character preset — full field set with Foundry paths

Expand the `drawsteel-character` entity preset from 4 fields to a complete Draw Steel
character sheet. All fields that map to Foundry data get `foundry_path` annotations.

**Fields to add (with Foundry paths):**

| Key | Label | Type | Foundry Path | Writable |
|-----|-------|------|-------------|----------|
| class | Class | string | (none — derived from class item) | — |
| subclass | Subclass | string | (none — derived from subclass item) | — |
| level | Level | number | system.details.level (if exists) | yes |
| ancestry | Ancestry | string | (none — derived from ancestry item) | — |
| career | Career | string | (none — derived from career item) | — |
| might | Might | number | system.characteristics.might.value | yes |
| agility | Agility | number | system.characteristics.agility.value | yes |
| reason | Reason | number | system.characteristics.reason.value | yes |
| intuition | Intuition | number | system.characteristics.intuition.value | yes |
| presence | Presence | number | system.characteristics.presence.value | yes |
| stamina_current | Current Stamina | number | system.stamina.value | yes |
| stamina_max | Max Stamina | number | system.stamina.max | no (derived) |
| stamina_temp | Temporary Stamina | number | system.stamina.temporary | yes |
| recoveries_current | Recoveries | number | system.recoveries.value | yes |
| recoveries_max | Max Recoveries | number | system.recoveries.max | no (derived) |
| heroic_resource | Heroic Resource | number | system.hero.primary.value | yes |
| surges | Surges | number | system.hero.surges | yes |
| victories | Victories | number | system.hero.victories | yes |
| xp | Experience | number | system.hero.xp | yes |
| renown | Renown | number | system.hero.renown | yes |
| wealth | Wealth | number | system.hero.wealth | yes |
| speed | Speed | number | system.movement.value | no (derived) |
| stability | Stability | number | system.combat.stability | no (derived) |
| size | Size | number | system.combat.size.value | no |

**Sections:** Group characteristics under "Characteristics", stamina/recoveries under
"Resources", combat stats under "Combat", victories/xp/renown under "Progression".

**File:** `internal/systems/drawsteel/manifest.json`

### 2B. Creature preset — add with proper Draw Steel terminology

Add a `drawsteel-creature` entity preset for NPCs/monsters.

**Fields:**
- level (number), role (string: "Ambusher", "Artillery", "Brute", etc.),
  ev (number, Encounter Value), role_type (string: "Minion", "Standard", "Elite", "Solo"),
  stamina (number), speed (number), stability (number), size (string),
  free_strike_damage (number)

### 2C. Kit preset — add Foundry paths

The existing kit preset fields are fine but need `foundry_path` annotations where applicable.
Kits are items in Foundry Draw Steel, so these map to item system data.

### 2D. Update manifest status

Change `"status": "coming_soon"` to `"status": "available"`.

---

## Phase 3: Reference Data Foundation

### 3A. Create data directory structure

Create `internal/systems/drawsteel/data/` with JSON files for each category.

### 3B. Seed initial reference data

Using correct Draw Steel terminology and CC-BY-4.0 content:

- `data/abilities.json` — Seed with 5-10 representative abilities across different classes
  (e.g., a Tactician, Shadow, Fury, and Elementalist ability each). Fields: name, class,
  level, type (action/maneuver/triggered), keywords, description.

- `data/creatures.json` — Seed with 5-8 creatures covering different roles and levels.
  Fields: name, level, role, role_type, ev, stamina, speed, description.

- `data/ancestries.json` — Seed with the core ancestries (Human, Dwarf, Elf (Wode/High),
  Orc, Hakaan, Memonek, Revenant, Polder, Dragon Knight). Fields: name, size, speed,
  description.

---

## Phase 4: Generic Adapter Enhancement

The user explicitly wants everything dynamic — no hand-written adapters. The generic
adapter must be enhanced to handle system-specific details like actor type.

### 4A. Enhance generic adapter

**File:** `foundry-module/scripts/adapters/generic-adapter.mjs`

- Read `foundry_actor_type` from the API response and include it in the returned object
  as `actorType` (default: `'character'`)
- This is already largely handled by the existing generic adapter; just add the
  `actorType` property

### 4B. Actor sync — use adapter's actorType

**File:** `foundry-module/scripts/actor-sync.mjs`

- `_handleCreateActor`: Replace `actor.type !== 'character'` with
  `actor.type !== (this._adapter.actorType || 'character')`
- `_onCharacterCreated`: Replace `type: 'character'` with
  `type: this._adapter.actorType || 'character'`
- `getSyncedActors`: Replace `.filter(a => a.type === 'character')` with
  `.filter(a => a.type === (this._adapter?.actorType || 'character'))`

---

## Phase 5: Documentation Updates

### 5A. Update `.ai.md`

Update `internal/systems/drawsteel/.ai.md` to reflect completed work — mark handlers,
data files, and routes as done.

### 5B. Update `.ai/status.md`

Add sprint entry documenting the Draw Steel system completion.

### 5C. Update `.ai/todo.md`

Mark Draw Steel tasks as complete, add any follow-up items.

---

## Execution Order

1. Phase 1C (trivial fix — SYSTEM_MAP_FALLBACK key)
2. Phase 1B (foundry_actor_type — manifest struct + API + adapter + actor-sync)
3. Phase 2A-D (manifest expansion — all entity presets with Foundry paths)
4. Phase 3A-B (reference data files)
5. Phase 1A (CORS admin whitelist — biggest infrastructure change)
6. Phase 4A-B (generic adapter enhancement)
7. Phase 5 (documentation)

## Risk Notes

- **Draw Steel Foundry paths may change** — The Draw Steel Foundry system is actively
  developed (v0.11.1). Paths like `system.hero.primary.value` could shift. The generic
  adapter approach mitigates this since path definitions live in the manifest, not in code.
- **Class/subclass/ancestry/career** are item-derived in Foundry — these can't be set via
  `actor.update()` in Foundry, so they're included as read-only Chronicle fields with no
  `foundry_path` (sync is name/description only for these).
- **The `section` field** on FieldDef is used in manifests but not in the Go struct. It's
  preserved through JSON round-tripping. No changes needed.
