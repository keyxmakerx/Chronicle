# Build a content extension

A content extension (a "content pack") is a zip a site admin installs once and
each campaign owner turns on for their campaign. Turning it on copies its
content into that campaign. It can also carry widgets and WASM plugins.

## Files

```
my-pack/
  manifest.json      # required, at the zip root (one wrapping folder is fine)
  widgets/*.js       # optional
  *.wasm             # optional, see wasm-plugin.md
  migrations/        # optional, NNN_name.up.sql / .down.sql
```

Allowed file types: `.json .css .svg .png .webp .jpg .jpeg .txt .md .js .wasm`.
No dotfiles. Limits: 50 MB upload, 1000 files, 20 MB per file, 100 MB unpacked.
CSS may not `@import` or load remote `url()`; SVGs may not contain scripts.

## Minimal manifest

```json
{
  "manifest_version": 1,
  "id": "my-pack",
  "name": "My Pack",
  "version": "1.0.0",
  "description": "Factions and tags for my setting.",
  "contributes": {
    "entity_type_templates": [
      { "slug": "faction", "name": "Faction", "name_plural": "Factions",
        "icon": "fa-flag", "color": "#7c3aed" }
    ],
    "tag_collections": [
      { "slug": "moods", "name": "Moods",
        "tags": [{ "name": "Hostile", "color": "#ef4444" }, { "name": "Friendly" }] }
    ]
  }
}
```

- Required: `manifest_version` (`1`), `id` (`^[a-z0-9][a-z0-9-]{1,62}[a-z0-9]$`),
  `name`, `version` (`X.Y.Z`), `description`.
- Optional: `author {name}`, `license`, `homepage`, `repository`, `keywords`,
  `icon`, `requires_addons`, `dependencies`, `conflicts`.
- File paths are relative and may not contain `..`. Missing files are not
  caught at install.
- Validation: `internal/extensions/manifest.go` (`ValidateManifest`).

## What each `contributes` key does today

| Key | Effect when a campaign turns the pack on |
|---|---|
| `entity_type_templates` | Creates entity types (name, plural, icon, colour, `category`). Template `fields` are not applied. |
| `entity_packs` | Creates the pages in each pack file. See "Entity packs" below. |
| `tag_collections` | Creates the tags. `parent` is ignored. |
| `widgets` | Loads the scripts on that campaign's pages and adds them to the layout palette. See [widget.md](widget.md). |
| `wasm_plugins` | Loads the plugins. See [wasm-plugin.md](wasm-plugin.md). |
| `marker_icon_packs`, `themes` | Stored and listed by JSON endpoints only; no page uses them yet. |
| `calendar_presets` | Accepted, not applied yet (#1882). |
| `relation_types`, `reference_data` | Accepted, not applied. |

An apply error is logged and the pack still shows as on. Content is copied
once per campaign: turning the pack off does not delete it, and turning it on
again does not create entity types or pack pages a second time.

## Entity packs

```json
"entity_packs": [
  { "slug": "monsters", "name": "Monsters", "entity_type_slug": "monster",
    "file": "data/monsters.json" }
]
```

`entity_type_slug` is the `slug` of one of this pack's `entity_type_templates`,
or else the slug of a category the campaign already has (`character`, say). A
pack whose category can't be found is skipped. The file is a JSON array of
pages, at most 10,000:

```json
[
  { "name": "Goblin", "type_label": "Humanoid",
    "entry": "Small, **mean** and always in a hurry.",
    "fields": { "cr": "1/4" } },
  { "name": "Owlbear", "private": false }
]
```

- `name` is required (up to 200 characters).
- `entry` is the page's text in Markdown; raw HTML is not kept.
- `fields` sets custom field values by key.
- Pages start GM-only, since a pack may hold spoilers; `"private": false` makes
  one visible to players from the start.
- A page that fails (a duplicate is fine, a blank name is not) is skipped and
  logged; the rest of the pack still goes in.

## Own tables (optional)

Files in `migrations/` (`001_init.up.sql`, `001_init.down.sql`) run at install
and roll back at uninstall. Every table they create, alter or write must be
named `ext_<id>_…` (hyphens in the id become underscores).

## Install and test

1. Zip the folder.
2. Admin > Extensions (`/admin/extensions`) > upload the zip, confirm your
   password. Or, on a server you run, copy the folder into `EXTENSIONS_PATH`
   (default `./extensions`) and press Rescan. Rescan does not run migrations.
3. In a campaign, as owner: Manage > Game & features
   (`/campaigns/:id/extensions`) > Content Packs > turn it on.

Working: the new entity types appear in the sidebar and the tags in the tag
picker. Broken: the upload is refused with the failing rule, or the pack turns
on with nothing new (check the server log for the apply error).

Examples: `extensions/dice-roller/` (widget), `extensions/harptos-calendar/`
(tags; its calendar preset is not applied yet).
