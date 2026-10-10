# System Package Rendering Contract

This document describes how a Chronicle system package (Draw Steel,
D&D 5.5e, future systems) plugs into Chronicle's entity-show page to
render system-specific layouts. It is the external-facing contract
for system-package authors. Internal-only conventions live in
`.ai/conventions.md` instead.

If you are building a system package and want characters / monsters /
items to render with system-specific design rather than the generic
layout-block fallback, this is your starting point.

## What you ship vs. what the host ships

The host (Chronicle core) ships:

- The `EntityShowRendererRegistry` extension point.
- The dispatch guard in `internal/plugins/entities/show.templ` that
  consults the registry before falling through to the layout-block
  iteration.
- The CSS theming variables (see "Theming contract" below).
- The generic block dispatch system (`BlockRegistry`, the existing
  block types like `title`, `image`, `entry`, `attributes`, etc.) —
  this is the fallback when no renderer is registered.

You (the system package) ship, in `manifest.json` and your widget scripts:

- A widget for each entity-type slug (or preset category) your package
  cares about, plus a `renderers` entry binding the two (see "Declaring
  renderers in `manifest.json`" below). Packages are manifest-only; they
  contain no Go.
- Any data model conventions (which `fields_data` keys, which
  layouts) the widget relies on.
- Optionally: JS widgets referenced from `ext_widget` blocks if you want
  layout-editor-friendly building blocks rather than a single whole-page
  renderer.

The host ships **zero character-specific code**. No `blockStatBlock`,
no `blockHPBar`, no default character layout JSON. Everything system-
shaped is your package's responsibility.

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
}

type EntityShowRenderer func(ctx EntityShowRenderContext) templ.Component

type EntityShowRendererRegistry struct { /* opaque */ }

