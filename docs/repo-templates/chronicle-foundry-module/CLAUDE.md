# Chronicle Sync - Foundry VTT Module

## What This Is

A Foundry VTT module (v12/v13) that provides bidirectional real-time sync between
a Foundry VTT game world and a Chronicle worldbuilding instance. Pure ES6 JavaScript,
no build step, no transpilation.

## What This Is NOT

- This is NOT the Chronicle server application (that's the Chronicle repo)
- This is NOT the game system data packs (that's chronicle-systems)
- This module CONNECTS TO a running Chronicle instance via REST API + WebSocket
- It does NOT bundle or embed any Chronicle server code

## AI Documentation

All AI context files live in `.ai/`:

| File | When to Read |
|------|-------------|
| `.ai/architecture.md` | Module architecture, data flow, component relationships |
| `.ai/api-contract.md` | **CRITICAL** — Every Chronicle API endpoint this module calls |
| `.ai/adapters.md` | System adapter architecture for character sync |
| `.ai/conventions.md` | Foundry VTT coding patterns and style conventions |

## Quick Reference

```bash
# No build step needed. To test:
# 1. Symlink or copy this directory into Foundry's Data/modules/chronicle-sync/
# 2. Enable the module in a Foundry world
# 3. Configure Chronicle URL, API key, and campaign ID in module settings
```

## File Structure

| File | Lines | Purpose |
|------|-------|---------|
| `module.json` | ~57 | Foundry manifest: version, compatibility, entry points, optional deps |
| `scripts/module.mjs` | ~194 | Entry point. Registers settings (init), starts SyncManager (ready), adds UI |
| `scripts/settings.mjs` | ~294 | 18 Foundry world-scoped settings (URL, key, campaign, toggles, config) |
| `scripts/api-client.mjs` | ~612 | REST + WebSocket client. Bearer auth, auto-reconnect, message queue, health metrics |
| `scripts/sync-manager.mjs` | ~390 | Orchestrator. System detection, initial sync, WS message routing, activity log |
| `scripts/journal-sync.mjs` | ~662 | Entity <-> JournalEntry. Monk's Enhanced Journal support, permission mapping |
| `scripts/map-sync.mjs` | ~1104 | Drawings/tokens/fog. Percentage<->pixel coordinate conversion, scene linking |
| `scripts/calendar-sync.mjs` | ~717 | Calendar adapter pattern. Calendaria + SimpleCalendar. 0/1-indexed dates |
| `scripts/actor-sync.mjs` | ~602 | Actor <-> character entity. System adapter loading, Foundry hook registration |
| `scripts/item-sync.mjs` | ~436 | Item <-> inventory relations |
| `scripts/shop-widget.mjs` | ~307 | Shop inventory UI (Application v1). Context menu, drag-to-character |
| `scripts/sync-dashboard.mjs` | ~1431 | 6-tab ApplicationV2 dashboard. Pull/push, metrics, configuration |
| `scripts/adapters/dnd5e-adapter.mjs` | ~91 | D&D 5e: 15 fields (abilities, HP, AC, speed, level, class, race) |
| `scripts/adapters/pf2e-adapter.mjs` | ~91 | PF2e: 15 fields (mostly read-only from Foundry) |
| `scripts/adapters/generic-adapter.mjs` | ~133 | API-driven: auto-generates mappings from foundry_path annotations |
| `templates/sync-dashboard.hbs` | - | 6-tab Handlebars template (Config, Entities, Shops, Maps, Characters, Calendar, Status) |
| `templates/shop-window.hbs` | - | Shop inventory display |
| `styles/chronicle-sync.css` | - | Status indicators, shop window styles |
| `lang/en.json` | ~150 keys | English localization for all UI strings |

## Key Conventions

- **ES6 modules only** — `import`/`export`, no CommonJS, no bundling
- **No external dependencies** — everything is vanilla JS + Foundry's API
- **GM-only sync** — SyncManager only starts for the GM user (`game.user.isGM`)
- **Sync guards** — Every sync operation sets a `_chronicleSyncing` flag to prevent echo loops
- **Error resilience** — API client auto-reconnects WebSocket, queues failed messages, tracks health metrics
- **Foundry patterns** — Use `Hooks.on/once`, `game.settings`, `ApplicationV2`, `game.socket`

## Important Rules

1. **NEVER break backwards compatibility** with the Chronicle REST API contract
2. **ALWAYS use sync guards** (`_chronicleSyncing` flag) when applying remote changes to prevent loops
3. **NEVER hardcode system-specific logic** outside of adapter files
4. **ALL user-visible strings** must go through `game.i18n.localize()` with keys in `lang/en.json`
5. **ALWAYS handle WebSocket disconnection gracefully** — queue messages, auto-reconnect
6. Character sync must work with ANY game system via the generic adapter — built-in adapters are optimizations only
7. The module must function even if some Chronicle features are disabled (calendar, maps, etc.)
