# Creating a New Chronicle System Pack

This is a step-by-step guide for creating a new game system content pack
for Chronicle. Follow these steps to add a new TTRPG system.

## Prerequisites

- Familiarity with the game system you're creating a pack for
- Access to legally shareable reference content (SRD, OGL, ORC, CC-BY, etc.)
- A text editor that can validate JSON

## Step 1: Create the Directory

```bash
mkdir -p systems/<system-id>/data
```

Choose a system ID that is:
- Lowercase, no spaces
- Short and recognizable (e.g., `dnd5e`, `drawsteel`, `pathfinder2e`)
- Matching any existing community conventions

## Step 2: Create manifest.json

Create `systems/<system-id>/manifest.json` with the required structure.

### 2a: Basic Metadata

```json
{
  "id": "<system-id>",
  "name": "Full System Name",
  "description": "Short description of the system.",
  "version": "0.1.0",
  "author": "Your Name",
  "license": "CC-BY-4.0",
  "icon": "fa-dice-d20",
  "api_version": "1",
  "status": "available"
}
```

**Choose an icon** from Font Awesome 6 Free:
- `fa-dragon` — Fantasy
- `fa-dice-d20` — Generic TTRPG
- `fa-gun` — Modern/Sci-fi
- `fa-skull` — Horror
- `fa-shield-halved` — Medieval

### 2b: Define Categories

Categories represent types of reference content. Each category gets its own
data file and browsable page in Chronicle.

```json
"categories": [
  {
    "slug": "spells",
    "name": "Spells",
    "icon": "fa-wand-sparkles",
    "fields": [
      { "key": "level", "label": "Level", "type": "number" },
      { "key": "school", "label": "School", "type": "string" },
      { "key": "casting_time", "label": "Casting Time", "type": "string" },
      { "key": "range", "label": "Range", "type": "string" },
      { "key": "duration", "label": "Duration", "type": "string" }
    ]
  },
  {
    "slug": "creatures",
    "name": "Creatures",
    "icon": "fa-paw",
    "fields": [
      { "key": "cr", "label": "Challenge Rating", "type": "string" },
      { "key": "type", "label": "Type", "type": "string" },
      { "key": "size", "label": "Size", "type": "string" }
    ]
  }
]
```

**Category fields** define what properties appear in tooltips and detail views.
They DON'T need Foundry annotations — those are only for entity preset fields.

### 2c: Define Entity Presets

Entity presets create entity types when a campaign enables the system.
At minimum, define a character preset:

```json
"entity_presets": [
  {
    "slug": "<system-id>-character",
    "name": "System Character",
    "name_plural": "System Characters",
    "icon": "fa-user",
    "color": "#7C3AED",
    "category": "character",
    "fields": [
      { "key": "health", "label": "Health", "type": "number" },
      { "key": "strength", "label": "Strength", "type": "number" },
      { "key": "defense", "label": "Defense", "type": "number" }
    ]
  }
]
```

**CRITICAL:** The character preset slug MUST end with `"-character"`.
Chronicle uses this convention to find the character preset programmatically.

### 2d: Add Foundry VTT Integration (Optional)

If you want character sync with Foundry VTT:

1. **Set `foundry_system_id`** on the manifest (the Foundry `game.system.id`):
   ```json
   "foundry_system_id": "dnd5e"
   ```

2. **Set `foundry_actor_type`** on the character preset:
   ```json
   "foundry_actor_type": "character"
   ```
   Common values: `"character"`, `"hero"`, `"npc"`

3. **Add `foundry_path`** to character preset fields:
   ```json
   {
     "key": "strength",
     "label": "Strength",
     "type": "number",
     "foundry_path": "system.abilities.str.value",
     "foundry_writable": true
   }
   ```

4. **Mark read-only fields** with `"foundry_writable": false`:
   ```json
   {
     "key": "ac",
     "label": "Armor Class",
     "type": "number",
     "foundry_path": "system.attributes.ac.value",
     "foundry_writable": false
   }
   ```

### 2e: Add Relation Presets (Optional)

For systems with inventory tracking:

