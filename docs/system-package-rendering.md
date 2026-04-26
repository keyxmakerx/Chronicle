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
