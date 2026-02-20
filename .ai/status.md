# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC                                                        -->
<!-- Purpose: Session handoff document. The outgoing AI session writes what    -->
<!--          the incoming session needs to know.                             -->
<!-- Update: At the END of every AI work session.                             -->
<!-- ====================================================================== -->

## Last Updated
2026-02-20 -- Competitor-inspired UI overhaul: terminology rename (Entity→Page, Entity Type→Category), drill-down sidebar, category dashboard pages, tighter card grid.

## Current Phase
**Phase 2: COMPLETE + Polish.** Media & UI work is done. Additional polish pass
completed for dark mode, badge contrast, and admin/campaign settings UX.

## What Was Built in Phase 2 (Summary)

### Core Infrastructure Additions
- **Semantic color system:** CSS custom properties (`--color-text-primary`, etc.)
  with light/dark values on `:root` / `.dark`. Wired into Tailwind as utility
  classes (`text-fg`, `bg-surface`, `border-edge`, etc.). Components auto-switch
  without per-element `dark:` variants.
- **Dark mode toggle:** Theme.js manages `localStorage` + `.dark` class on `<html>`.
  Toggle button in sidebar. All templ files and CSS components use semantic tokens.
- **Toast notification system:** `notifications.js` with `Chronicle.notify(msg, type)`
  API. HTMX integration for auto error toasts. Wired into `base.templ`.

### Plugins Built
- **Media plugin:** File upload with thumbnails, magic byte validation, rate limiting.
- **Audit plugin:** Campaign-scoped activity timeline with stats. Wired into entity,
  campaign, and tag mutation handlers.
- **Site settings plugin:** Global storage limits, per-user/campaign overrides,
  combined Storage + Settings tabbed admin page.
- **Admin modules page:** Module registry with D&D 5e, Pathfinder 2e, Draw Steel
  entries. Card grid UI with status badges.

### Widgets Built
- **Editor widget:** TipTap rich text with view/edit mode toggle (read-only default,
  Edit/Done button), autosave, @mention search popup with keyboard navigation.
- **Attributes widget:** Inline edit UI for all field types (text, number, url,
  textarea, select, checkbox). Full-stack: JS widget + Go API + repo/service.
- **Tag picker widget:** Search, create, assign tags on entity profile pages.
  Tags display on entity list cards with batch fetch and colored chips.
- **Relations widget:** Bi-directional entity linking with common relation types,
  reverse auto-create, and deletion.
- **Template editor:** Drag-and-drop visual page builder with two-column,
  three-column, tabs, sections. Block preview overlay. Context menu.
- **Entity tooltip:** Hover preview popover with image, type badge, excerpt, LRU cache.

### Entity Enhancements
- Entity type CRUD (create, edit, delete, icon/color/fields management)
- Entity list page redesign (horizontal tabs, search bar, stats subtitle)
- Entity image upload pipeline with upload/placeholder UI
- Descriptor rename (Subtype Label -> Descriptor)
- Dynamic sidebar with entity types from DB + count badges
- Sidebar customization (drag-to-reorder, hide/show entity types per campaign)
- Layout-driven entity profile pages (layout_json on entity types)

### UI & Styling
- Visual polish pass (gradient hero, icon cards, refined buttons/cards/inputs)
- CSS component library (btn, card, input, table, badge, empty-state, stat-card)
- All CSS components migrated to semantic tokens (no per-component `dark:`)
- All 20+ templ files migrated to semantic color tokens
- Public landing page with discoverable campaign cards
- Collapsible admin sidebar with modules section

### Security
- Comprehensive security audit (14 vulnerability fixes)
- IDOR protection on all entity endpoints
- HSTS security header
- Rate limiting on auth + uploads
- Storage limit enforcement in media upload handler

### Phase 2 Polish (2026-02-20)
- **Entity type badge contrast:** Luminance-based text color (white/dark) for
  entity type badges in entity cards, profile pages, and tooltips. No more
  white-on-white for light-colored entity types.
- **Dark mode fix for entity type config widget:** Replaced hardcoded gray
  Tailwind classes with semantic tokens (`text-fg-body`, `border-edge`,
  `bg-surface-alt`, etc.) in `entity_type_config.js`.