```json
"relation_presets": [
  {
    "slug": "has-item",
    "name": "Has Item",
    "reverse_name": "In Inventory Of",
    "metadata_schema": {
      "quantity": { "type": "number", "default": 1 },
      "equipped": { "type": "boolean", "default": false }
    }
  }
]
```

## Step 3: Create Data Files

For each category in your manifest, create a data file at
`data/<category-slug>.json`.

### Data File Format

```json
[
  {
    "id": "item-slug",
    "name": "Item Name",
    "summary": "One-line description for search results.",
    "description": "Full reference text. Supports **markdown** formatting.\n\nMultiple paragraphs are OK.",
    "properties": {
      "key1": "value matching manifest field type",
      "key2": 42
    },
    "tags": ["tag1", "tag2"],
    "source": "Source Name"
  }
]
```

### Tips for Good Data

- **IDs**: Use the item name in `lowercase-hyphenated` form
- **Summaries**: Keep under 100 characters. Plain text only.
- **Descriptions**: Can be long. Use markdown for formatting.
- **Properties**: Keys MUST match the `key` values in your category's `fields` array
- **Tags**: Lowercase, relevant for search. Include type, school, element, etc.
- **Source**: ALWAYS include for license compliance

## Step 4: Validate

Run the validation script:

```bash
./scripts/validate.sh systems/<system-id>
```

This checks:
- manifest.json parses as valid JSON and has required fields
- All data files parse as valid JSON arrays
- Data file items have required `id` and `name` fields
- Category slugs in manifest have matching data files
- Properties keys match manifest field definitions

## Step 5: Test in Chronicle

1. Package as ZIP:
   ```bash
   cd systems/<system-id>
   zip -r ../../<system-id>.zip manifest.json data/
   ```

2. Upload to a Chronicle instance:
   - Go to **Campaign Settings > Content Packs > Upload System**
   - Upload the ZIP
   - Check the validation report

3. Verify:
   - Browse reference pages: `/campaigns/<id>/systems/<system-id>`
   - Test search: type in the search bar
   - Test tooltips: @mention an item in an entity
   - Test entity preset: create a new entity of the system's character type
   - If Foundry-annotated: test character sync in Foundry VTT

## Step 6: Submit

1. Fork this repository
2. Add your system directory under `systems/`
3. Validate passes
4. Submit a pull request with:
   - Description of the system
   - License information
   - Source attribution

## Researching Foundry VTT Data Paths

To find the correct `foundry_path` values for a game system:

### Method 1: Console Inspection
1. Open a Foundry VTT world using the target game system
2. Create a character Actor
3. Open browser console (F12)
4. Run: `console.log(JSON.stringify(game.actors.contents[0].system, null, 2))`
5. Browse the JSON structure to find field paths

### Method 2: Active Effects Wiki
Many Foundry game systems document their data paths in a GitHub wiki
for Active Effects. Search for `<system-name> foundry active effects wiki`.

### Method 3: System Source Code
1. Find the game system's GitHub repository
2. Look for `template.json` or `system.json` in the source
3. The `Actor.templates` or `Actor.types` section defines the data structure

### Method 4: Data Inspector Module
Install the "Data Inspector" Foundry module to browse Actor data in a tree view.

## Common Patterns

### D&D-like Systems
```json
{
  "key": "str", "foundry_path": "system.abilities.str.value",
  "key": "hp_current", "foundry_path": "system.attributes.hp.value",
  "key": "ac", "foundry_path": "system.attributes.ac.value", "foundry_writable": false,
  "key": "level", "foundry_path": "system.details.level", "foundry_writable": false
}
```

### Narrative Systems
```json
{
  "key": "stress", "foundry_path": "system.stress.value",
  "key": "harm_1", "foundry_path": "system.harm.level1",
  "key": "xp", "foundry_path": "system.experience.value"
}
```

### Systems with Non-Standard Actor Types
Some systems use custom actor types instead of `"character"`:
- Draw Steel: `"hero"` for player characters
- Some systems: `"pc"` vs `"npc"`
- Check `CONFIG.Actor.typeLabels` in the Foundry console
