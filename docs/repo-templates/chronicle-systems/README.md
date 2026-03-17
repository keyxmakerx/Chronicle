# Chronicle Systems - Game System Content Packs

Game system reference data packs for [Chronicle](https://github.com/keyxmakerx/Chronicle),
a self-hosted TTRPG worldbuilding platform.

## What This Is

This repository contains JSON data packs that provide reference content (spells, monsters,
items, classes, ancestries, etc.) for tabletop RPG game systems. Chronicle loads these packs
to provide:

- Browsable reference pages with search
- Hover tooltips for @mentioned game content
- Entity type presets (e.g., "D&D Character" with predefined stat fields)
- Foundry VTT character sync field mappings

## Available Systems

| System | ID | Foundry ID | License | Categories |
|--------|-----|-----------|---------|------------|
| D&D 5th Edition | `dnd5e` | `dnd5e` | OGL-1.0a | spells, monsters, items, classes, races, conditions |
| Draw Steel | `drawsteel` | `draw-steel` | CC-BY-4.0 | abilities, creatures, ancestries, kits |
| Pathfinder 2e | `pathfinder2e` | `pf2e` | ORC | spells, creatures, equipment, ancestries, classes, conditions |

## How Systems Are Used

### Installation via Chronicle Admin Panel

1. Chronicle's admin panel fetches releases from this repository
2. Admin selects which systems and versions to install
3. Chronicle downloads the ZIP, validates it, and loads it into memory
4. Campaign owners enable systems as addons for their campaigns

### Manual Upload

1. Download a system ZIP from [Releases](../../releases)
2. In Chronicle: **Campaign Settings > Content Packs > Upload System**
3. Upload the ZIP file

### ZIP Format

Each system is packaged as a ZIP containing:
```
manifest.json        # System metadata, categories, presets, field definitions
data/
  spells.json        # Reference items for the "spells" category
  monsters.json      # Reference items for the "monsters" category
  ...                # One JSON file per category
```

## Repository Structure

```
chronicle-systems/
  systems/
    dnd5e/
      manifest.json
      data/
        spells.json
        monsters.json
        items.json
        classes.json
        races.json
        conditions.json
    drawsteel/
      manifest.json
      data/
        abilities.json
        creatures.json
        ancestries.json
        kits.json
    pathfinder2e/
      manifest.json
      data/
        spells.json
        creatures.json
        equipment.json
        ancestries.json
        classes.json
        conditions.json
  schema/
    manifest.schema.json         # JSON Schema for manifest validation
    reference-item.schema.json   # JSON Schema for data file validation
  scripts/
    validate.sh                  # Validates all systems against schemas
    package.sh                   # Builds per-system ZIPs for release
  .ai/                           # AI documentation
  .github/
    workflows/
      validate.yml               # CI: lint JSON, validate schemas
      release.yml                # On tag: build ZIPs, create release
```

## Contributing a New System

See `.ai/creating-a-system.md` for a complete step-by-step guide.

Quick summary:
1. Create `systems/<system-id>/manifest.json` with categories, presets, and field definitions
2. Add `data/*.json` files with reference items for each category
3. Validate: `./scripts/validate.sh systems/<system-id>`
4. Submit a pull request

### License Requirements

- All reference data must be legally shareable (OGL, ORC, CC-BY, SRD, etc.)
- Specify the license in `manifest.json` → `license` field
- Include `source` attribution on each reference item
- **Never include copyrighted content** that isn't covered by an open license

## Releases

Tags trigger GitHub Actions to build per-system ZIPs:
- `chronicle-system-dnd5e-v1.0.0.zip`
- `chronicle-system-drawsteel-v1.0.0.zip`
- `chronicle-system-pathfinder2e-v1.0.0.zip`

Chronicle's package manager fetches these releases automatically.

## License

Each system's data is licensed per its `manifest.json` `license` field.
Repository infrastructure (schemas, scripts, docs) is MIT licensed.
