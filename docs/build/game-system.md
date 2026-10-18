# Build a game system package

A game system package is a folder of JSON (plus optional JS and YAML) that
teaches Chronicle one game: reference data with tooltips, entity type presets,
character fields, widgets and a rulebook. No Go code. Chronicle installs it
from a GitHub release and runs it through a generic loader.

## Files

```
my-game/
  manifest.json          # required
  data/<category>.json   # one per category; the filename is the category slug
  widgets/<slug>.js      # optional, see widget.md
  book/book.yaml         # optional rulebook, see ../system-rulebook-book.md
  book/chapters/*.yaml
```

Executables and scripts other than `.js` (`.sh`, `.py`, `.exe`, …) and
symlinks are rejected at install.

## Minimal manifest

```json
{
  "id": "mygame",
  "name": "My Game",
  "api_version": "1",
  "status": "available",
  "categories": [
    {
      "slug": "spells",
      "name": "Spells",
      "fields": [{ "key": "level", "label": "Level", "type": "number" }]
    }
  ]
}
```

- Required: `id`, `name`, `api_version` (use `"1"`).
- Set `"status": "available"`. A missing status means `coming_soon`: the
  package loads but can't be turned on in a campaign.
- `id` and every slug match `^[a-z0-9][a-z0-9_-]*$`. The `id` becomes the
  campaign add-on slug and the URL segment (`/campaigns/:id/systems/mygame`).
- Leave `version` out. Install sets it from the release tag.
- Field `type` is one of `string`, `number`, `boolean`, `list`, `markdown`,
  `enum`, `url`.
- `"director": true` on a category keeps it to the campaign's Directors: its
  pages, search hits, tooltips and raw `data/<slug>.json`. Use it for content
  your book keeps in a Director-only chapter, so players can't read it there.
- Caps: 20 categories, 100 fields each, 10 entity presets, 20 relation
  presets, 10 widgets.

Validation lives in `internal/systems/manifest.go` (`ValidateManifest`).

## Minimal data file

`data/spells.json` is an array of reference items:

```json
[
  {
    "slug": "fire-bolt",
    "name": "Fire Bolt",
    "summary": "A mote of fire.",
    "description": "You hurl a mote of fire at a creature or object.",
    "properties": { "level": 0 },
    "tags": ["evocation"],
    "source": "custom"
  }
]
```

- An item needs `slug` (or `id`) and `name`; items without them are skipped.
- A manifest field `key` reads `properties[key]` and prints it with Go's
  `%v`. Keep displayed properties scalar: a nested object prints as
  `map[...]`. See [../system-package-rendering.md](../system-package-rendering.md).
- A data file whose category isn't in the manifest is loaded but never shown.
- Widgets can read any data file raw at
  `GET /campaigns/:id/systems/<id>/data/<file>.json`.

## Optional manifest sections

| Key | What it does | Read by |
|---|---|---|
| `entity_presets` | Entity types (with fields) a campaign gets when it adopts the system | `internal/systems/manifest.go` |
| `relation_presets` | Relation types (with reverse names) | same |
| `widgets`, `text_renderers` | Browser JS; renderers load first | `internal/systems/handler.go` |
| `renderers`, `entity_panels` | Widgets Chronicle mounts itself on entity pages | `internal/systems/handler.go` |
| `dm_screen` | Party tracker and conditions on the DM Screen | `internal/systems/manifest_dm_screen.go` |
| `foundry_system_id` | Matches Foundry's `game.system.id` for character sync | Foundry module |
| `tooltip_template` | Go `text/template` for hover tooltips | `internal/systems/generic_tooltip.go` |
| field `choices` | Names the data file a character field picks from | `internal/systems/character_choices.go` |

## Install and test

1. Push the folder to a GitHub repo and publish a **release** (the tag is the
   version, a leading `v` is dropped). Chronicle downloads the release's first
   `.zip` asset, or the tag's source zipball if there is none. A single
   top-level folder in the zip is unwrapped.
2. Admin > Game systems & modules (`/admin/packages`) > Add package, paste
   `https://github.com/you/my-game`, confirm your password. The latest
   non-prerelease installs.
3. In a campaign, as owner: Manage > Game & features
   (`/campaigns/:id/extensions`) > Game system > pick My Game.
4. Check it: `/campaigns/:id/systems/mygame` lists your categories, and
   `/campaigns/:id/systems/status` shows item counts and validation warnings.

Working: your categories and items appear and tooltips open. Broken: the
system is missing from the picker. Open Admin > Health & diagnostics
(`/admin/systems`); its load events name the skipped manifest or item and why.

Faster loop on a dev server: put the folder at
`<MEDIA_PATH>/packages/systems/mygame/0.0.1/` (default `./data/media`) and
restart; the server scans that tree at startup.

## Further reading

- [../system-package-rendering.md](../system-package-rendering.md): how fields render.
- [../system-rulebook-book.md](../system-rulebook-book.md): the `book/` format.
- [../system-plugin-marketplace.md](../system-plugin-marketplace.md): the submission and review flow.
