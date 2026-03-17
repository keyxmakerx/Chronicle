# System Adapter Architecture

System adapters map between Foundry VTT Actor data and Chronicle entity `fields_data`.
They enable character sync by translating game-system-specific data paths.

## Adapter Interface

Every adapter (built-in or generic) exposes:

```javascript
{
  systemId: string,           // Chronicle system ID (e.g., "dnd5e", "drawsteel")
  characterTypeSlug: string,  // Entity type slug (e.g., "dnd5e-character")
  actorType: string,          // Foundry Actor type (e.g., "character", "hero")

  toChronicleFields(actor: Actor): object,
  // Reads Foundry Actor system data, returns Chronicle fields_data object.
  // Example: { str: 18, hp_current: 45, level: 5 }

  fromChronicleFields(entity: object): object,
  // Takes Chronicle entity, returns Foundry actor.update() data.
  // Uses dot-notation keys: { "system.abilities.str.value": 18, name: "Aragorn" }
}
```

## Adapter Loading Priority

When `ActorSync.init()` is called with a matched system ID, it loads an adapter:

```
1. Check built-in adapters:
   - "dnd5e"  → import('./adapters/dnd5e-adapter.mjs')
   - "pf2e"   → import('./adapters/pf2e-adapter.mjs')
   (Hardcoded switch in actor-sync.mjs)

2. Fall back to generic adapter:
   → createGenericAdapter(api, systemId)
   → Fetches GET /systems/:id/character-fields
   → Auto-generates toChronicleFields/fromChronicleFields from foundry_path annotations

3. If generic adapter returns null (no fields with foundry_path):
   → Character sync is disabled for this system
```

## How actorType Works

Different game systems define different Actor types in Foundry VTT:

| Game System | Foundry system.id | Actor Type | Chronicle Preset Slug |
|-------------|-------------------|-----------|----------------------|
| D&D 5e | `dnd5e` | `character` | `dnd5e-character` |
| Pathfinder 2e | `pf2e` | `character` | `pf2e-character` |
| Draw Steel | `draw-steel` | `hero` | `drawsteel-character` |

The adapter's `actorType` is used to:
1. **Filter actors** — Only sync actors of the correct type
2. **Create actors** — Set `type` when creating new Actor documents
3. **List actors** — Filter the sync dashboard's character list

The generic adapter reads `actorType` from the API response's `foundry_actor_type` field.
If not specified, it defaults to `"character"`.

## Writing a Built-In Adapter

Create a new file at `scripts/adapters/<system>-adapter.mjs`:

```javascript
/**
 * Chronicle Sync - <System Name> Adapter
 *
 * Maps fields between Foundry VTT <system> Actor data and Chronicle entity
 * fields_data. Used by ActorSync when the matched system is "<systemId>".
 */

/** Chronicle system ID. */
export const systemId = '<chronicle-system-id>';

/** Chronicle entity type slug for characters. */
export const characterTypeSlug = '<system>-character';

/** Foundry VTT actor type. */
export const actorType = 'character'; // or 'hero', 'npc', etc.

/**
 * Extract Chronicle fields from a Foundry Actor.
 * @param {Actor} actor - Foundry Actor document.
 * @returns {object} Chronicle fields_data.
 */
export function toChronicleFields(actor) {
  const sys = actor.system || {};
  return {
    // Map each Chronicle field key to its Foundry system data path:
    field_key: sys.path?.to?.value ?? null,
  };
}

/**
 * Convert Chronicle fields to a Foundry Actor update.
 * @param {object} entity - Chronicle entity with fields_data.
 * @returns {object} Foundry update data (dot-notation keys).
 */
export function fromChronicleFields(entity) {
  const f = entity.fields_data || {};
  const update = {};

  // Only write non-null values to avoid overwriting Foundry calculations:
  if (f.field_key != null) update['system.path.to.value'] = Number(f.field_key);

  // Name syncs at document level:
  if (entity.name) update.name = entity.name;

  return update;
}
```

Then add the import to the switch in `actor-sync.mjs`:

```javascript
async _loadAdapter(systemId) {
  switch (systemId) {
    case 'dnd5e':
      return import('./adapters/dnd5e-adapter.mjs');
    case 'pathfinder2e':
      return import('./adapters/pf2e-adapter.mjs');
    case 'newsystem':                                    // ADD THIS
      return import('./adapters/newsystem-adapter.mjs');  // ADD THIS
    default:
      return createGenericAdapter(this.api, systemId);
  }
}
```

## Generic Adapter Internals

The generic adapter requires NO code changes. It works for any system whose
manifest includes `foundry_path` annotations on character fields.

### How It Works

1. **Fetch field definitions:**
   ```
   GET /systems/<systemId>/character-fields
   ```

2. **Filter fields with `foundry_path`:**
   Only fields that have a non-empty `foundry_path` are mapped.

3. **Generate `toChronicleFields(actor)`:**
   For each mapped field, reads `actor.<foundry_path>` using dot-notation traversal.

4. **Generate `fromChronicleFields(entity)`:**
   For each writable field (`foundry_writable !== false`), writes `entity.fields_data[key]`
   to the `foundry_path` using dot-notation keys for `actor.update()`.

5. **Type casting:**
   Fields with `type: "number"` are cast via `Number()` before writing to Foundry.

### Example: Draw Steel

The Draw Steel manifest defines:
```json
{
  "key": "might",
  "label": "Might",
  "type": "number",
  "foundry_path": "system.characteristics.might.value",
  "foundry_writable": true
}
```

Generic adapter generates:
```javascript
// toChronicleFields:
result.might = actor.system.characteristics.might.value;

// fromChronicleFields:
update['system.characteristics.might.value'] = Number(entity.fields_data.might);
```

### When to Use Generic vs Built-In

**Use the generic adapter** (recommended for new systems):
- System manifest has `foundry_path` annotations on all character fields
- Field mappings are straightforward (one Chronicle field = one Foundry path)
- No special transformation logic needed

**Write a built-in adapter** when:
- Fields require complex transformations (e.g., combining multiple Foundry paths)
- You need to read from paths that can't be expressed as simple dot-notation
- Performance matters (avoids an extra API call to fetch field definitions)
- The system is widely used and worth maintaining a hand-optimized adapter

## D&D 5e Adapter Reference

The `dnd5e-adapter.mjs` maps 15 fields:

| Chronicle Key | Foundry Path | Writable | Notes |
|---------------|-------------|----------|-------|
| `str` | `system.abilities.str.value` | Yes | Ability score |
| `dex` | `system.abilities.dex.value` | Yes | |
| `con` | `system.abilities.con.value` | Yes | |
| `int` | `system.abilities.int.value` | Yes | |
| `wis` | `system.abilities.wis.value` | Yes | |
| `cha` | `system.abilities.cha.value` | Yes | |
| `hp_current` | `system.attributes.hp.value` | Yes | Current HP |
| `hp_max` | `system.attributes.hp.max` | Yes | Max HP |
| `ac` | `system.attributes.ac.value` | No | Calculated |
| `speed` | `system.attributes.movement.walk` | No | |
| `level` | `system.details.level` | No | |
| `class` | `system.details.class` | No | |
| `race` | `system.details.race` | No | |
| `alignment` | `system.details.alignment` | No | |
| `proficiency_bonus` | `system.attributes.prof` | No | Calculated |

Note: The built-in adapter only writes ability scores and HP back to Foundry.
All other fields are read-only (pulled from Foundry to Chronicle). This prevents
overwriting Foundry's calculated values.
