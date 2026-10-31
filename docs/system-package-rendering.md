# System Package Rendering Contract

How a system package (Draw Steel, D&D 5.5e, ...) plugs into the entity-show
page to render system-specific layouts. This is the external contract for
package authors; internal conventions are in `.ai/conventions.md`.

## What you ship vs. what the host ships

The host ships:

- The `EntityShowRendererRegistry` extension point.
- The dispatch guard in `internal/plugins/entities/show.templ`, which consults
  the registry before the layout-block iteration.
- The CSS theming variables (see "Theming contract").
- The generic block system (`BlockRegistry`; `title`, `image`, `entry`,
  `attributes`, ...), the fallback when no renderer is registered.

The host ships no character-specific code: no `blockStatBlock`, `blockHPBar`
or default character layout.

You ship, in `manifest.json` and widget scripts (packages are manifest-only, no Go):

- A widget for each entity-type slug or preset category you care about, plus a
  `renderers` entry binding them (see "Declaring renderers in `manifest.json`").
- The data conventions (`fields_data` keys, layouts) the widget relies on.
- Optionally, JS widgets for `ext_widget` blocks, for layout-editor building
  blocks instead of a whole-page renderer.

## The registry

`internal/plugins/entities/show_renderer_registry.go` defines:

```go
type EntityShowRenderContext struct {
    CC             *campaigns.CampaignContext
    Entity         *Entity
    EntityType     *EntityType
    Ancestors      []Entity
    Children       []Entity
    ShowAttributes bool
    ShowCalendar   bool
    CSRFToken      string
    UserID         string // viewing user's id; empty for an anonymous viewer
    OwnerName      string
    NPCPage        bool
    NamedLayout    bool
}

type EntityShowRenderer func(ctx EntityShowRenderContext) templ.Component

type EntityShowRendererRegistry struct { /* opaque */ }

func NewEntityShowRendererRegistry() *EntityShowRendererRegistry
func (r *EntityShowRendererRegistry) Register(slug string, renderer EntityShowRenderer)
func (r *EntityShowRendererRegistry) Lookup(slug string) (EntityShowRenderer, bool)
```

The registry is keyed on the entity-type **slug**, not on system ID. One
package can register several slugs (`drawsteel-character`,
`drawsteel-monster`, ...), each with exactly one renderer.

`EntityShowRenderContext` carries what the block fallback can read
(`Ancestors`, `Children`, `CSRFToken`, the addon flags). If you need more, file
an issue to extend the struct; do not reach into globals.

## How registration works

Registration is driven by the manifest. At startup and after every package
install or update, the host walks each loaded manifest's `renderers` and
registers a Go renderer that emits one widget mount point
(`registerManifestRenderers` in `internal/app/routes.go`). The set is built in a
fresh registry and then published, so in-flight requests never see a
half-built state. No restart is needed; new renderers apply to the next page
view. Packages never call `Register` themselves; that call is for in-tree Go
code. Both paths fill the same registry, and the last registration for a slug
wins (admins control which packages are installed, so collisions are rare).

## Dispatch

On an entity show page:

1. Build `EntityShowRenderContext` from the request data.
2. Look up the entity type's slug in the registry.
3. **If a renderer is registered**: render the page using your
   renderer's component. Done.
4. **If no renderer is registered**: fall through to the existing
   layout-block dispatch. The page renders using whatever
   `entityType.Layout.Rows` defines.

One exception comes before step 2: a page wearing one of its category's
named layouts (a "Hero" layout beside an "NPC" one, say) is drawn from that
layout, and `EntityShowRenderContext.NamedLayout` is true. Your renderer is
then not asked for the page; the owner places your sheet with the Game
System Sheet block (`game_system_sheet`), which calls your renderer with
`NamedLayout` false.

A panic inside your renderer is not caught: it produces a 500 page like any
other render bug. Return a `templ.Component` that shows the error for any
recoverable failure.

## Theming contract

Use these CSS variables so a campaign-level theme retints your output:

