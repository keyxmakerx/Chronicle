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
| `tag_collections` | Creates the tags. `parent` is ignored. |
| `widgets` | Loads the scripts on that campaign's pages and adds them to the layout palette. See [widget.md](widget.md). |
| `wasm_plugins` | Loads the plugins. See [wasm-plugin.md](wasm-plugin.md). |
| `marker_icon_packs`, `themes` | Stored and listed by JSON endpoints only; no page uses them yet. |
| `entity_packs`, `calendar_presets` | Accepted, not applied yet (#660). |
| `relation_types`, `reference_data` | Accepted, not applied. |

An apply error is logged and the pack still shows as on. Content is copied
once per campaign; turning the pack off does not delete it.

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
