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

You (the system package) ship:

- A renderer function for each entity-type slug your package
  cares about.
- A `Register…` function that wires those renderers into Chronicle's
  registry at startup.
- Any data model conventions (which `fields_data` keys, which
  layouts) the renderer relies on.
- Optionally: custom block types registered against the existing
  `BlockRegistry` if you want layout-editor-friendly building blocks
  rather than a single whole-page renderer.

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

Your system package exposes one function:

```go
// In your system package (e.g. internal/systems/drawsteel/render.go)
package drawsteel

import "github.com/keyxmakerx/chronicle/internal/plugins/entities"

// RegisterEntityShowRenderers wires drawsteel renderers into the
// host registry. Called from internal/app/routes.go during startup.
func RegisterEntityShowRenderers(reg *entities.EntityShowRendererRegistry) {
    reg.Register("drawsteel-character", renderCharacter)
    reg.Register("drawsteel-monster", renderMonster)
}

func renderCharacter(ctx entities.EntityShowRenderContext) templ.Component {
    // Build and return a templ.Component that renders the character
    // sheet. Uses ctx.Entity, ctx.Ancestors, etc.
    return drawSteelCharacterSheet(ctx)
}
```

The host wires the call into `internal/app/routes.go` next to the
existing `BlockRegistry` registrations:

```go
showRegistry := entities.NewEntityShowRendererRegistry()
drawsteel.RegisterEntityShowRenderers(showRegistry)
// dnd5e.RegisterEntityShowRenderers(showRegistry)  // future
entities.SetGlobalEntityShowRendererRegistry(showRegistry)
```

Mirror the established `calendar.RegisterCalendarBlock(blockRegistry)`
pattern. **Do not** register from `init()` — `init()` runs before
the registry exists, and the explicit-call pattern keeps wiring
order obvious in `routes.go`.

## Registration timing — V1 lifecycle

- Renderers register at startup, during `RegisterRoutes`, after
  `BlockRegistry` is built and before the global is set.
- The HTTP server starts only after registration completes.
- The registry is **mutable but not live-reloadable in V1**.
  Installing or disabling a system package requires a restart.
  Live registry mutation may come later if it becomes a real
  product need; design your package assuming restart-required.
- "Last registration wins" — if two registrations target the same
  slug, the later one replaces the earlier. Order is determined by
  the order of calls in `routes.go`.

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
- **Live reload of the registry** — restart-required.
- **Renderer-level config**. Per-entity state lives in
  `fields_data`; per-campaign state lives in campaign settings.
  The renderer takes a slug and a context, no plugin config.
- **Edit-in-place, dice integration, encounter / session state.**
  These are larger product surfaces not part of CH4.

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

## Declaring renderers in `manifest.json` (CH4.5)

The Go-side `Register` call described above is the low-level path. If
all you need is "render this entity type by mounting that widget,"
you don't have to touch Go at all — declare the binding in your
package's `manifest.json` and the host wires it up at boot:

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

### Lifecycle and overrides

CH4.5 uses the same V1 lifecycle as the Go-side path: registration
happens once, at boot, after `loadSystemsFromPackages` runs and
before the registry global is published. Installing a new package
or upgrading one requires a Chronicle restart for the new renderers
to take effect. There is no live reload.

If two installed manifests declare a renderer for the same slug,
the last one to register wins — the underlying registry's
documented "last write wins" semantics. Admins control which
packages are installed, so this collision case is rare in practice;
if you need a deterministic override, use the Go-side `Register`
path and order the registration calls explicitly in
`internal/app/routes.go`.

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

### When to use the Go path instead

The manifest path is the right fit when "this entity type renders
as that widget" is a complete description of what you want. Use
the Go-side `Register` API when you need:

- A renderer that does server-side work before producing markup
  (e.g., loading sibling entities, computing derived values).
- Conditional dispatch based on the entity's data, not just its
  slug.
- A renderer that emits multiple widgets or a non-widget layout.
- Behavior that overrides another package's manifest renderer.

Both paths share the same `EntityShowRendererRegistry` — choose per
renderer based on what each one needs.