- **Merged campaign Edit + Settings:** Combined `/campaigns/:id/edit` and
  `/campaigns/:id/settings` into a single unified settings page. Edit form is
  now the top section of settings. Old `/edit` URL redirects to `/settings`.
- **Game Modules in campaign settings:** Campaign owners can now see available
  game modules (from the registry) in their campaign settings page.
- **Admin plugins page:** New `/admin/plugins` page showing all registered
  plugins with active/planned status, category grouping, and descriptions.
  Plugin registry with 11 entries (8 active, 3 planned).

### Phase 3: Competitor-Inspired UI Overhaul (2026-02-20)
- **Terminology rename:** All user-facing labels changed from "Entity/Entity Type"
  to "Page/Category". Go code and DB unchanged. Templates, breadcrumbs, buttons,
  empty states, settings, and dashboard cards all updated.
- **Drill-down sidebar:** iOS Settings-style push navigation. Clicking a category
  slides the main nav left (10px peek), revealing a category-specific sub-panel
  with search, "View All", and "New Page" links. CSS transforms + JS controller
  (`sidebar_drill.js`). Peek zone has hover glow animation for back affordance.
- **Category dashboard pages:** Each category gets a customizable landing page
  with icon header, description, pinned pages section, tag filter bar,
  grid/table view toggle, and tighter card grid. Replaces plain entity grid
  when browsing a specific type.
- **DB migration 000013:** Added `description` (TEXT) and `pinned_entity_ids`
  (JSON) columns to entity_types table.
- **Tighter card spacing:** Card padding, image height, and grid gaps reduced.
  4-column grid on XL screens. Smaller type badges and tag chips.
- **Service/repo updates:** `UpdateDashboard` method added to EntityTypeRepository
  and EntityService interfaces. Handler for `PUT /entity-types/:etid/dashboard`.

### In Progress
- Nothing currently in progress

### Blocked
- Nothing blocked

## Active Branch
`claude/explore-project-soSu8`

## Next Session Should
1. **Run `make templ`, `make tailwind`, and `make migrate-up`** before testing --
   generated files are gitignored, migration 000013 needs to be applied.
2. **Grid/Table view toggle** -- Wire the toggle buttons on category dashboard to
   switch between grid cards and a compact table view.
3. **Sub-folder support** -- Use existing `parent_id` field to show parent/child
   grouping on category dashboards with expand/collapse.
4. **Settings consolidation** -- Move entity type management to a "Navigation &
   Layout" section; add category page layout editor.
5. **Persistent filters** -- Save filter state per category in localStorage.
6. **Tests** -- Many plugins/widgets have no tests yet. Priority: entities service
   (extend existing 30 tests), relations service, tags service, audit service.
7. **Password reset** -- Wire auth password reset with SMTP when configured.
8. **Map viewer** -- Leaflet.js map widget with entity pins.

## Known Issues Right Now
- `make dev` requires `air` to be installed (`go install github.com/air-verse/air@latest`)
- Templ generated files (`*_templ.go`) are gitignored, so `templ generate`
  must run before build on a fresh clone
- Tailwind CSS output (`static/css/app.css`) is gitignored, needs `make tailwind`
- Tailwind standalone CLI (`tailwindcss`) is v3; do NOT use `npx @tailwindcss/cli` (v4 syntax)

## Completed Phases
- **2026-02-19: Phase 0** -- Project scaffolding, AI docs, build config
- **2026-02-19: Phase 1** -- Auth, campaigns, SMTP, admin, entities, editor, UI layouts,
  unit tests, Dockerfile, CI/CD, production deployment, auto-migrations
- **2026-02-19 to 2026-02-20: Phase 2** -- Media plugin, security audit (14 fixes),
  dynamic sidebar, entity images, sidebar customization, layout builder, entity type
  config/color picker, public campaigns, visual template editor, dark mode, tags,
  audit logging, site settings, tag picker, @mentions, entity tooltips, relations,
  entity type CRUD, visual polish, semantic color system, notifications, modules page,
  attributes widget, editor view/edit toggle, entity list redesign