| Variable | Use |
|---|---|
| `--color-accent` | Primary brand / interactive color. |
| `--color-accent-hover` | Hover state for interactive elements. |
| `--color-accent-light` | Lighter shade for badges, soft tints. |
| `--color-accent-rgb` | Comma-separated RGB triple for `rgba()` calls. |
| `--color-accent-hover-rgb` | Same, for the hover variant. |
| `--color-accent-light-rgb` | Same, for the light variant. |
| `--font-campaign` | Campaign body font override. |

Plus the Tailwind tokens in `.ai/conventions.md` and `tailwind.config.js`:

- `bg-surface`, `bg-surface-alt`, `bg-surface-raised`, `bg-page`
- `text-fg`, `text-fg-body`, `text-fg-secondary`, `text-fg-muted`,
  `text-fg-faint`
- `border-edge`, `border-edge-light`
- `bg-accent`, `bg-accent-hover`, `text-accent`

Do not define your own brand colors; a campaign's accent must apply to your
renderer too.

## Out of scope

- Host-provided system-shaped blocks (stat blocks, resource bars, ability cards) and default character layouts: the package owns them.
- Renderer-level config: per-entity state lives in `fields_data`, per-campaign state in campaign settings. A renderer takes a slug and a context.
- Edit-in-place, dice integration, encounter and session state.

## Lighter extension points

The registry is for whole-page rendering. For less:

- **Custom block types:** register against `BlockRegistry`
  (`internal/plugins/entities/block_registry.go`) and reference them from a
  layout JSON; the owner mixes them in the layout editor.
- **JS widgets:** `Chronicle.register()` plus an `ext_widget` block, for
  client-side interactivity (dice rollers, animated cards).

## Declaring renderers in `manifest.json`

Declare the binding "render this entity type by mounting that widget" in your
package's `manifest.json` and the host wires it up:

```json
{
  "id": "drawsteel",
  "name": "Draw Steel",
  "api_version": "1",
  "entity_presets": [
    { "slug": "drawsteel-character", "name": "Draw Steel Character" }
  ],
  "widgets": [
    { "slug": "drawsteel-character-card", "name": "Character Card",
      "script_file": "widgets/character-card.js" }
  ],
  "renderers": [
    { "slug": "drawsteel-character",
      "widget": "drawsteel-character-card",
      "description": "Stat-block style character sheet." }
  ]
}
```

