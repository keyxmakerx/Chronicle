# calendar_v2 widget

Reusable templ + CSS components shared by the calendar plugin's V2
surface and any downstream consumer. Established by Wave 1 PR 4 per
the template-first directive in
`decisions/2026-05-28-cal-timeline-v2-design.md` §"Wave 1 PR 4
template-first directive — reusable widget layer".

## Components

| Component | File | Purpose |
|---|---|---|
| `EventCard` | `event_card.templ` | Single event card; 3 densities (Compact / Standard / Detailed); 3 tier variants (Minor / Standard / Major). |
| `MultiDayRibbon` | `multi_day_ribbon.templ` | Horizontal ribbon spanning grid cells for multi-day events. Reusable for era bands (PR 5), session blocks (Wave 4), encounter timelines. |
| `VisibilityEditor` | `visibility_editor.templ` | Chip-row builder per Q-V2-7. Radio (public vs specific), inline pickers (user + role), allow/deny chips with color side bars, effective-audience summary. Reusable for any per-entity visibility surface. |

## Density + Tier

```go
calwidget.EventCard(
    calwidget.EventCardData{
        ID:            event.ID,
        Name:          event.Name,
        CategoryColor: "#ff8800",
        CategoryName:  "Festival",
        Tier:          calwidget.TierStandard,
        IsPublic:      true,
        StartLabel:    "Mirtul 15",
    },
    calwidget.DensityCompact,
)
```

Densities:
- `DensityCompact` — cell-inline; just `● Event Name` + lock if private.
- `DensityStandard` — card; metadata + category chip + visibility icon.
- `DensityDetailed` — full body; markdown description + visibility editor.

Tiers:
- `TierMajor` — elevation +1, accent ring, bolder border, diamond badge.
- `TierStandard` — default elevation, hollow diamond badge.
- `TierMinor` — 60% opacity, no badge.

## Animation classes

CSS lives in `static/css/input.css` under the `V2 widget animations`
section. Each pattern composes PR #357 motion + elevation tokens and
includes a `@media (prefers-reduced-motion: reduce)` override.

| Class | Use |
|---|---|
| `.lift-hover` | Card hover lift (translateY(-1px) + elev step) |
| `.drawer-slide-in` / `.drawer-slide-out` | Spring-feel right-slide |
| `.ribbon-enter` | Multi-day ribbon mount-in |
| `.drag-ghost` | Applied during HTML5 D&D drag |
| `.drop-receive-pulse` | One-shot pulse on drop |
| `.today-pulse` | View-mount today highlight |
| `.tier-transition` | Smooth color/shadow swap on tier change |
| `.chip-add` / `.chip-remove` | Visibility editor rule chip lifecycle |

## Cross-plugin consumption

Downstream repos (Chronicle-DnD-5.5e, Chronicle-Draw-Steel) and other
plugins import the package directly:

```go
import calwidget "github.com/keyxmakerx/chronicle/internal/widgets/calendar_v2"
```

The CSS animation classes are bundled into Chronicle's main CSS via
`tailwind`. Consumers ship their own CSS bundle should ensure the
animation section is mirrored — or they can import Chronicle's
`static/css/input.css` directly and let Tailwind cascade-resolve.

## Customization-readiness

Every value resolves from tokens:
- Colors: per-campaign accent palette + neutral palette
- Shadows: PR #357 `--elev-*` tokens
- Motion: PR #357 `--motion-*` tokens

Zero hardcoded color hex; zero `transition: all`; zero raw shadow
rules. The customization-readiness CI guard from PR #357 enforces
this; widget contributions must pass.

## Reduced-motion compliance

All animation classes honor `@media (prefers-reduced-motion: reduce)`
per `decisions/2026-05-22-ui-modernization-decisions.md §Q6` and
WCAG 2.3.3. End-states stay correct (cards still elevate; chips still
appear) but the animations don't play for users who opt out.

## Visibility editor — Q-V2-7 chip-row builder

```go
calwidget.VisibilityEditor(calwidget.VisibilityEditorData{
    IsPublic: false,
    Rules: []calwidget.VisibilityRule{
        {Mode: "allow", Kind: "user", Target: "alice", Label: "@alice"},
        {Mode: "deny",  Kind: "role", Target: "player", Label: "Players"},
    },
    AvailableRoles: []calwidget.RoleOption{ /* ... */ },
    AvailableUsers: []calwidget.UserOption{ /* ... */ },
    FieldPrefix:    "event_visibility",
})
```

The hidden input `<input data-visibility-rules-json>` round-trips the
rule list as JSON for form Save. Host pages read the input value on
submit. Effective-audience summary computes client-side; the server
remains the authority on render-time access checks.

## Test coverage

`helpers_test.go` covers:
- Tier class + badge composition (Compact / Standard / Detailed)
- Ribbon grid-column + tint style
- Chip class color treatment (allow=green, deny=amber)
- Chip label formatting (user prefix, role label, explicit override)
- Effective-audience summary across 5 scenarios (public, empty,
  allow-only with cap, deny-only, mixed)
- Rule JSON round-trip
- Specific-panel visibility toggle
- Left-bar color fallback to CSS variable

DB-level + drag-interaction tests live in the consuming plugin
(calendar plugin's `calendar_v2_events_test.go`).
