# Foundry Module Coding Conventions

## Language & Module System

- **Pure ES6 JavaScript** — no TypeScript, no JSX, no build step
- **ES modules** — `import`/`export` only, never CommonJS `require()`
- **No external dependencies** — everything uses browser APIs + Foundry's global API
- **No transpilation** — code runs directly in the browser as-is
- **Target:** Modern browsers that Foundry VTT supports (Chrome 90+, Firefox 90+)

## File Organization

- One class/concern per file
- Entry point: `scripts/module.mjs`
- Each sync type gets its own file: `journal-sync.mjs`, `map-sync.mjs`, etc.
- System adapters live in `scripts/adapters/`
- UI templates: `templates/*.hbs` (Handlebars, Foundry's template engine)
- Styles: `styles/*.css`
- Localization: `lang/*.json`

## Naming Conventions

| Thing | Convention | Example |
|-------|-----------|---------|
| Files | `kebab-case.mjs` | `actor-sync.mjs` |
| Classes | `PascalCase` | `SyncManager`, `ChronicleAPI` |
| Functions/methods | `camelCase` | `toChronicleFields()` |
| Private methods | `_camelCase` | `_detectSystem()` |
| Constants | `UPPER_SNAKE_CASE` | `MODULE_ID`, `SYSTEM_MAP_FALLBACK` |
| Settings keys | `camelCase` | `apiUrl`, `syncEnabled` |
| CSS classes | `chronicle-*` prefix | `chronicle-sync-status` |
| i18n keys | `CHRONICLE.Section.Key.Name` | `CHRONICLE.Settings.ApiUrl.Name` |

## Foundry VTT Patterns

### Hooks

```javascript
// One-time initialization
Hooks.once('init', () => { /* register settings */ });
Hooks.once('ready', () => { /* start sync (GM only) */ });

// Ongoing event listeners
Hooks.on('createJournalEntry', (journal, options, userId) => { ... });
Hooks.on('updateActor', (actor, changes, options, userId) => { ... });
```

### Settings

```javascript
// Register (in init hook)
game.settings.register('chronicle-sync', 'settingKey', {
  name: game.i18n.localize('CHRONICLE.Settings.Key.Name'),
  hint: game.i18n.localize('CHRONICLE.Settings.Key.Hint'),
  scope: 'world',      // Per-world, not per-user
  config: true,         // Show in settings dialog (false for internal settings)
  type: String,         // String, Number, Boolean
  default: '',
  requiresReload: true, // Whether changing requires page reload
});

// Read
const value = game.settings.get('chronicle-sync', 'settingKey');

// Write
await game.settings.set('chronicle-sync', 'settingKey', newValue);
```

### ApplicationV2 (Foundry v12+)

```javascript
class MyApp extends foundry.applications.api.ApplicationV2 {
  static DEFAULT_OPTIONS = {
    id: 'my-app',
    classes: ['chronicle-sync'],
    window: { title: 'CHRONICLE.Dashboard.Title', resizable: true },
    position: { width: 800, height: 600 },
  };

  static PARTS = {
    main: { template: 'modules/chronicle-sync/templates/my-template.hbs' }
  };

  async _prepareContext(options) {
    return { /* template data */ };
  }
}
```

### Application v1 (legacy, still used for some UIs)

```javascript
class MyWindow extends Application {
  static get defaultOptions() {
    return foundry.utils.mergeObject(super.defaultOptions, {
      id: 'my-window',
      template: 'modules/chronicle-sync/templates/my-template.hbs',
      width: 400,
      height: 300,
    });
  }

  getData() {
    return { /* template data */ };
  }
}
```

## Error Handling

- **Never throw from hook callbacks** — catch and log errors, continue running
- **API errors** — catch in api-client.mjs, emit to error log, retry if transient
- **WebSocket disconnects** — auto-reconnect with exponential backoff (2s → 30s max)
- **Missing settings** — check `isConfigured()` before starting sync, show status indicator
- **User-facing errors** — use `ui.notifications.warn()` or `ui.notifications.error()`

```javascript
try {
  await this.api.post('/entities', entityData);
} catch (err) {
  console.error('Chronicle: Failed to create entity', err);
  ui.notifications.error('Chronicle: Failed to sync entity');
}
```

## Sync Guard Pattern

**CRITICAL:** Every sync operation that modifies Foundry documents must use a sync guard
to prevent infinite echo loops:

```javascript
// Set flag before applying remote changes
this._chronicleSyncing = true;
try {
  await foundryDocument.update(data);
} finally {
  this._chronicleSyncing = false;
}

// Check flag in hook handlers
Hooks.on('updateSomething', (doc, changes) => {
  if (this._chronicleSyncing) return; // Our own change, skip
  this._pushToChronicle(doc);
});
```

## Localization

All user-visible strings must use Foundry's i18n system:

```javascript
// In JavaScript
const label = game.i18n.localize('CHRONICLE.Settings.ApiUrl.Name');
const formatted = game.i18n.format('CHRONICLE.Status.Connected', { system: 'D&D 5e' });

// In Handlebars templates
{{localize "CHRONICLE.Dashboard.Title"}}
```

Keys are defined in `lang/en.json`:
```json
{
  "CHRONICLE.Settings.ApiUrl.Name": "Chronicle URL",
  "CHRONICLE.Settings.ApiUrl.Hint": "The URL of your Chronicle instance"
}
```

## Logging

Use `console.log/warn/error` with `Chronicle:` prefix for easy filtering:

```javascript
console.log('Chronicle: System matched — Foundry "dnd5e" → Chronicle "dnd5e"');
console.warn('Chronicle: No character fields for system "drawsteel"');
console.error('Chronicle: WebSocket connection failed', error);
```

## Code Style

- **JSDoc comments** on all exported functions and class methods
- **Single-line arrow functions** for simple callbacks
- **Template literals** for string interpolation
- **Optional chaining** (`?.`) and **nullish coalescing** (`??`) preferred
- **No semicolons** — the codebase uses ASI (automatic semicolon insertion) is NOT followed; semicolons ARE used
- **2-space indentation**
- **Single quotes** for strings
