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
| widget `host_mounted` | `true` for a widget Chronicle mounts itself elsewhere (the encounter builder's budget panel); the Customizer doesn't offer it. A widget the package's book places is left out the same way. | `internal/systems/handler.go` |
| `dm_screen` | Party tracker and conditions on the DM Screen | `internal/systems/manifest_dm_screen.go` |
| `terms.gm` | The game's name for the game master ("Director"), used across the campaign; default "GM" | `internal/systems/manifest.go` |
| `designs` | Saved designs the package ships (see "Shipping designs" below) | `internal/systems/package_designs.go` |
| `foundry_system_id` | Matches Foundry's `game.system.id` for character sync | Foundry module |
| `tooltip_template` | Go `text/template` for hover tooltips | `internal/systems/generic_tooltip.go` |
| field `choices` | Names the data file a character field picks from | `internal/systems/character_choices.go` |

## Shipping designs

`designs` lists saved designs (a shop counter room theme, a crafting design)
that every campaign on your system sees under **From <your system>** in the
design library, read only. An Owner picks one to use it, or uses **Save as my
design** to keep their own copy; the copy never follows later releases.

```json
"designs": [
  { "kind": "room-theme", "name": "Ashgate Forge", "note": "Warm oak and iron",
    "maker": "Your Name", "file": "designs/ashgate-forge.json" }
]
```

- `kind` is a design kind the server knows (`room-theme`, `crafting-circle`,
  `crafting-item-look`, `crafting-bench`). `name` is up to 60 characters,
  `note` 140, `maker` 100 (both optional).
- `file` is a `.json` file in the package: a relative path, no `..`, not
  absolute, at most 16 KB, holding the design's data. Keep it outside `data/`,
  which is served raw. Use at most 100 designs.
- The data must pass the kind's own checks. Any entry that fails is skipped and
  named in `/campaigns/:id/systems/status` and Admin > Health & diagnostics;
  the rest of the package still loads.
- Designs are read from a loaded package, not from a campaign's uploaded custom
  system.

Owners can send you designs: in the library, **Suggest for <your system>**
downloads one JSON file:

```json
{ "format": "chronicle-design-suggestion:v1", "system": "yourgame",
  "designs": [ { "kind": "room-theme", "name": "...", "note": "...", "maker": "...", "file": "designs/name.json" } ],
  "data": { "designs/name.json": { } },
  "messageForMaintainer": "optional note, up to 240 characters" }
```

Paste each `designs` entry into your manifest, save each `data` value as the
file named by its key, and release. `maker` is present only if the Owner chose
to be credited.

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