func NewEntityShowRendererRegistry() *EntityShowRendererRegistry
func (r *EntityShowRendererRegistry) Register(slug string, renderer EntityShowRenderer)
func (r *EntityShowRendererRegistry) Lookup(slug string) (EntityShowRenderer, bool)
```

The registry is keyed on the entity-type **slug**, not on system ID
or campaign-level concepts. A single system package can register
multiple slugs (e.g. `drawsteel-character`, `drawsteel-monster`,
`drawsteel-item`). Each slug gets exactly one renderer.

The `EntityShowRenderContext` mirrors the args of the `EntityShowPage`
templ exactly. Anything the block-dispatch fallback can read, your
renderer can read too — `Ancestors`, `Children`, `CSRFToken`, the
addon flags. If you find yourself needing data the context doesn't
expose, file a request: the host extends the context struct in a
follow-up rather than have you reach into globals.

## How registration works

For an installed package, registration is driven by the manifest. At
startup, and again after every package install or update, the host
walks each loaded system manifest's `renderers` and registers, for each
entry, a Go renderer that emits one widget mount point
(`registerManifestRenderers` in `internal/app/routes.go`). The set is
built in a fresh registry and then published, so in-flight requests never
see a half-built state. Packages do not call `Register` themselves.

The direct `Register(slug, renderer)` call on the registry is for
Chronicle's own in-tree Go code only. Both paths fill the same
`EntityShowRendererRegistry`, and "last registration wins" if two target
the same slug.

## Registration timing

- Renderers register at startup, after `BlockRegistry` is built and before
  the global registry is published.
- Installing or updating a system package rebuilds and publishes a fresh
  registry without a restart; new renderers apply to the next page view.

## Failure modes — exactly one

When you navigate to an entity show page, dispatch goes:

1. Build `EntityShowRenderContext` from the request data.
2. Look up the entity type's slug in the registry.
3. **If a renderer is registered**: render the page using your
   renderer's component. Done.
4. **If no renderer is registered**: fall through to the existing
   layout-block dispatch. The page renders using whatever
   `entityType.Layout.Rows` defines.

That is the full failure-mode table. There is no "renderer crashes,
generic fallback runs" scenario — a panic inside your renderer
propagates through templ's normal error handling and produces a
500 page, same as any other render bug. Don't panic in your
renderer; return a `templ.Component` that displays the error
gracefully if your renderer has a recoverable failure mode.

## Theming contract

The host exposes these CSS variables. Use them in your renderer's
markup so a campaign-level theme override automatically retints
your output without per-renderer code:

| Variable | Use |
|---|---|
| `--color-accent` | Primary brand / interactive color. |
| `--color-accent-hover` | Hover state for interactive elements. |
| `--color-accent-light` | Lighter shade for badges, soft tints. |
| `--color-accent-rgb` | Comma-separated RGB triple for `rgba()` calls. |
| `--color-accent-hover-rgb` | Same, for the hover variant. |
| `--color-accent-light-rgb` | Same, for the light variant. |
| `--font-campaign` | Campaign body font override. |

Plus the canonical Tailwind tokens documented in
`.ai/conventions.md` and `tailwind.config.js`:

- `bg-surface`, `bg-surface-alt`, `bg-surface-raised`, `bg-page`
- `text-fg`, `text-fg-body`, `text-fg-secondary`, `text-fg-muted`,
  `text-fg-faint`
- `border-edge`, `border-edge-light`
- `bg-accent`, `bg-accent-hover`, `text-accent`

Do not define your own brand colors. A campaign owner who sets a
custom accent color expects it to apply across the whole
experience, including your system's renderer.

## What's out of scope (V1)

- **Generic system-shaped blocks** (host-provided `blockStatBlock`,
  resource bars, ability cards). System packages own these.
- **Default character layouts** that ship with the host. Your
  package's renderer is the layout for your slugs.
- **Renderer-level config**. Per-entity state lives in
  `fields_data`; per-campaign state lives in campaign settings.
  The renderer takes a slug and a context, no plugin config.
- **Edit-in-place, dice integration, encounter / session state.**
  These are larger product surfaces this registry does not cover.

## Related extension points

If your needs are simpler than a whole-page renderer, you may not
need this registry at all:

- **Custom block types**: register against the existing
  `BlockRegistry` (`internal/plugins/entities/block_registry.go`)
  and reference them from a layout JSON. This is the right fit
  when you want building blocks the campaign owner can mix and
  match in the layout editor, rather than an opinionated system-
  specific page.
- **JS widgets**: register a JS module with `Chronicle.register()`
  and reference it from an `ext_widget` block. Right fit for
  client-side interactivity (dice rollers, animated cards) that
  doesn't need server rendering.

The slug-keyed registry is for **system-specific page rendering**:
"a Draw Steel character should look like *this*, not like a
collection of generic blocks." If that doesn't match your need,
one of the lighter-weight extension points above probably does.

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

For each entry the host registers a renderer that emits a single
mount point — `<div data-widget="drawsteel-character-card"
data-entity-id="…" data-campaign-id="…">` — and the existing
`boot.js` auto-mounter takes over: it finds the element, calls the
widget's registered `init(el, config)`, and the widget owns the
rest of the page from there. The `data-*` attributes round-trip
into `config.entityId` and `config.campaignId` (see
`static/js/boot.js` for the kebab-to-camel conversion rules).

The mount also carries what the widget needs to draw its own controls, all
`"true"`/`"false"` except the name and visibility:

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

To change the picture, a widget dispatches a bubbling DOM event,
`chronicle:change-image` with `detail: { entityId }`; the page opens its
existing upload for that page, and does nothing for a viewer who may not
change it. After a rename saved from the header, the page dispatches
`chronicle:page-renamed` with `detail: { entityId, name }` for a widget that
draws the name itself.

The host draws the page header (name, favorite, claim marker, Edit name,
History, and Clone and Delete under a more menu) above the widget, and Player
notes, Relations, Tags and Sub-pages below it, so a widget does not draw
those.

### Validation rules (enforced at install time)

A manifest with an invalid `renderers` block is rejected during
package install — the package never lands on disk and never reaches
the renderer registry. Every entry must satisfy:

- **Exactly one** of `slug` or `preset_category` is set (see "Binding a
  renderer by preset category" below). `slug` matches one of this manifest's
  `entity_presets[].slug`; `preset_category` matches one of this manifest's
  `entity_presets[].category`. Cross-manifest references are forbidden — a
  manifest can only register renderers for entity types/categories it owns.
- `widget` matches one of this manifest's `widgets[].slug`. Same
  rule — a renderer must mount a widget the same package ships.
- `slug`/`preset_category` and `widget` are valid slugs (lowercase letters,
  numbers, hyphens, underscores).
- A manifest declares no more than 10 renderers.

### Adding a panel to NPC pages (`entity_panels`)

A renderer replaces a page; a panel adds to one. To offer a widget on NPC or
monster pages in campaigns that have your system enabled, declare it in
`entity_panels`. The panels show where the owner places the "Game System
Panels" block in the page template (and above a renderer's page, which has no
template); nothing is added to a page the owner has not placed it on:

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
- `applies_to` must be `"npc"`, the only audience for now. Any other value is
  rejected at install so a typo is loud. "NPC" means the same page types
  as the NPC gallery: the ones the campaign owner listed as NPCs on the
  Characters page, with their sub-types but not the player-character type, and
  never a page a player has claimed as their character. A system's creature
  types are not NPCs until the owner lists them.
- A manifest declares no more than 10 panels.

The host emits `<div data-widget="…" data-campaign-id="…" data-entity-id="…"
data-system-id="…" data-is-gm="true|false">`. `data-is-gm` is true for the
campaign owner and members granted DM access; use it to decide whether to show
GM-only controls, and never rely on it to protect data. Your widget script is
loaded the same way as any other widget of the enabled system.

To keep state for the panel, use the page-state API (the owner/DM team writes,
anyone who can view the page reads the public half):

- `GET|PUT /campaigns/:id/entities/:eid/system-state/:system/:key` with the
  session. `PUT` takes `{"public": {…}?, "gm": {…}?}`; an absent half keeps what
  is stored and a present half replaces it. Each half must be a JSON object of
  at most 16 KiB. `GET` returns `{systemId, key, public, gm?, isGm, updatedAt}`,
  and `gm` is present only for the DM team.
- `system` is your manifest `id` and must be enabled for the campaign; `system`
  and `key` match `^[a-z0-9][a-z0-9_-]{0,63}$`.
- Writes publish `system_state.updated` (`{systemId, key}`, no content) to the
  GM side only, so a DM's other tabs and the Foundry module can re-read.

### Lifecycle and overrides

Registration happens at boot and again after each package install or update
(see "Registration timing"); no restart is needed.

If two installed manifests declare a renderer for the same slug,
the last one to register wins. Admins control which packages are
installed, so this collision case is rare in practice.

### Binding a renderer by preset category (system-agnostic)

A renderer entry binds by **either `slug` or `preset_category`** — exactly
one (the manifest validator enforces this). `slug` binds your widget to a
specific entity type your package owns. `preset_category` binds it to a
*category* instead, matching one of your `entity_presets[].category` values:

```json
"renderers": [
  { "preset_category": "character", "widget": "character-sheet" }
]
```

The widget then renders **any** entity type carrying that preset category —
not just one bespoke slug. At render time the host resolves **slug first**
(most specific), then falls back to the entity type's `preset_category`, so a
preset binding never shadows another package's slug-bound type. This is the
system-agnostic seam that lets a package fill a category Chronicle core owns
(see "Player-character sheet" below). No DB change — it's manifest JSON only.

### Player-character sheet

Chronicle's **Player Character Claiming** addon owns a player-character
*category* (Chronicle owns the category; the system fills the rendering). To
fill it, a system package ships:

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

   The **slug** form binds the sheet to your own character type — that type
   already renders via it, which is why a package that owns its type's slug
   needs nothing more. The **`preset_category`** form is the modularity
   capability: it renders any `character`-category type, including a generic
   Chronicle-owned "Player Characters" type, without your package owning its
   slug. Ship one or both.

When the addon is enabled in a campaign that has your system, it **nests your
character type under "Characters"** as the player-character sub-category —
without renaming it (your terminology is preserved) and without copying fields
(your type already carries them and renders via your own renderer). A
system-less campaign gets a generic "Player Characters" type instead.

**Modularity — this is the whole point.** A new system (e.g. D&D 5e) gets the
**identical** behavior with **zero Chronicle core changes**: ship a manifest
with a `category: "character"` preset (its fields) and a `character-sheet`
renderer, and the same nesting, the same claiming, and the same sheet rendering
apply. Chronicle core detects "a system character type" generically (by preset
category / claimability) and never references any system by name — every system
specific (fields, widget, renderer, the type's name) lives in the package's
manifest.

### Hero creator

Chronicle's hero creator (Characters → Create hero) needs nothing beyond what
the sheet already uses. It asks one question per **text field** of your
`category: "character"` preset that has entries to pick from (your data file
for that field, plus any entries the campaign's Directors added), in the
order the preset lists its fields. The field's `label` names the step, so
call it "Lineage" and the creator says "Choose your lineage".

Each entry opens into a leaf drawn only from what the entry carries, with no
empty sections:

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

### Server-side rendering

A renderer that needs server-side work (loading sibling entities,
conditional dispatch on entity data) is Chronicle in-tree Go code, not
something a package can ship; a package's widget fetches what it needs
from the API instead.