Each entry registers a renderer that emits one mount point,
`<div data-widget="drawsteel-character-card" data-entity-id="…"
data-campaign-id="…">`. `boot.js` finds it and calls the widget's `init(el,
config)`; the widget owns the page from there. `data-*` attributes become
`config.entityId` and so on (kebab to camel; see `static/js/boot.js`).

The mount also carries, all `"true"`/`"false"` except the name and visibility:

| Attribute | Meaning |
|---|---|
| `data-is-gm` | Viewer can see GM-only content. |
| `data-is-owner` | Viewer is this page's claimed owner. |
| `data-visibility` | The page's visibility mode. |
| `data-can-edit-identity` | Viewer may rename the page and set the identity fields (ancestry, culture, career, kit, race, species, heritage, background): Scribe and up, or the claimed owner. The server enforces the same rule. |
| `data-can-change-image` | Viewer may replace the picture (Scribe and up; the claimed owner may not). |
| `data-armory-items` | The Items and money panel draws on this same page, so the widget leaves its own item list out. |
| `data-claimed` | Someone has claimed the page. |
| `data-claimed-by-me` | The viewer is that someone. |
| `data-claimed-name` | The claimant's display name ("a player" when it can't be resolved), empty when unclaimed. |

To change the picture, a widget dispatches the bubbling event
`chronicle:change-image` with `detail: { entityId }`; the page opens its upload,
and does nothing for a viewer who may not change it. After a rename from the
header the page dispatches `chronicle:page-renamed` with `detail: { entityId,
name }`.

The host draws the page header (name, favorite, claim marker, Edit name,
History, Clone and Delete under a more menu) above the widget, and Player
notes, Relations, Tags and Sub-pages below it. Do not draw those.

### Validation rules (enforced at install time)

An invalid `renderers` block rejects the package at install. Every entry
must satisfy:

- Exactly one of `slug` or `preset_category` is set (see "Binding by preset
  category"). `slug` matches one of this manifest's `entity_presets[].slug`;
  `preset_category` matches one of its `entity_presets[].category`. A manifest
  cannot bind types or categories it does not own.
- `widget` matches one of this manifest's `widgets[].slug`.
- `slug`/`preset_category` and `widget` are valid slugs (lowercase letters,
  numbers, hyphens, underscores).
- A manifest declares no more than 10 renderers.

### Adding a panel to NPC pages (`entity_panels`)

A renderer replaces a page; a panel adds to one. Declare a widget in
`entity_panels` to offer it on NPC or monster pages in campaigns with your
system enabled. Panels show where the owner places the "Game System Panels"
block in the page template (and above a renderer's page, which has no
template); nothing is added where the owner has not placed it:

```json
"widgets": [
  { "slug": "drawsteel-negotiation", "name": "Negotiation",
    "script_file": "widgets/negotiation.js" }
],
"entity_panels": [
  { "widget": "drawsteel-negotiation", "applies_to": "npc" }
]
```

- `widget` must match one of this manifest's `widgets[].slug`.
- `applies_to` is `"npc"` or `"creature"`; anything else is rejected at install.
  - `"npc"`: the page types the owner listed as NPCs on the Characters page,
    with their sub-types, not the player-character type. A system's creature
    types are not NPCs until the owner lists them.
  - `"creature"`: preset category `creature`, or a slug of `creature` or
    `*-monster`, with sub-types, listed as NPCs or not. For a stat block on every
    bestiary page.
  - Neither matches a page a player has claimed, and a widget aimed at both
    mounts once on a page that is both.
  - An older Chronicle rejects a manifest using `"creature"`; ship it once your
    target instances have it.
- A manifest declares no more than 10 panels.

The host emits `<div data-widget="…" data-campaign-id="…" data-entity-id="…"
data-system-id="…" data-is-gm="true|false">`. `data-is-gm` is true for the
campaign owner and members granted DM access; use it for GM-only controls, never
to protect data. The script loads like any other widget of the enabled system.

Panel state uses the page-state API (the owner/DM team writes; anyone who can
view the page reads the public half):

- `GET|PUT /campaigns/:id/entities/:eid/system-state/:system/:key` with the
  session. `PUT` takes `{"public": {…}?, "gm": {…}?}`; an absent half keeps what
  is stored and a present half replaces it. Each half must be a JSON object of
  at most 16 KiB. `GET` returns `{systemId, key, public, gm?, isGm, updatedAt}`,
  and `gm` is present only for the DM team.
- `system` is your manifest `id` and must be enabled for the campaign; `system`
  and `key` match `^[a-z0-9][a-z0-9_-]{0,63}$`.
- Writes publish `system_state.updated` (`{systemId, key}`, no content) to the
  GM side only, so a DM's other tabs and the Foundry module can re-read.
- For a document that belongs to the whole campaign rather than one page,
  `GET|PUT /campaigns/:id/campaign-system-state/:system/:key` takes and returns
  the same shapes under the same rules, except that any campaign member may
  read the public half and a write publishes nothing.

### Binding by preset category

A renderer binds by `slug` (a type your package owns) or `preset_category` (a
category, matching one of your `entity_presets[].category` values), never both:

```json
"renderers": [
  { "preset_category": "character", "widget": "character-sheet" }
]
```

The widget then renders any entity type carrying that category. The host
resolves slug first, then the type's `preset_category`, so a preset binding
never shadows another package's slug-bound type. This lets a package fill a
category core owns (see "Player-character sheet").

A `preset_category` binding draws a hero's sheet, so it skips NPC pages: an
unclaimed page whose type is on the campaign's NPC list keeps its layout. A
claimed page, or one marked playable (every hero the hero creator makes), gets
the renderer whatever its type. A `slug` binding applies to every page of that
type.

### Player-character sheet

The Player Character Claiming addon owns a player-character category; the
system fills the rendering. A package ships:

1. **An entity preset with `category: "character"`** plus its `fields` — the
   field schema your sheet reads:

   ```json
   "entity_presets": [
     { "slug": "drawsteel-character", "name": "Hero",
       "category": "character",
       "fields": [ { "key": "stamina", "label": "Stamina", "type": "number" } ] }
   ]
   ```

2. **A renderer for your sheet widget** — either form works:

   ```json
   "renderers": [
     { "slug": "drawsteel-character",  "widget": "character-sheet" },
     { "preset_category": "character", "widget": "character-sheet" }
   ]
   ```

   The `slug` form binds the sheet to your own character type. The
   `preset_category` form renders any `character`-category type, including a
   generic Chronicle-owned "Player Characters" type your package does not own.
   Ship one or both.

When the addon is enabled in a campaign with your system, your character preset
(fields and sheet) is applied to the existing top-level **Characters** category;
no second category is created. A system-less campaign gets a generic "Player
Characters" type.

A new system needs no core changes: ship a `category: "character"` preset and a
`character-sheet` renderer. Core finds a system character type generically (by
preset category and claimability) and never names a system; fields, widget,
renderer and type name all live in the manifest.

### Sheet styles and motion (`data-sheet`)

Chronicle supplies a sheet's styles and motion; the package ships markup only.
Put `data-sheet` on the sheet's root (a block element that is not transformed).
Chronicle sets `data-sheet-style` on it from the campaign's sheet style
(`html[data-cz-sheet]`), or `modern`, and loads that style's fonts. The twelve
styles are `modern`,
`parchment`, `ledger`, `journal`, `vellum`, `night`, `deck`, `pencil`,
`starship`, `neon`, `runes` and `brass`. Do not set colours, fonts, shadows,
durations or effects yourself. Read the `--paper-*` tokens (`--paper-ink`,
`--paper-ink-soft`, `--paper-mute`, `--paper-edge`, `--paper-accent`,
`--paper-neg`, `--paper-font`, `--paper-head-font`, `--paper-num-font`,
`--ts` for the type scale) and the shared classes in `static/css/paper.css`
(`.paper`, `.paper-stack`, `.paper-kind`, `.paper-title`, `.paper-pull`,
`.paper-slip`, `.paper-pin`) so style changes reach your sheet. Nothing outside
a `[data-sheet]` root is restyled.

What your markup carries:

| Attribute / class | Meaning |
|---|---|
| `data-sheet` | The root. Without one, none of this runs. |
| `data-sheet-folio` | Optional wrapper around the sheet; it slides left when a panel opens beside it. |
| `.paper-stack > .paper` | The sheet itself. |
| `data-sheet-section="id"` on a `.paper-pull` | A part with paper behind it. Pressing anywhere on it presses its button. |
| `data-sheet-open="panel-id"` | The button in a part that opens a panel. |
| `<template data-sheet-panel="panel-id" data-title data-kind>` | The panel's body. Chronicle builds the panel around it. |
| `<details data-sheet-fold>` with `.fold-body > .fold-in[data-move="fold"]` | A fold that opens to its real height. |
| `data-v` / `data-pv` on a number | A value that can change; call `Chronicle.sheetMotion.land(el, oldText)` after changing it. |

Moves: every moving element names its standard move with `data-move`:
`slide-out` (a panel from behind the sheet), `grow` (a slip out of the thing
pressed), `fold` (a fold's content) or `settle` (the sheet arriving on a new
page; add `.paper-settle` to play it once). A style never changes timing; it gives each move its look and picks the move
for each role (`--sheet-panel-move`, `--sheet-slip-move`, `--sheet-page-move`).
`data-move` is honoured only under `[data-sheet]`.

Changed values: each style sets `--change-move` (`mark`, `rewrite`, `roll`,
`glitch`, `carve` or `odometer`). `Chronicle.sheetMotion.land(el, oldText)`
applies it; with no old text it shows a new value arriving.

Events on the root: `sheet:panel-ready` (`detail.body` is the panel's body, so
the package can wire widgets inside it), `sheet:open`, `sheet:close`, and
`sheet:close-blocked` (a panel marked unsaved was asked to close). Mark one
with `Chronicle.sheetMotion.mount(root).setDirty(true)`.

Motion levels: Full plays every move. Calm (owner's reduce switch or a
person's Calm pick) fades in place and opens folds at once. Off (device
reduced-motion or a person's Off pick) jumps to the end state. The engine
enforces both; packages do nothing.

### Hero creator

The hero creator (Characters > Create hero) needs nothing beyond the sheet. It
asks one question per text field of your `category: "character"` preset that
has entries to pick from (your data file for that field plus Director-added
entries), in preset order. The field's `label` names the step ("Lineage" gives
"Choose your lineage").

Each entry opens into a leaf drawn only from what it carries, with no empty
sections:

- `summary` and `description` make the Overview; `{@category term}` markers
  read as plain words.
- Scalar `properties` (numbers, short strings) show as a row of facts.
  Keys ending `_display` are skipped, since they repeat a structured value.
- Each property that is a list of named items (`[{ "name": …, "description": … }]`)
  becomes its own tab, labelled from the key (`signature_traits` → "Signature traits").
- **Buying from a list.** When one of those lists gives every item a numeric
  `cost` and the entry has a positive number under a key ending `_points`
  (`ancestry_points: 3`), the tab becomes "Choose …": the hero buys items up
  to that budget. A `quick_build` list of names offers a suggested set.

The chosen entry's **name** is saved in the field, as the sheet's picker does.
Bought items are saved beside it as a JSON array of names in
`<field>_choices_json` (`ancestry_choices_json: "[\"Grounded\"]"`), so your
sheet can read them; declare that key as a `string` field if the sheet should
show or edit it.

### Checking the AI's numbers (`Chronicle.sheetChecks`)

When the campaign's AI writes a page with sheet fields, the AI Import review
row can set each number beside what your published formulas give and let the
director take the formula's. Chronicle does no arithmetic: your package adds
the check, keyed by sheet kind, from any script loaded for the campaign (text
renderer or widget):

```js
(Chronicle.sheetChecks = Chronicle.sheetChecks || {}).creature = {
  check: function (fields, env) {
    // fields: the sheet as the row would save it (on an update, the page's
    //   own fields with the AI's laid over them), keyed by field key.
    // env.campaignId: for fetching your data files.
    // env.written: the field keys the AI wrote.
    return Promise.resolve({
      note: 'Level 1 Platoon Harrier',            // optional line above the tiles
      items: [
        { id: 'stamina', label: 'Stamina', ai: '20', formula: '30', matches: false }
      ],
      patch: function (ids) { return { stamina: 30 }; }
    });
  }
};
```

- The kind is `creature` for a creature category or one of its sub-types,
  otherwise the category's preset `category` (`character`, …).
- Report only numbers the AI wrote (`env.written`) that a published formula
  covers. A number with no formula is left off, not shown as a guess. Return
  nothing, or no items, to leave the row as it is.
- `ai` and `formula` are display text and are never read as HTML. An item
  that differs needs a `formula`.
- `patch(ids)` gets the ids the director took the formula for and returns
  the field values to save, keyed by field key. Chronicle replaces only keys
  the AI wrote, then checks the result against the category's fields like any
  other `fields:`, so a patch can never add a field.
- A check that throws or rejects leaves the row without tiles; the import
  goes on.

### Server-side rendering

Server-side work (loading sibling entities, conditional dispatch on entity
data) is in-tree Go, not something a package ships; a widget fetches what it
needs from the API.
