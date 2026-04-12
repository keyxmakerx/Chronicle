# Content Extension System (Layer 1) — Design Document

**Date:** 2026-03-06
**Status:** Proposed
**Scope:** Declarative content packs — no code execution

---

## 1. Manifest Schema Design

The manifest extends Chronicle's existing `SystemManifest` pattern (see `internal/systems/manifest.go`) but generalizes it beyond game systems to cover all declarative content types.

### Design Influences

| Platform | What We Adopt | What We Skip |
|----------|---------------|-------------|
| **Foundry VTT** | `manifest.json` format, compatibility ranges, relationships array, flags storage | Esmodules/scripts (Layer 2+) |
| **Obsidian** | `manifest.json` identity fields, `minAppVersion` | Desktop plugin model |
| **VS Code** | `contributes` object pattern (declaring what you provide) | `activationEvents` (no code) |
| **npm** | `name`, `version`, `description`, `author`, `license`, `repository`, `keywords` | `scripts`, `dependencies` (no code) |
| **Chrome Extensions** | `permissions` array, `content_scripts` concept | Background scripts |

### Complete JSON Schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://chronicle.app/schemas/extension-manifest-v1.json",
  "title": "Chronicle Content Extension Manifest",
  "type": "object",
  "required": ["id", "name", "version", "manifest_version", "description"],
  "additionalProperties": false,
  "properties": {

    "manifest_version": {
      "type": "integer",
      "const": 1,
      "description": "Schema version. Always 1 for this spec."
    },

    "id": {
      "type": "string",
      "pattern": "^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$",
      "description": "Globally unique identifier. Lowercase alphanumeric + hyphens, 3-64 chars. Convention: author-name (e.g., 'taldorei-calendar', 'dnd5e-monster-pack')."
    },
    "name": {
      "type": "string",
      "minLength": 1,
      "maxLength": 100,
      "description": "Human-readable display name."
    },
    "version": {
      "type": "string",
      "pattern": "^\\d+\\.\\d+\\.\\d+$",
      "description": "Semantic version (major.minor.patch)."
    },
    "description": {
      "type": "string",
      "minLength": 1,
      "maxLength": 500,
      "description": "Short description shown in extension browser."
    },

    "author": {
      "type": "object",
      "properties": {
        "name":  { "type": "string", "maxLength": 100 },
        "email": { "type": "string", "format": "email" },
        "url":   { "type": "string", "format": "uri" }
      },
      "required": ["name"]
    },
    "license": {
      "type": "string",
      "maxLength": 50,
      "description": "SPDX license identifier (e.g., 'MIT', 'CC-BY-4.0', 'OGL-1.0a')."
    },
    "homepage": {
      "type": "string",
      "format": "uri"
    },
    "repository": {
      "type": "string",
      "format": "uri"
    },
    "keywords": {
      "type": "array",
      "items": { "type": "string", "maxLength": 30 },
      "maxItems": 10,
      "description": "Searchable tags (e.g., 'dnd5e', 'calendar', 'forgotten-realms')."
    },
    "icon": {
      "type": "string",
      "description": "Path to icon image within package (e.g., 'assets/icon.png'), or Font Awesome class (e.g., 'fa-dragon')."
    },

    "compatibility": {
      "type": "object",
      "properties": {
        "minimum": {
          "type": "string",
          "pattern": "^\\d+\\.\\d+\\.\\d+$",
          "description": "Minimum Chronicle version required."
        },
        "maximum": {
          "type": "string",
          "pattern": "^\\d+\\.\\d+\\.\\d+$",
          "description": "Maximum Chronicle version supported (optional)."
        },
        "verified": {
          "type": "string",
          "pattern": "^\\d+\\.\\d+\\.\\d+$",
          "description": "Last Chronicle version tested against."
        }
      },
      "required": ["minimum"]
    },

    "requires_addons": {
      "type": "array",
      "items": {
        "type": "string",
        "description": "Addon slug that must be enabled (e.g., 'calendar', 'maps', 'timeline')."
      },
      "description": "Campaign addons required. Extension cannot be enabled unless these addons are active."
    },

    "dependencies": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id":      { "type": "string", "description": "Extension ID." },
          "version": { "type": "string", "description": "Semver range (e.g., '>=1.0.0')." }
        },
        "required": ["id"]
      },
      "description": "Other content extensions this depends on."
    },

    "conflicts": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Extension IDs that conflict with this one."
    },

    "contributes": {
      "type": "object",
      "description": "Declares what content this extension provides. Inspired by VS Code's contributes pattern.",
      "properties": {

        "entity_type_templates": {
          "type": "array",
          "items": { "$ref": "#/$defs/entity_type_template" },
          "description": "Pre-configured entity types with fields, icons, colors, and page layouts."
        },

        "entity_packs": {
          "type": "array",
          "items": { "$ref": "#/$defs/entity_pack" },
          "description": "Collections of pre-made entities."
        },

        "calendar_presets": {
          "type": "array",
          "items": { "$ref": "#/$defs/calendar_preset" },
          "description": "Pre-configured calendar systems."
        },

        "tag_collections": {
          "type": "array",
          "items": { "$ref": "#/$defs/tag_collection" },
          "description": "Pre-defined tag sets."
        },

        "relation_types": {
          "type": "array",
          "items": { "$ref": "#/$defs/relation_type" },
          "description": "Pre-defined relationship type pairs."
        },

        "marker_icon_packs": {
          "type": "array",
          "items": { "$ref": "#/$defs/marker_icon_pack" },
          "description": "Custom map marker icons."
        },

        "themes": {
          "type": "array",
          "items": { "$ref": "#/$defs/theme" },
          "description": "CSS theme overrides."
        },

        "reference_data": {
          "type": "array",
          "items": { "$ref": "#/$defs/reference_data_pack" },
          "description": "Read-only reference data (similar to internal modules)."
        }
      }
    },

    "checksum": {
      "type": "string",
      "description": "SHA-256 of the zip contents (excluding manifest.json itself). For integrity verification."
    }
  },

  "$defs": {

    "entity_type_template": {
      "type": "object",
      "required": ["slug", "name", "name_plural"],
      "properties": {
        "slug":        { "type": "string", "pattern": "^[a-z0-9-]+$" },
        "name":        { "type": "string" },
        "name_plural": { "type": "string" },
        "icon":        { "type": "string", "default": "fa-file" },
        "color":       { "type": "string", "pattern": "^#[0-9a-fA-F]{6}$", "default": "#6b7280" },
        "description": { "type": "string" },
        "fields":      { "type": "array", "items": { "$ref": "#/$defs/field_def" } },
        "layout":      { "type": "object", "description": "EntityTypeLayout JSON (rows/columns/blocks)." }
      }
    },

    "field_def": {
      "type": "object",
      "required": ["key", "label", "type"],
      "properties": {
        "key":     { "type": "string" },
        "label":   { "type": "string" },
        "type":    { "type": "string", "enum": ["string", "number", "text", "select", "multiselect", "checkbox", "url", "date"] },
        "options": { "type": "array", "items": { "type": "string" }, "description": "For select/multiselect fields." },
        "default": { "description": "Default value for new entities." },
        "group":   { "type": "string", "description": "Field grouping label." }
      }
    },

    "entity_pack": {
      "type": "object",
      "required": ["slug", "name", "file"],
      "properties": {
        "slug":              { "type": "string" },
        "name":              { "type": "string" },
        "description":       { "type": "string" },
        "entity_type_slug":  { "type": "string", "description": "Target entity type. Must exist or be provided by entity_type_templates." },
        "file":              { "type": "string", "description": "Path to JSON data file within package (e.g., 'data/monsters.json')." },
        "count":             { "type": "integer", "description": "Number of entities in the pack (informational)." }
      }
    },

    "calendar_preset": {
      "type": "object",
      "required": ["slug", "name", "file"],
      "properties": {
        "slug":        { "type": "string" },
        "name":        { "type": "string" },
        "description": { "type": "string" },
        "file":        { "type": "string", "description": "Path to calendar JSON (e.g., 'data/harptos.json')." },
        "game_system": { "type": "string", "description": "Associated game system (e.g., 'dnd5e', 'pathfinder2e')." }
      }
    },

    "tag_collection": {
      "type": "object",
      "required": ["slug", "name", "tags"],
      "properties": {
        "slug":        { "type": "string" },
        "name":        { "type": "string" },
        "description": { "type": "string" },
        "tags": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["name"],
            "properties": {
              "name":      { "type": "string" },
              "slug":      { "type": "string" },
              "color":     { "type": "string", "pattern": "^#[0-9a-fA-F]{6}$" },
              "parent":    { "type": "string", "description": "Parent tag slug for nesting." }
            }
          }
        }
      }
    },

    "relation_type": {
      "type": "object",
      "required": ["type", "reverse_type"],
      "properties": {
        "type":         { "type": "string", "description": "Forward label (e.g., 'parent of')." },
        "reverse_type": { "type": "string", "description": "Reverse label (e.g., 'child of')." },
        "category":     { "type": "string", "description": "Grouping (e.g., 'family', 'political', 'military')." }
      }
    },

    "marker_icon_pack": {
      "type": "object",
      "required": ["slug", "name", "icons"],
      "properties": {
        "slug":        { "type": "string" },
        "name":        { "type": "string" },
        "description": { "type": "string" },
        "icons": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["id", "name", "file"],
            "properties": {
              "id":       { "type": "string" },
              "name":     { "type": "string" },
              "file":     { "type": "string", "description": "Path to SVG/PNG within package." },
              "category": { "type": "string" }
            }
          }
        }
      }
    },

    "theme": {
      "type": "object",
      "required": ["slug", "name", "file"],
      "properties": {
        "slug":        { "type": "string" },
        "name":        { "type": "string" },
        "description": { "type": "string" },
        "file":        { "type": "string", "description": "Path to CSS file within package." },
        "preview":     { "type": "string", "description": "Path to preview image." }
      }
    },

    "reference_data_pack": {
      "type": "object",
      "required": ["slug", "name", "categories"],
      "properties": {
        "slug":             { "type": "string" },
        "name":             { "type": "string" },
        "description":      { "type": "string" },
        "tooltip_template": { "type": "string" },
        "categories": {
          "type": "array",
          "items": {
            "type": "object",
            "required": ["slug", "name", "file"],
            "properties": {
              "slug":   { "type": "string" },
              "name":   { "type": "string" },
              "icon":   { "type": "string" },
              "file":   { "type": "string" },
              "fields": { "type": "array", "items": { "$ref": "#/$defs/field_def" } }
            }
          }
        }
      }
    }
  }
}
```

### Example Manifest: Forgotten Realms Calendar

```json
{
  "manifest_version": 1,
  "id": "forgotten-realms-calendar",
  "name": "Forgotten Realms Calendar (Harptos)",
  "version": "1.0.0",
  "description": "The Calendar of Harptos used in the Forgotten Realms campaign setting. Includes months, weekdays, festivals, and seasons.",
  "author": { "name": "Chronicle Community" },
  "license": "CC-BY-4.0",
  "keywords": ["dnd5e", "forgotten-realms", "calendar", "harptos"],
  "icon": "fa-calendar-days",

  "compatibility": {
    "minimum": "1.0.0",
    "verified": "1.2.0"
  },

  "requires_addons": ["calendar"],

  "contributes": {
    "calendar_presets": [
      {
        "slug": "harptos",
        "name": "Calendar of Harptos",
        "description": "12 months of 30 days + 5 intercalary festival days. 10-day weeks (tendays). Leap year (Shieldmeet) every 4 years.",
        "file": "data/harptos.json",
        "game_system": "dnd5e"
      }
    ]
  }
}
```

### Example Manifest: D&D 5e Monster Pack

```json
{
  "manifest_version": 1,
  "id": "dnd5e-srd-monsters",
  "name": "D&D 5e SRD Monster Pack",
  "version": "1.0.0",
  "description": "325 monsters from the D&D 5th Edition System Reference Document. Importable as entities with pre-filled stat blocks.",
  "author": { "name": "Chronicle Community" },
  "license": "OGL-1.0a",
  "keywords": ["dnd5e", "monsters", "bestiary", "srd"],
  "icon": "fa-dragon",

  "compatibility": {
    "minimum": "1.0.0"
  },

  "contributes": {
    "entity_type_templates": [
      {
        "slug": "dnd5e-creature",
        "name": "D&D Creature",
        "name_plural": "D&D Creatures",
        "icon": "fa-skull",
        "color": "#DC2626",
        "fields": [
          { "key": "cr", "label": "Challenge Rating", "type": "string" },
          { "key": "type", "label": "Type", "type": "select", "options": ["Aberration", "Beast", "Celestial", "Construct", "Dragon", "Elemental", "Fey", "Fiend", "Giant", "Humanoid", "Monstrosity", "Ooze", "Plant", "Undead"] },
          { "key": "size", "label": "Size", "type": "select", "options": ["Tiny", "Small", "Medium", "Large", "Huge", "Gargantuan"] },
          { "key": "alignment", "label": "Alignment", "type": "string" },
          { "key": "ac", "label": "Armor Class", "type": "number" },
          { "key": "hp", "label": "Hit Points", "type": "string" },
          { "key": "speed", "label": "Speed", "type": "string" },
          { "key": "str", "label": "STR", "type": "number", "group": "Ability Scores" },
          { "key": "dex", "label": "DEX", "type": "number", "group": "Ability Scores" },
          { "key": "con", "label": "CON", "type": "number", "group": "Ability Scores" },
          { "key": "int", "label": "INT", "type": "number", "group": "Ability Scores" },
          { "key": "wis", "label": "WIS", "type": "number", "group": "Ability Scores" },
          { "key": "cha", "label": "CHA", "type": "number", "group": "Ability Scores" }
        ]
      }
    ],
    "entity_packs": [
      {
        "slug": "srd-monsters",
        "name": "SRD Monsters",
        "description": "All 325 SRD monsters with full stat blocks.",
        "entity_type_slug": "dnd5e-creature",
        "file": "data/monsters.json",
        "count": 325
      }
    ],
    "tag_collections": [
      {
        "slug": "monster-types",
        "name": "Monster Type Tags",
        "tags": [
          { "name": "Aberration", "color": "#7C3AED" },
          { "name": "Beast", "color": "#059669" },
          { "name": "Dragon", "color": "#DC2626" },
          { "name": "Undead", "color": "#4B5563" }
        ]
      }
    ]
  }
}
```

---

## 2. Extension Types / Content Categories

### 2.1 Entity Type Templates

**What it provides:** Pre-configured entity types with field definitions, icons, colors, and optionally page layouts.

**Files in package:**
- Declared inline in `manifest.json` under `contributes.entity_type_templates`
- Optional: layout JSON can reference a separate file

**How Chronicle imports:**
1. Creates new `entity_types` row with the template's slug (prefixed with extension ID to avoid conflicts: `ext:dnd5e-srd-monsters:dnd5e-creature`).
2. User can rename, customize fields, or detach (copy without link to extension).
3. If the entity type slug already exists in the campaign, prompt user: skip, rename, or replace.

**Merge behavior:** Additive. Never replaces existing entity types. Creates new ones alongside.

### 2.2 Entity Packs

**What it provides:** Collections of pre-made entities with field data, descriptions, images, tags, and relations.

**Files in package:**
```
data/monsters.json          # Array of entity objects
assets/images/beholder.webp # Optional entity images
```

**Entity data file format** (matches Chronicle's export format for compatibility):
```json
[
  {
    "name": "Beholder",
    "slug": "beholder",
    "entity_type_slug": "dnd5e-creature",
    "entry": null,
    "entry_html": "<p>A beholder is a floating orb...</p>",
    "image": "assets/images/beholder.webp",
    "fields_data": {
      "cr": "13",
      "type": "Aberration",
      "ac": 18,
      "hp": "180 (19d10 + 76)"
    },
    "tags": ["Aberration"],
    "type_label": "Aberration"
  }
]
```

**How Chronicle imports:**
1. Validates entity type slug exists (from templates or campaign's existing types).
2. Creates entities with `created_by` set to the importing user.
3. Copies referenced images from extension assets to campaign media storage.
4. Creates tags that don't already exist, links entity-tag associations.
5. Slug conflicts: append numeric suffix (`beholder-2`).

**Merge behavior:** Additive. Entities are created alongside existing ones. No replacement.

### 2.3 Calendar Presets

**What it provides:** Complete calendar configurations including months, weekdays, moons, seasons, eras, and optionally events.

**Files in package:**
```
data/harptos.json
```

**Calendar data file format:**
```json
{
  "name": "Calendar of Harptos",
  "epoch_name": "Dale Reckoning",
  "current_year": 1492,
  "current_month": 1,
  "current_day": 1,
  "hours_per_day": 24,
  "minutes_per_hour": 60,
  "seconds_per_minute": 60,
  "leap_year_every": 4,
  "leap_year_offset": 0,
  "months": [
    { "name": "Hammer", "days": 30, "sort_order": 0, "is_intercalary": false },
    { "name": "Midwinter", "days": 1, "sort_order": 1, "is_intercalary": true, "leap_year_days": 1 },
    { "name": "Alturiak", "days": 30, "sort_order": 2, "is_intercalary": false }
  ],
  "weekdays": [
    { "name": "First-day", "sort_order": 0 },
    { "name": "Second-day", "sort_order": 1 }
  ],
  "moons": [
    { "name": "Selune", "cycle_days": 30.4375, "phase_offset": 0, "color": "#e8e8e8" }
  ],
  "seasons": [
    { "name": "Winter", "start_month": 0, "start_day": 1, "end_month": 2, "end_day": 30, "color": "#93c5fd" }
  ],
  "eras": [
    { "name": "Dale Reckoning", "start_year": 1, "end_year": null, "color": "#6366f1" }
  ],
  "event_categories": [
    { "slug": "festival", "name": "Festival", "icon": "fa-champagne-glasses", "color": "#f59e0b" },
    { "slug": "holy-day", "name": "Holy Day", "icon": "fa-church", "color": "#a855f7" }
  ],
  "events": [
    {
      "name": "Midwinter Celebration",
      "year": 0,
      "month": 1,
      "day": 1,
      "category": "festival",
      "is_recurring": true,
      "recurrence_type": "yearly",
      "description": "A feast celebrating the midpoint of winter."
    }
  ]
}
```

**How Chronicle imports:**
1. Campaign must have calendar addon enabled.
2. If campaign already has a calendar configured, prompt: "Replace existing calendar?" or "Cancel."
3. Calendar presets are **replacement** -- they configure the entire calendar system.
4. Events with `year: 0` are treated as templates applied to `current_year`.

**Merge behavior:** Replacement for calendar structure. Events are additive (appended to existing events if keeping current calendar).

### 2.4 Map Marker Icon Packs

**What it provides:** Custom SVG/PNG icons for map markers beyond the built-in Font Awesome set.

**Files in package:**
```
assets/icons/castle.svg
assets/icons/tavern.svg
assets/icons/dungeon.svg
```

**How Chronicle imports:**
1. Icons copied to `data/extensions/<ext-id>/icons/`.
2. Marker icon picker UI extended to show extension icon packs as additional groups.
3. Map markers reference custom icons as `ext:<ext-id>:<icon-id>`.

**Merge behavior:** Additive. Icon packs stack.

### 2.5 Theme Variants

**What it provides:** CSS overrides for visual customization.

**Files in package:**
```
styles/sepia.css
assets/preview.png
```

**CSS constraints:**
- Only CSS custom properties (variables) and class overrides allowed.
- No `@import`, no `url()` pointing outside the extension directory.
- Validated at install time; rejected if constraints violated.

**How Chronicle imports:**
1. CSS file copied to `data/extensions/<ext-id>/styles/`.
2. Theme appears in campaign settings theme picker.
3. Selected theme CSS is injected via `<link>` tag after the base stylesheet.

**Merge behavior:** Selectable (one theme active per campaign). Does not merge.

### 2.6 Tag Collections

**What it provides:** Pre-defined tag hierarchies with colors.

**How Chronicle imports:**
1. Creates tags that don't already exist (matched by slug within campaign).
2. Existing tags with same slug are skipped (not overwritten).
3. Parent-child relationships are established if `parent` slug references another tag in the collection.

**Merge behavior:** Additive. Skips duplicates by slug.

### 2.7 Relation Type Templates

**What it provides:** Suggested relationship type/reverse_type pairs for the relation creation UI.

**How Chronicle imports:**
1. Stored in `extension_data` table (not in entity_relations -- these are *suggestions*, not actual relations).
2. Relation creation modal shows extension-provided types as autocomplete suggestions.
3. Users can still type custom relation types.

**Merge behavior:** Additive. Suggestions stack from multiple extensions.

### 2.8 Reference Data Packs

**What it provides:** Read-only reference content identical to internal modules (spells, items, conditions, etc.).

**Files in package:**
```
data/spells.json
data/monsters.json
```

**How Chronicle imports:**
1. Loaded into memory by a `JSONProvider` instance (same as internal modules).
2. Registered with the module system as a content-extension-backed module.
3. Appears in tooltip lookups and reference pages.

**Merge behavior:** Additive. Multiple reference packs can coexist if categories don't collide. Same-category collision: last-installed wins (with warning).

---

## 3. Storage Design

### Recommended: Hybrid Approach (Option B + Option C)

After evaluating the three options:

**Option A (Generic key-value table):** Flexible but loses relational integrity. Querying "all entities from extension X" requires scanning a denormalized table. Uninstall is clean (DELETE WHERE extension_id = ?), but runtime queries are slow.

**Option B (Write to existing tables):** Extension entity types go into `entity_types`, tags go into `tags`, calendar configs go into `calendars`. Data lives where it naturally belongs. Queries work normally. But tracking provenance ("which extension created this?") requires annotation.

**Option C (Foundry-style flags):** Namespaced metadata on existing records. Elegant for metadata, but overkill for creating whole new entity types or entities.

### Decision: Option B with provenance tracking

Extensions write data into existing tables, but every record created by an extension is tracked in a provenance table.

#### New Tables

```sql
-- Migration: 000055_content_extensions
-- Description: Content extension system tables.

