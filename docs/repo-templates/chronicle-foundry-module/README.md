# Chronicle Sync - Foundry VTT Module

Bidirectional real-time sync between [Chronicle](https://github.com/keyxmakerx/Chronicle) (a self-hosted TTRPG worldbuilding platform) and [Foundry VTT](https://foundryvtt.com).

## Features

- **Journal Sync** — Chronicle entities sync to Foundry journal entries and vice versa. Supports Monk's Enhanced Journal pages.
- **Map Sync** — Drawings, tokens, fog of war, and scene layers sync bidirectionally with Chronicle maps.
- **Calendar Sync** — Calendar events, date advancement, and time tracking sync between Chronicle calendars and Foundry calendar modules (Calendaria, Simple Calendar).
- **Character Sync** — Actor stat blocks sync with Chronicle character entities. Built-in adapters for D&D 5e and Pathfinder 2e; a generic adapter handles any system with `foundry_path` annotations.
- **Item/Shop Sync** — Shop inventory relations sync with Foundry item data. Drag-to-character support.
- **Real-time Updates** — WebSocket connection delivers live updates as changes happen on either side.
- **Sync Dashboard** — 6-tab ApplicationV2 UI for managing sync state, viewing metrics, and performing manual pull/push operations.

## Compatibility

| Component | Version |
|-----------|---------|
| Foundry VTT | v12 - v13 |
| Chronicle | v0.2.0+ |

### Optional Module Support

| Module | Feature |
|--------|---------|
| Monk's Enhanced Journal | Enhanced journal page sync |
| Calendaria | Calendar sync adapter |
| Simple Calendar | Calendar sync adapter |

## Installation

### From Chronicle Instance (Recommended)

1. In your Chronicle admin panel, go to **Admin > Packages**
2. Copy the **Foundry Module Manifest URL**
3. In Foundry VTT: **Add-on Modules > Install Module > Manifest URL**
4. Paste the URL and click Install

### From GitHub Release

1. Copy this manifest URL:
   ```
   https://github.com/keyxmakerx/chronicle-foundry-module/releases/latest/download/module.json
   ```
2. In Foundry VTT: **Add-on Modules > Install Module > Manifest URL**
3. Paste and install

## Configuration

After installation, enable the module in your Foundry world, then configure these settings:

| Setting | Description |
|---------|-------------|
| **Chronicle URL** | Your Chronicle instance URL (e.g., `https://chronicle.example.com`) |
| **API Key** | API key from Chronicle campaign settings (Campaign > API Keys) |
| **Campaign ID** | UUID of the Chronicle campaign to sync with |
| **Sync Enabled** | Master toggle for all sync features |
| **Sync Journals** | Enable journal/entity sync |
| **Sync Maps** | Enable map/drawing/token sync |
| **Sync Calendar** | Enable calendar sync |
| **Sync Characters** | Enable character actor sync (requires matching game system) |

### CORS Configuration

Your Chronicle instance must whitelist your Foundry VTT server's origin. In Chronicle:
**Admin > API Settings > CORS Origin Whitelist** — add your Foundry URL (e.g., `http://localhost:30000`).

## Development

This module is pure ES6 JavaScript with no build step. Files are loaded directly by Foundry VTT.

### File Structure

```
chronicle-foundry-module/
  module.json              # Foundry VTT module manifest
  scripts/
    module.mjs             # Entry point (init + ready hooks)
    settings.mjs           # 18 Foundry world-scoped settings
    api-client.mjs         # REST + WebSocket client with retry/reconnect
    sync-manager.mjs       # Orchestrator: system detection, sync routing
    journal-sync.mjs       # Entity <-> JournalEntry sync
    map-sync.mjs           # Map drawings/tokens/fog sync
    calendar-sync.mjs      # Calendar adapter pattern (Calendaria/SimpleCalendar)
    actor-sync.mjs         # Actor <-> character entity sync
    item-sync.mjs          # Item <-> inventory relation sync
    shop-widget.mjs        # Shop inventory UI (Application v1)
    sync-dashboard.mjs     # 6-tab management dashboard (ApplicationV2)
    adapters/
      dnd5e-adapter.mjs    # D&D 5e field mappings (hand-written)
      pf2e-adapter.mjs     # Pathfinder 2e field mappings (hand-written)
      generic-adapter.mjs  # API-driven field mappings (auto-generated)
  templates/
    sync-dashboard.hbs     # Dashboard Handlebars template
    shop-window.hbs        # Shop inventory template
  styles/
    chronicle-sync.css     # Status indicators + shop styles
  lang/
    en.json                # English localization (~150 keys)
```

### Testing

See `TESTING.md` for the complete E2E testing checklist covering all sync features.

### Creating a Release

1. Update `version` in `module.json`
2. Tag the release: `git tag v0.3.0`
3. Push the tag: `git push origin v0.3.0`
4. GitHub Actions builds `chronicle-sync.zip` and publishes a release
5. The release includes an updated `module.json` with correct download URLs

## License

MIT
