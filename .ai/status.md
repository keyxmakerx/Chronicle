# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-24 -- Phase E: Quick Search + Customization Hub rework.

## Current Phase
**Phase E: Core UX & Discovery.** Quick Search and Customize page rework complete.

## Phase E: Core UX & Discovery (2026-02-24)

### Quick Search (Ctrl+K) — COMPLETE
- **`static/js/search_modal.js`**: Standalone JS module (not a widget — global scope).
  Opens centered modal overlay with search input, result list, keyboard hints.
- **Keyboard shortcut**: Ctrl+K / Cmd+K opens/closes modal. Escape closes.
- **Search**: Uses existing `GET /campaigns/:id/entities/search?q=...` JSON endpoint
  (Accept: application/json). 200ms debounce, AbortController for in-flight cancellation.
- **Results**: Entity type icon + color, entity name, type name. Mouse hover and
  arrow keys for navigation, Enter to open, click to navigate.
- **Topbar**: Replaced inline HTMX search input with a trigger button that calls
  `Chronicle.openSearch()`. Shows search icon, "Search..." label, and Cmd+K kbd hint.
  Works on all screen sizes (responsive).
- **Campaign-scoped**: Extracts campaign ID from URL; modal only opens when in a
  campaign context (pattern: /campaigns/:id/...).
- **Cleanup**: Closes on `chronicle:navigated` (hx-boost navigation).
- **Script loaded**: Added to `base.templ` after all widget scripts.

### Customization Hub Rework — COMPLETE
- **Old structure (5 tabs)**: Navigation, Dashboard, Categories (link grid), Category
  Dashboards, Page Layouts. Categories tab was just links to a separate config page.
  Category Dashboards and Page Layouts duplicated entity type config functionality.
  Attribute field editor was missing entirely.
- **New structure (4 tabs)**:
  1. **Dashboard** — Campaign dashboard editor (unchanged).
  2. **Categories** — Category selector → HTMX lazy-loads identity, attributes, and
     category dashboard for the selected category. Inline editing via Alpine.js + fetch.
  3. **Page Templates** — Category selector → HTMX lazy-loads template-editor (renamed).
  4. **Navigation** — Sidebar ordering + custom links (unchanged).
- **New endpoint**: `GET /campaigns/:id/entity-types/:etid/customize` returns an HTMX
  fragment with Identity card (name/icon/color), Attributes card (entity-type-editor
  fields-only), and Category Dashboard card (description + dashboard-editor widget).
- **Bug fix**: Entity type config page Nav Panel tab used HTMX `hx-put` + `hx-include`
  to send form data, but handler expected JSON. Switched to Alpine.js + fetch() with
  proper JSON body. Same pattern used in new Categories tab identity card.
- **Back link fix**: Entity types management page now links "Back to Customize" instead
  of "Back to Settings".

### In Progress
- Nothing currently in progress.

### Blocked
- Nothing blocked

## Active Branch
`claude/document-architecture-tiers-yuIuH`

## Competitive Analysis & Roadmap
Created `.ai/roadmap.md` with comprehensive comparison vs WorldAnvil, Kanka, and
LegendKeeper. Key findings:
- Chronicle is ahead on page layout editor, dashboards, self-hosting, and modern stack
- Critical gaps: Quick Search (Ctrl+K), entity hierarchy, calendar, maps, inline secrets
- Calendar is identified as a DIRE NEED — Kanka's is the gold standard
- API technical documentation needed for Foundry VTT integration
- Foundry VTT module planned in phases: notes sync → calendar sync → actor sync
- Features organized by tier: Core, Plugin, Module, Widget, External
- Revised priority phases: D (complete) → E (UX) → F (calendar/time) → G (maps) →
  H (secrets) → I (integrations) → J (visualization) → K (delight)

## Next Session Should
1. **Phase E continued:** Entity Nesting (parent_id + tree UI + breadcrumbs),
   Backlinks ("Referenced by" on entity profiles), API documentation.
2. **Phase F:** Calendar plugin (custom months, moons, eras, events, entity linking).
   See `.ai/roadmap.md` for full data model and implementation plan.
3. **Handler-level "view as player":** Extend toggle to filter is_private entities
   at repository level (currently template-only).

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)

## Completed Phases
- **2026-02-19: Phase 0** — Project scaffolding, AI docs, build config
- **2026-02-19: Phase 1** — Auth, campaigns, SMTP, admin, entities, editor, UI layouts,
  unit tests, Dockerfile, CI/CD, production deployment, auto-migrations
- **2026-02-19 to 2026-02-20: Phase 2** — Media plugin, security audit (14 fixes),
  dynamic sidebar, entity images, sidebar customization, layout builder, entity type
  config/color picker, public campaigns, visual template editor, dark mode, tags,
  audit logging, site settings, tag picker, @mentions, entity tooltips, relations,
  entity type CRUD, visual polish, semantic color system, notifications, modules page,
  attributes widget, editor view/edit toggle, entity list redesign
- **2026-02-20: Phase 3** — Competitor-inspired UI overhaul: Page/Category rename,
  drill-down sidebar, category dashboards, tighter cards
- **2026-02-20: Phase B** — Discover page split, template editor block resizing &
  visibility, field overrides, extension framework (addons), sync API plugin with
  admin/owner dashboards, REST API v1 endpoints (read/write/sync)
- **2026-02-20: Phase C** — Player notes widget, terminology standardization
- **2026-02-22 to 2026-02-24: Phase D** — Customization Hub (sidebar config, custom
  nav, dashboard editor, category dashboards, page layouts tab), Player Notes Overhaul
  (shared notes, edit locking, version history, rich text), Sprint 5 polish (hx-boost
  sidebar, widget lifecycle, "view as player" toggle)