-- Installed extensions registry.
CREATE TABLE extensions (
    id VARCHAR(36) NOT NULL PRIMARY KEY,
    ext_id VARCHAR(64) NOT NULL UNIQUE,          -- manifest id (e.g., 'dnd5e-srd-monsters')
    name VARCHAR(100) NOT NULL,
    version VARCHAR(20) NOT NULL,
    manifest JSON NOT NULL,                       -- Full manifest.json stored for reference
    installed_by VARCHAR(36) NOT NULL,            -- FK -> users.id
    status VARCHAR(20) NOT NULL DEFAULT 'active', -- 'active', 'disabled'
    created_at DATETIME NOT NULL DEFAULT NOW(),
    updated_at DATETIME NOT NULL DEFAULT NOW() ON UPDATE NOW(),
    CONSTRAINT fk_ext_installed_by FOREIGN KEY (installed_by) REFERENCES users(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Per-campaign extension activation.
-- Reuses the existing addons/campaign_addons pattern but links to extensions.
CREATE TABLE campaign_extensions (
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    applied_contents JSON DEFAULT '{}',           -- Tracks which contributes were imported
    enabled_at DATETIME NOT NULL DEFAULT NOW(),
    enabled_by VARCHAR(36) NULL,
    PRIMARY KEY (campaign_id, extension_id),
    CONSTRAINT fk_ce_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ce_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Provenance tracking: which extension created which records.
-- Enables clean uninstall and conflict detection.
CREATE TABLE extension_provenance (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    table_name VARCHAR(64) NOT NULL,              -- 'entity_types', 'entities', 'tags', etc.
    record_id VARCHAR(36) NOT NULL,               -- PK of the created record
    record_type VARCHAR(50) NOT NULL DEFAULT '',   -- Sub-type hint (e.g., 'entity_pack:srd-monsters')
    created_at DATETIME NOT NULL DEFAULT NOW(),
    INDEX idx_ep_campaign_ext (campaign_id, extension_id),
    INDEX idx_ep_table_record (table_name, record_id),
    CONSTRAINT fk_ep_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ep_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Extension-specific data that doesn't fit existing tables.
-- Used for relation type suggestions, marker icon metadata, etc.
CREATE TABLE extension_data (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    extension_id VARCHAR(36) NOT NULL,
    namespace VARCHAR(50) NOT NULL,               -- 'relation_types', 'marker_icons', etc.
    data_key VARCHAR(100) NOT NULL,
    data_value JSON NOT NULL,
    UNIQUE KEY uk_ext_data (campaign_id, extension_id, namespace, data_key),
    CONSTRAINT fk_ed_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ed_extension FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### Uninstall Behavior

1. Query `extension_provenance` for all records created by the extension in the target campaign.
2. For each record: check if it has been modified by the user since creation.
   - Unmodified: safe to delete.
   - Modified (entity edited, entity type fields customized): warn user, ask for confirmation.
   - Has dependents (entities exist under an extension-provided entity type): block or warn with count.
3. Delete provenance records after cleanup.
4. Delete `extension_data` rows.
5. Delete `campaign_extensions` row.

### Conflict Avoidance

- Entity type slugs: prefixed with extension ID at creation time. User can rename.
- Tag slugs: skip-if-exists behavior. No overwrites.
- Calendar presets: explicit replacement prompt.
- Multiple extensions providing the same content type: allowed. Each tracked independently.

---

## 4. Installation & Lifecycle

### 4.1 Installation Methods

**Method 1: Zip Upload (Primary)**
- Site admin uploads `.chronicle-ext` file (renamed `.zip`) via admin panel.
- Maximum upload size: configurable, default 50 MB.
- File is extracted to `data/extensions/<ext-id>/`.

**Method 2: Directory Scan**
- Admin places extension directory in `data/extensions/`.
- On startup or admin-triggered rescan, Chronicle discovers new extensions.
- Useful for Docker volume mounts and development.

**Method 3: URL Install (Future)**
- Admin provides a URL to a `.chronicle-ext` file.
- Chronicle downloads, verifies checksum, installs.
- Supports manifest URL for update checking (Foundry VTT pattern).

### 4.2 Installation Flow

```
User uploads zip
    |
    v
[Extract to temp dir]
    |
    v
[Read manifest.json] --> Invalid? --> Return error
    |
    v
[Validate manifest schema]
    |
    v
[Validate file types: only .json, .css, .svg, .png, .webp, .jpg]
    |
    v
[Check compatibility.minimum <= Chronicle version]
    |
    v
[Check conflicts with installed extensions]
    |
    v
[Check dependencies are installed]
    |
    v
[Move to data/extensions/<ext-id>/]
    |
    v
[INSERT into extensions table]
    |
    v
[Register as addon in addons table (category='extension')]
    |
    v
Success: Extension available for per-campaign activation
```

### 4.3 Per-Campaign Activation

1. Campaign owner navigates to Plugins page (existing `campaigns/:id/plugins`).
2. Extensions appear alongside built-in addons in a separate "Content Extensions" section.
3. Owner clicks "Enable" on an extension.
4. **Import wizard** appears:
   - Shows what the extension contributes (entity types, entity packs, calendar presets, etc.).
   - Each contribution type has a checkbox (default: all checked).
   - For entity packs: shows count of entities to import.
   - For calendar presets: warns if calendar already configured.
5. On confirm:
   - Selected contributions are imported into campaign tables.
   - Provenance records created for each imported record.
   - `campaign_extensions.applied_contents` JSON updated to track what was imported.
6. Extension is now "enabled" for the campaign.

### 4.4 Disable vs Uninstall

**Disable (per-campaign):**
- Extension content remains in the campaign (entities, entity types, tags stay).
- Extension-provided theme CSS is no longer injected.
- Extension-provided relation type suggestions are hidden.
- Reference data is no longer available for tooltips.
- Reversible: re-enable restores everything without re-import.

**Uninstall (site-wide):**
- Removes extension files from disk.
- For each campaign that had it enabled:
  - Provenance-tracked records: user prompted for cleanup strategy.
  - "Keep data" (default): records stay, provenance records deleted (data becomes "native").
  - "Remove data": provenance-tracked records deleted (with dependent checks).
- Extension row deleted from `extensions` table (CASCADE deletes `campaign_extensions` and `extension_provenance`).

### 4.5 Updates

1. Admin uploads new version of the same extension (same `id`, higher `version`).
2. Chronicle compares old and new manifests:
   - New contributions added: available for import on next campaign activation.
   - Removed contributions: existing data stays (orphaned gracefully).
   - Modified contributions: no automatic update to campaign data (user controls their data).
3. Extension files replaced on disk.
4. `extensions.version` and `extensions.manifest` updated.
5. Per-campaign: owner can re-run import wizard to pull in new content.

### 4.6 Version Conflict Resolution

- If extension A requires extension B `>=2.0.0` but installed B is `1.5.0`: block install of A with error message.
- If two extensions declare `conflicts` with each other: block install of the second with error message.
- Semver comparison uses Go's `semver` package (or simple major.minor.patch comparison).

---

## 5. Directory Structure

### 5.1 Extension Package (What the Author Creates)

```
forgotten-realms-calendar/
├── manifest.json
├── data/
│   └── harptos.json
├── assets/
│   └── icon.png
└── README.md              (optional, not processed by Chronicle)
```

```
dnd5e-monster-pack/
├── manifest.json
├── data/
│   └── monsters.json
├── assets/
│   └── images/
│       ├── beholder.webp
│       ├── dragon-red.webp
│       └── mind-flayer.webp
└── README.md
```

```
dark-parchment-theme/
├── manifest.json
├── styles/
│   └── dark-parchment.css
├── assets/
│   ├── icon.png
│   └── preview.png
└── README.md
```

```
fantasy-map-icons/
├── manifest.json
├── assets/
│   └── icons/
│       ├── castle.svg
│       ├── tavern.svg
│       ├── dungeon.svg
│       ├── port.svg
│       └── forest.svg
└── README.md
```

### 5.2 Installed Extension (Where Chronicle Stores It)

```
data/                                    # Chronicle data root (configurable via DATA_DIR env)
└── extensions/
    ├── forgotten-realms-calendar/
    │   ├── manifest.json
    │   ├── data/
    │   │   └── harptos.json
    │   └── assets/
    │       └── icon.png
    ├── dnd5e-monster-pack/
    │   ├── manifest.json
    │   ├── data/
    │   │   └── monsters.json
    │   └── assets/
    │       └── images/
    │           ├── beholder.webp
    │           └── dragon-red.webp
    ├── dark-parchment-theme/
    │   ├── manifest.json
    │   ├── styles/
    │   │   └── dark-parchment.css
    │   └── assets/
    │       ├── icon.png
    │       └── preview.png
    └── fantasy-map-icons/
        ├── manifest.json
        └── assets/
            └── icons/
                ├── castle.svg
                └── tavern.svg
```

### 5.3 Go Package Structure

```
internal/
└── extensions/                          # New plugin for extension management
    ├── .ai.md
    ├── model.go                         # Extension, CampaignExtension, ExtensionProvenance, ExtensionData
    ├── manifest.go                      # ExtensionManifest type, validation, compatibility check
    ├── repository.go                    # CRUD for extensions, campaign_extensions, provenance, data
    ├── service.go                       # Install, uninstall, enable, disable, import logic
    ├── importer.go                      # Content import logic per contribution type
    ├── handler.go                       # HTTP handlers (admin install/uninstall, campaign enable/disable)
    ├── routes.go                        # Route registration
    ├── security.go                      # Zip validation, path traversal prevention, file type checks
    └── service_test.go
```

---

## 6. API Design

### 6.1 Admin Endpoints (Site-wide Extension Management)

```
# List all installed extensions
GET /admin/extensions
Response: { "extensions": [ { "id": "...", "ext_id": "...", "name": "...", "version": "...", "status": "active", ... } ] }

# Get extension details
GET /admin/extensions/:extID
Response: { "extension": { ... }, "campaigns_using": 5 }

# Install extension (zip upload)
POST /admin/extensions/install
Content-Type: multipart/form-data
Body: file=<.chronicle-ext file>
Response: 201 { "extension": { ... } }

# Install extension from URL (future)
POST /admin/extensions/install-url
Body: { "url": "https://example.com/ext.chronicle-ext" }
Response: 201 { "extension": { ... } }

# Uninstall extension
DELETE /admin/extensions/:extID
Query: ?strategy=keep_data|remove_data (default: keep_data)
Response: 200 { "removed_from_campaigns": 3 }

# Rescan extensions directory
POST /admin/extensions/rescan
Response: 200 { "discovered": 2, "total": 8 }

# Update extension (upload new version)
PUT /admin/extensions/:extID
Content-Type: multipart/form-data
Body: file=<.chronicle-ext file>
Response: 200 { "extension": { ... }, "old_version": "1.0.0", "new_version": "1.1.0" }
```

### 6.2 Campaign Endpoints (Per-Campaign Extension Management)

```
# List extensions available for this campaign
GET /campaigns/:id/extensions
Response: { "extensions": [ { "ext_id": "...", "name": "...", "enabled": true, "applied_contents": {...} } ] }

# Get extension details for campaign (what it contributes, what's been imported)
GET /campaigns/:id/extensions/:extID
Response: { "extension": { ... }, "contributions": { "entity_types": 2, "entities": 325, ... }, "applied": {...} }

# Enable extension for campaign (triggers import wizard data)
GET /campaigns/:id/extensions/:extID/preview
Response: { "contributes": { "entity_type_templates": [...], "entity_packs": [...], ... } }

# Apply extension to campaign
POST /campaigns/:id/extensions/:extID/apply
Body: {
  "import": {
    "entity_type_templates": true,
    "entity_packs": ["srd-monsters"],
    "calendar_presets": ["harptos"],
    "tag_collections": true
  }
}
Response: 200 { "imported": { "entity_types": 1, "entities": 325, "tags": 14 } }

# Disable extension for campaign
DELETE /campaigns/:id/extensions/:extID
Query: ?strategy=keep_data|remove_data
Response: 200

# List extension-provided content in campaign
GET /campaigns/:id/extensions/:extID/content
Response: { "entity_types": [...], "entities": { "count": 325 }, "tags": [...] }
```

### 6.3 Static Asset Serving

```
# Serve extension static assets (icons, images, CSS)
GET /extensions/:extID/assets/*filepath
# Served from data/extensions/<extID>/assets/
# Only allowed file types: .svg, .png, .webp, .jpg, .css
# Cache-Control: public, max-age=86400
```

### 6.4 HTMX Integration

All campaign-facing endpoints support HTMX fragment responses:

```
# If HX-Request: true
# Returns HTML fragment for the extension card/list/wizard
# If HX-Request absent
# Returns full page with layout
```

The admin extension management page follows the same pattern as the existing admin addons/modules pages. The campaign extension page integrates into the existing Plugins page as a new section.

---

## 7. Migration Path from Internal Modules

### Strategy: Internal modules stay internal. Extensions are a parallel system.

**Rationale:**
1. Internal modules (`dnd5e`, `pathfinder2e`, `drawsteel`) are compiled into the Go binary. They use the `Module` interface with `DataProvider` and `TooltipRenderer`.
2. Content extensions are user-uploaded, live on disk, and are loaded at runtime.
3. The infrastructure overlap is in reference data serving -- both use `ReferenceItem` and could share the `JSONProvider`.

### Concrete Plan

1. **Keep internal modules as-is.** They ship with the Docker image, are always available, and use the existing factory/registry pattern.

2. **Content extensions that provide `reference_data` reuse `JSONProvider`.** The extension importer creates a `JSONProvider` instance from the extension's data files and registers it with the module system under a unique ID (`ext:<ext-id>`).

3. **Shared interfaces:** The existing `DataProvider` and `ReferenceItem` types in `internal/systems/system.go` are used by both built-in systems and content extension reference data. No duplication.

4. **Addon integration:** Content extensions register themselves in the `addons` table with `category='extension'`. The existing `campaign_addons` per-campaign enable/disable system works for them. The new `campaign_extensions` table adds extension-specific metadata (applied contents, provenance).

5. **Future:** If a content extension provides the same data as an internal module (e.g., a community "D&D 5e Expanded Monsters" pack alongside the built-in `dnd5e` module), they coexist. Search results and tooltips aggregate across both.

### What NOT to do

- Do not convert existing internal modules to content extensions. They benefit from being compiled in (type safety, direct integration with tooltip rendering, performance).
- Do not duplicate the module loading infrastructure. Content extensions plug into the existing module registry for reference data.

---

## 8. Security Considerations

### 8.1 File Type Validation

**Allowlist (strict):**
```go
var allowedExtensionFileTypes = map[string]bool{
    ".json":  true,
    ".css":   true,
    ".svg":   true,
    ".png":   true,
    ".webp":  true,
    ".jpg":   true,
    ".jpeg":  true,
    ".txt":   true,
    ".md":    true,
}
```

Any file not matching the allowlist is rejected during installation.

**Explicitly blocked:**
- `.js`, `.html`, `.htm` (no executable content in Layer 1)
- `.exe`, `.sh`, `.bat`, `.py`, `.go`, `.wasm` (no executables)
- `.php`, `.asp`, `.jsp` (no server-side scripts)
- Dotfiles (`.env`, `.git`, `.htaccess`)

### 8.2 Path Traversal Prevention

```go
// validateZipEntry checks that a zip entry path is safe.
func validateZipEntry(name string) error {
    // Reject absolute paths
    if filepath.IsAbs(name) {
        return fmt.Errorf("absolute path not allowed: %s", name)
    }
    // Reject path traversal
    cleaned := filepath.Clean(name)
    if strings.HasPrefix(cleaned, "..") || strings.Contains(cleaned, ".."+string(filepath.Separator)) {
        return fmt.Errorf("path traversal not allowed: %s", name)
    }
    // Reject paths starting with /
    if strings.HasPrefix(name, "/") {
        return fmt.Errorf("leading slash not allowed: %s", name)
    }
    return nil
}
```

Applied to every entry during zip extraction. Extraction target directory is always `data/extensions/<ext-id>/` -- the ext-id is derived from the validated manifest, not from the zip structure.

### 8.3 Manifest Validation

1. **Schema validation:** `manifest.json` must conform to the JSON schema defined above. Missing required fields or wrong types are rejected.
2. **ID validation:** Extension ID must match `^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`. No special characters, no path components.
3. **Version format:** Must be valid semver (`major.minor.patch`).
4. **File path validation:** All paths in `contributes` (file references) must be relative, within the package, and point to allowed file types.
5. **Duplicate ID check:** Cannot install an extension with the same ID as an already-installed extension (unless updating).

### 8.4 Size Limits

| Limit | Default | Configurable |
|-------|---------|-------------|
| Max zip size | 50 MB | `EXT_MAX_UPLOAD_SIZE` |
| Max extracted size | 100 MB | `EXT_MAX_EXTRACTED_SIZE` |
| Max files in zip | 1000 | `EXT_MAX_FILES` |
| Max single file | 20 MB | `EXT_MAX_FILE_SIZE` |
| Max entity pack size | 10,000 entities | `EXT_MAX_ENTITY_PACK` |
| Max CSS file size | 500 KB | `EXT_MAX_CSS_SIZE` |

### 8.5 CSS Validation

```go
// validateCSS checks that extension CSS is safe.
func validateCSS(content []byte) error {
    s := string(content)
    // Block @import (could load external resources)
    if strings.Contains(s, "@import") {
        return fmt.Errorf("@import is not allowed in extension CSS")
    }
    // Block url() except for data: URIs (could load external resources)
    // Allow: url(data:...) for small inline images
    // Block: url(https://...), url(//...), url(http://...)
    urlPattern := regexp.MustCompile(`url\s*\(\s*(['"]?)(?!data:)`)
    if urlPattern.MatchString(s) {
        return fmt.Errorf("url() with external resources is not allowed")
    }
    // Block expression() (IE-specific, can execute JS)
    if strings.Contains(strings.ToLower(s), "expression(") {
        return fmt.Errorf("expression() is not allowed")
    }
    // Block behavior: (IE-specific, can execute HTC files)
    if strings.Contains(strings.ToLower(s), "behavior:") {
        return fmt.Errorf("behavior: is not allowed")
    }
    return nil
}
```

### 8.6 SVG Validation

SVG files are validated to ensure they don't contain embedded scripts:

```go
// validateSVG checks that an SVG file is safe.
func validateSVG(content []byte) error {
    s := string(content)
    lower := strings.ToLower(s)
    // Block <script> tags
    if strings.Contains(lower, "<script") {
        return fmt.Errorf("script tags not allowed in SVG")
    }
    // Block event handlers (onclick, onload, etc.)
    if regexp.MustCompile(`\bon\w+\s*=`).MatchString(lower) {
        return fmt.Errorf("event handlers not allowed in SVG")
    }
    // Block href="javascript:"
    if strings.Contains(lower, "javascript:") {
        return fmt.Errorf("javascript: URIs not allowed in SVG")
    }
    return nil
}
```

### 8.7 Extension Signing (Optional, Defense-in-Depth)

For verified extensions (e.g., official Chronicle community packs):

1. Author generates SHA-256 checksums of all files, writes to `checksums.sha256` in the package.
2. Author signs `checksums.sha256` with their ED25519 private key, producing `checksums.sha256.sig`.
3. Chronicle admin can configure trusted public keys.
4. On install, if a signature is present and a trusted key matches, the extension is marked "verified."
5. Unverified extensions install with a warning banner but are not blocked.

This is defense-in-depth, not a hard requirement. The primary security model is the file type allowlist and no-code-execution constraint.

---

## 9. Implementation Roadmap

### Phase 1: Foundation (1-2 sprints)
- [ ] Migration 000055: `extensions`, `campaign_extensions`, `extension_provenance`, `extension_data` tables
- [ ] `internal/extensions/model.go`: Extension, CampaignExtension, Provenance, Data types
- [ ] `internal/extensions/manifest.go`: ExtensionManifest parsing and validation
- [ ] `internal/extensions/security.go`: Zip validation, path traversal, file type checks
- [ ] `internal/extensions/repository.go`: CRUD for all four tables

### Phase 2: Installation & Admin UI (1-2 sprints)
- [ ] `internal/extensions/service.go`: Install, uninstall, list, rescan
- [ ] `internal/extensions/handler.go`: Admin endpoints (install, uninstall, list)
- [ ] Admin extensions management page (Templ templates)
- [ ] Zip upload UI with validation feedback
- [ ] Static asset serving endpoint

### Phase 3: Campaign Integration & Import (2-3 sprints)
- [ ] `internal/extensions/importer.go`: Per-content-type import logic
  - Entity type templates -> entity_types table
  - Entity packs -> entities table (with image copying)
  - Calendar presets -> calendar tables
  - Tag collections -> tags table
  - Relation type suggestions -> extension_data table
- [ ] Campaign extension enable/disable endpoints
- [ ] Import wizard UI (preview contributions, select what to import)
- [ ] Provenance tracking on all imported records

### Phase 4: Advanced Content Types (1-2 sprints)
- [ ] Marker icon packs (custom icon serving, icon picker integration)
- [ ] Theme variants (CSS injection, theme picker integration)
- [ ] Reference data packs (JSONProvider integration with module system)

### Phase 5: Polish (1 sprint)
- [ ] Uninstall with data cleanup wizard
- [ ] Extension update flow
- [ ] Extension info page (campaign-facing, shows what was imported)
- [ ] Tests: manifest validation, security checks, import/export roundtrip
