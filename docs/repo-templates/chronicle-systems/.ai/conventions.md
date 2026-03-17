# JSON Authoring Conventions

## File Formatting

- **2-space indentation**
- **No trailing commas** (invalid JSON)
- **UTF-8 encoding**
- **LF line endings** (Unix-style)
- **Final newline** at end of file

## Naming Conventions

| Thing | Convention | Example |
|-------|-----------|---------|
| System ID | `lowercase` (no hyphens in Chronicle ID) | `dnd5e`, `drawsteel`, `pathfinder2e` |
| Category slug | `lowercase`, plural | `spells`, `monsters`, `ancestries` |
| Item ID | `lowercase-hyphenated` | `acid-splash`, `dragon-knight` |
| Field key | `snake_case` | `hp_current`, `casting_time`, `stamina_max` |
| Entity preset slug | `<system>-<type>` | `dnd5e-character`, `drawsteel-creature` |
| Data file name | `<category-slug>.json` | `spells.json`, `creatures.json` |

## manifest.json Conventions

### Required Fields
```json
{
  "id": "mysystem",
  "name": "My Game System",
  "version": "1.0.0",
  "api_version": "1",
  "status": "available"
}
```

### System ID Rules
- Must be unique across all systems
- Lowercase, no spaces, no special characters except hyphens
- The ID in manifest.json MUST match the directory name
- Chronicle IDs typically omit hyphens: `drawsteel` not `draw-steel`
- Foundry system IDs may differ: `foundry_system_id: "draw-steel"`

### Version Numbering
- Use semantic versioning: `MAJOR.MINOR.PATCH`
- Bump MAJOR for breaking manifest schema changes
- Bump MINOR for new categories or significant data additions
- Bump PATCH for data corrections and small additions

### License Values
| License | When to Use |
|---------|------------|
| `OGL-1.0a` | D&D SRD content under Open Gaming License |
| `ORC` | Pathfinder content under Open RPG Creative License |
| `CC-BY-4.0` | Content under Creative Commons Attribution |
| `MIT` | For original/homebrew content |

## Data File Conventions

### Item IDs
- Must be unique within the category (not globally)
- Use the item name in `lowercase-hyphenated` form
- Examples: `acid-splash`, `ancient-red-dragon`, `cloak-and-dagger`

### Summaries vs Descriptions
- **`summary`**: One sentence, plain text, for search results and list views. Max ~100 chars.
- **`description`**: Full reference text, supports markdown. Can be multiple paragraphs.
- If content is short, `summary` alone is sufficient. `description` is optional.

### Properties
- Keys MUST match the `key` values in the corresponding `CategoryDef.fields` array
- Values should match the declared `type`:
  - `"string"` → string value
  - `"number"` → numeric value (not stringified)
  - `"list"` → comma-separated string (e.g., `"fire, evocation"`)
  - `"markdown"` → markdown-formatted string
- Only include properties defined in the manifest schema

### Tags
- Lowercase, singular form preferred (e.g., `"cantrip"` not `"Cantrips"`)
- Used for search and filtering
- Include: type, school/category, element, creature type, etc.

### Source Attribution
- ALWAYS include `source` on every item
- Format: `"<Source Name> <Version/Page>"` or just `"<Source Name>"`
- Examples: `"SRD 5.1"`, `"ORC"`, `"Arcana Archive"`, `"Draw Steel Backer Packet"`

## Foundry Path Annotation Conventions

### When to Add `foundry_path`

Add `foundry_path` to entity preset fields (NOT category reference fields) when:
- The field maps to a specific path in Foundry VTT Actor system data
- You know the exact dot-notation path for the target Foundry game system
- The field represents a value that exists on the Foundry Actor

### Path Format
- Always starts with `system.` (referring to `actor.system`)
- Use dot notation for nested objects: `system.abilities.str.value`
- Match the exact path from the Foundry game system's data model

### Research Foundry Paths

To find correct Foundry data paths for a game system:

1. **Active Effects wiki:** Check the game system's GitHub wiki for Active Effects documentation
2. **Console inspection:** In a Foundry world, run `console.log(actor.system)` and explore the data structure
3. **System source code:** Read the game system's `template.json` or `system.json` for the data model
4. **Foundry API docs:** Check `Actor.system` documentation

### When to Set `foundry_writable: false`

Set writable to false for fields that are:
- **Calculated by the game system** (e.g., AC, proficiency bonus, ability modifiers)
- **Derived from other fields** (e.g., max HP from class/level/constitution)
- **Managed by items/features** (e.g., speed from race, class features)

These fields should be READ from Foundry (to display in Chronicle) but never WRITTEN
back (to avoid overwriting the game system's calculations).

## Example: Complete Minimal System

```
systems/mysystem/
  manifest.json
  data/
    abilities.json
```

**manifest.json:**
```json
{
  "id": "mysystem",
  "name": "My Game System",
  "description": "A custom TTRPG system for Chronicle.",
  "version": "1.0.0",
  "author": "Your Name",
  "license": "CC-BY-4.0",
  "icon": "fa-dice-d20",
  "api_version": "1",
  "status": "available",
  "categories": [
    {
      "slug": "abilities",
      "name": "Abilities",
      "icon": "fa-bolt",
      "fields": [
        { "key": "level", "label": "Level", "type": "number" },
        { "key": "type", "label": "Type", "type": "string" }
      ]
    }
  ],
  "entity_presets": [
    {
      "slug": "mysystem-character",
      "name": "My System Character",
      "name_plural": "My System Characters",
      "icon": "fa-user",
      "color": "#7C3AED",
      "category": "character",
      "fields": [
        { "key": "health", "label": "Health", "type": "number" },
        { "key": "strength", "label": "Strength", "type": "number" }
      ]
    }
  ]
}
```

**data/abilities.json:**
```json
[
  {
    "id": "power-strike",
    "name": "Power Strike",
    "summary": "A powerful melee attack that deals extra damage.",
    "description": "Make a melee attack. On a hit, deal an extra 1d6 damage.",
    "properties": {
      "level": 1,
      "type": "Attack"
    },
    "tags": ["attack", "melee"],
    "source": "My System Core Rules"
  }
]
```
