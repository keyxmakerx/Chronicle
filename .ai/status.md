# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-24 -- Sprint 5: Polish. hx-boost sidebar navigation, "View as player"
toggle, widget lifecycle cleanup.

## Current Phase
**Phase D: COMPLETE.** Sprint 5 (Polish) wraps up Phase D. Phase E next.

## Sprint 5: Polish (2026-02-24)

### hx-boost Sidebar Navigation
- Added `hx-boost="true" hx-target="#main-content" hx-select="#main-content"
  hx-swap="innerHTML show:window:top"` to the sidebar `<nav>` and admin sidebar.
- Category drill-down links, custom links, and context-switching links ("All
  Campaigns", "Discover") have `hx-boost="false"` to prevent incorrect swaps.
- **boot.js** active link highlighting: `updateSidebarActiveLinks()` uses
  longest-prefix-match to determine which sidebar link should be active after
  a boosted navigation. Listens for `htmx:pushedIntoHistory` and `popstate`.
- **Mobile sidebar auto-close:** Dispatches `chronicle:navigated` custom event
  that the Alpine.js layout component catches to close the mobile sidebar.
- **sidebar_drill.js** closes drill-down panel on `chronicle:navigated`.

### Widget Lifecycle Cleanup
- **tag_picker.js**: Fixed `closeHandler` leak — stores reference on element,
  removed in `destroy()` if dropdown was open during teardown.
- **image_upload.js**: Added `destroy()` method for clean teardown.
- **notes.js**: Added `chronicle:navigated` listener to detect entity context
  changes during hx-boost navigation. Destroys and re-mounts the widget with
  the correct `data-entity-id` when the URL changes. Handler reference stored
  and cleaned up in `destroy()`.
- **entity_tooltip.js / editor.js / editor_mention.js**: Confirmed as false
  positives — global event delegation and Ctrl+S handler are by design.

### "View as Player" Toggle
- **Cookie-based toggle**: `chronicle_view_as_player` cookie — no session or
  Redis changes needed. LayoutInjector reads cookie and overrides template role.
- **LayoutInjector**: When owner has toggle active, `GetCampaignRole(ctx)` returns
  `RolePlayer` instead of `RoleOwner`. New `IsOwner(ctx)` always returns actual
  ownership for toggle button rendering. Entity counts also use effective role.
- **Topbar button**: Eye/eye-slash icon, amber highlight when active, HTMX POST
  to toggle endpoint with `HX-Refresh: true` for full re-render.
- **Banner**: Amber notification bar below topbar: "Viewing as player — owner-only
  content is hidden."
- **Endpoint**: `POST /campaigns/:id/toggle-view-mode` (owner-only). Toggles cookie,
  returns HTMX refresh or redirect.
- **Context helpers**: `SetViewingAsPlayer`/`IsViewingAsPlayer`, `SetIsOwner`/`IsOwner`
  in `internal/templates/layouts/data.go`.
- **Phase 1 scope**: Template-level visibility only (sidebar links, dashboard blocks,
  entity counts, DM-only sections). Handler-level privacy filtering (is_private
  entities in lists) is a future enhancement.

### In Progress
- Nothing — Sprint 5 complete.

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
1. **Phase E:** Quick Search (Ctrl+K), Entity Nesting (parent_id UI), Backlinks,
   API documentation.
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
