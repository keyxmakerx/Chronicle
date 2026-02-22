# Chronicle Phase D: Campaign Customization Hub, Player Notes Overhaul, Bug Fixes

## Context

Chronicle has reached a point where the core infrastructure (auth, campaigns,
entities, template editor, extensions) is solid, but the campaign owner
experience lacks a centralized customization hub. Configuration is scattered
across entity type config pages, campaign settings, and sidebar config. Three
things need to happen:

1. **Campaign Customization Hub** ("Template Editor") — Centralized page for
   campaign owners to control navigation, dashboards, entity templates, and
   category layouts. **HIGHEST PRIORITY.**
2. **Player Notes Overhaul** — Transform per-user quick-notes into shared
   collaborative rich-text with edit locking, history, and template block mount.
3. **Admin Panel Flickering** — Fix Alpine.js reinit flicker + QoL.

### Decisions Made
- **Edit sync:** Pessimistic locking (one editor at a time, 5-min auto-expiry,
  owner force-unlock). Simplest, zero conflict risk. Real-time collab later.
- **Hub location:** Separate `/campaigns/:id/customize` page. Settings keeps
  name/description/members/API keys. Customize handles layout/appearance.
- **Dashboard blocks:** Start minimal (6 core blocks), expand based on usage.
- **Plan saved to:** `.ai/phase-d-plan.md` for permanent reference.

---

## Changes by Component Tier

All changes categorized by Chronicle's three-tier extension architecture plus
core. **Notes is a Widget (not Plugin)** — it has minimal backend and lives
primarily in `static/js/widgets/`. It is gated by the `player-notes` addon
(extension).

| Component | Tier | Changes |
|-----------|------|---------|
| **campaigns** | Plugin | Customize page, dashboard layout CRUD, nav editor |
| **entities** | Plugin | Category dashboard layout CRUD |
| **notes** | Widget (addon-gated) | Collab, locking, versions, rich text, embed mode |
| **sidebar_config** | Widget | Custom sections + links in nav editor |
| **dashboard_editor** | Widget (NEW) | Drag-and-drop dashboard layout builder |
| **template_editor** | Widget | Add `player_notes` block to palette |
| **app layout** | Core (templates) | x-cloak fix, custom nav rendering, hx-boost |
| **sidebar_drill** | Core (JS) | Remove debug console.log statements |
| **input.css** | Core (CSS) | `[x-cloak]` rule, dashboard editor styles |

---

## Interactive Features Reference Table

All interactive text/UI features currently in Chronicle and their status:

| Feature | Files | Tier | Status |
|---------|-------|------|--------|
| **@Mentions** | `editor_mention.js`, `entities/handler.go` (SearchAPI) | Widget | Working |
| **Entity Tooltips** | `entity_tooltip.js`, `entities/handler.go` (PreviewAPI) | Widget | Working |
| **Rich Text Editor** | `editor.js` (TipTap-based) | Widget | Working |
| **Relations** | `relations.js`, `entities/handler.go` | Widget | Working |
| **Tag Picker** | `tag_picker.js`, `tags/handler.go` | Widget | Working |
| **Attributes** | `attributes.js`, field overrides | Widget | Working |
| **Image Upload** | `image_upload.js`, `media/handler.go` | Widget | Working |
| **Template Editor** | `template_editor.js` | Widget | Working |
| **Entity Type Editor** | `entity_type_editor.js` | Widget | Working |
| **Sidebar Config** | `sidebar_config.js` | Widget | Working |
| **Notes** | `notes.js` | Widget | Working (overhaul in this phase) |
| **Sidebar Drill-Down** | `sidebar_drill.js` | Core JS | Working |
| **Dark Mode Toggle** | `theme.js` | Core JS | Working |
| **Toast Notifications** | `Chronicle.notify()` | Core JS | Working |
| **Widget Auto-Mount** | `boot.js` | Core JS | Working |

### How They Work Together
- Mention links automatically get `data-entity-preview` attributes for tooltips
- Relation links also support tooltips via the same attribute
- Preview API serves lightweight entity data (cached 60s)
- Search API supports JSON (widgets) and HTML (HTMX) responses
- All widgets follow boot.js auto-mount pattern via `data-widget` attributes

---

## Cleanup Items (Pre-Sprint)

### Sidebar Debug Logs to Remove
`static/js/sidebar_drill.js` has 3 `console.log()` calls that should be
removed (lines 21, 65, 70). The `console.warn()` on lines 59 and 108 are
error diagnostics and should stay.

```
Line 21: console.log('[sidebar_drill] script loaded, readyState=...')  ← REMOVE
Line 65: console.log('[sidebar_drill] init: found N category links')   ← REMOVE
Line 70: console.log('[sidebar_drill] click: ...')                     ← REMOVE
```

---

## Documentation Requirements

Every sprint must include:
1. **Code comments** — Package-level, exported types, non-obvious logic blocks
2. **AI docs** — Update `.ai/status.md` and `.ai/todo.md` at end of each sprint
3. **ADRs** — Record architecture decisions in `.ai/decisions.md`
4. **Plugin/widget .ai.md** — Create for any new component, update for modified
5. **Data model** — Update `.ai/data-model.md` for new columns/tables

---

## Competitive Research Summary

### World Anvil
- 28+ article templates with built-in worldbuilding prompts
- Creative Studio for custom stat blocks and character sheets
- Visual editor with @mentions, "/" slash commands, drag-and-drop images
- Dashboard with progress tracking, quick-edit buttons, draft filtering
- Content sharing via subscriber groups and role-based access

### Kanka
- **Dashboard widgets** (6 types): Entity List, Entity Preview, Calendar, Header Text, Random Entity, Campaign Header
- **Drag-and-drop** widget positioning; rows with 1-12 column grid
- **Premium: multiple dashboards** per campaign (role-specific — e.g., player dashboard vs DM dashboard)
- Custom CSS per campaign; theme builder for non-coders
- **Custom modules** (entity types) since v3.0 (Feb 2025) — up to 5 per premium campaign
- Module disable/enable per campaign

### LegendKeeper
- **Real-time collaborative editing** with multiplayer cursors
- **Offline editing** with graceful conflict merging on reconnect
- **Version tracking** — save/compare multiple document versions
- **Page locking** — prevent edits until owner unlocks
- **Inline secrets** — hidden text blocks within shared pages, invisible even to editors
- **Sub-templates** — creating a "Town" page auto-creates Tavern, Inn, Shop child pages
- **Page Properties** panel (right sidebar: media, properties, metadata)
- **Tag Index** blocks — auto-generated list of pages matching a tag
- **Multi-column layouts** in the editor (no CSS required)
- **Boards** — collaborative whiteboards with shapes, sticky notes, page card links

### Features We Should Adopt (This Phase)
| Feature | Inspired By | Priority |
|---------|-------------|----------|
| Dashboard widget system with drag-and-drop | Kanka | **High** |
| Role-specific dashboard views (player vs owner) | Kanka | Medium |
| Page/edit locking | LegendKeeper | **High** (for notes) |
| Version history / "last edited by" | LegendKeeper | **High** (for notes) |
| "View as player" toggle for owners | Kanka | Medium |
| Custom nav sections with dividers/links | Kanka/WA | Medium |
| Breadcrumb navigation | General QoL | Low |
| Quick search (Ctrl+K) | LegendKeeper | Low (future) |

### Features to Defer (Future Phases)
- Real-time multiplayer cursors (complex WebSocket infra)
- Offline editing (service worker complexity)
- Sub-templates (nice but not core)
- Collaborative whiteboards/boards
- Custom CSS per campaign
- Slash commands in editor

---

## Workstream 1: Campaign Customization Hub (HIGHEST PRIORITY)

### Goal
Create a centralized `/campaigns/:id/customize` page for campaign owners that consolidates all campaign appearance and layout controls into one place with a tabbed interface.

### Tabs
1. **Navigation** — Sidebar item management (reorder, hide/show, custom sections/links)
2. **Dashboard** — Campaign dashboard layout builder (drag-and-drop widgets)
3. **Categories** — Entity type management grid linking to per-type config
4. **Category Dashboards** — Per-category landing page layout builder

### 1A. Navigation Editor Tab

**What exists:** `sidebar_config` JSON column on campaigns table with `EntityTypeOrder` and `HiddenTypeIDs`. Sidebar config widget in `static/js/widgets/sidebar_config.js`.

**What to add:**
- Custom nav sections (dividers with labels)
- Custom links (name + URL, internal or external)
- "View as player" preview toggle

**Data model change:**
```go
// Extend SidebarConfig in internal/plugins/campaigns/model.go
type SidebarConfig struct {
    EntityTypeOrder []string          `json:"entity_type_order"`
    HiddenTypeIDs   []string          `json:"hidden_type_ids"`
    CustomSections  []NavSection      `json:"custom_sections,omitempty"`  // NEW
    CustomLinks     []NavLink         `json:"custom_links,omitempty"`     // NEW
}

type NavSection struct {
    ID    string `json:"id"`
    Label string `json:"label"`
    After string `json:"after"` // ID of entity type this appears after
}

type NavLink struct {
    ID       string `json:"id"`
    Label    string `json:"label"`
    URL      string `json:"url"`
    Icon     string `json:"icon"`
    Section  string `json:"section"`  // Which section it belongs to, empty = top level
    Position int    `json:"position"`
}
```

**No migration needed** — `sidebar_config` is already a JSON column. Just extend the JSON structure.

**Files to modify:**
- `internal/plugins/campaigns/model.go` — Add NavSection, NavLink structs
- `internal/plugins/campaigns/handler.go` — Add Customize handler method
- `internal/plugins/campaigns/routes.go` — Add `/campaigns/:id/customize` route
- `internal/templates/layouts/app.templ` — Render custom sections/links in CampaignSidebarNav
- `static/js/widgets/sidebar_config.js` — Extend with section/link management UI
- New: `internal/plugins/campaigns/templates/customize.templ` — Hub page template

### 1B. Campaign Dashboard Editor Tab

**What exists:** Campaign dashboard is hardcoded in `internal/plugins/campaigns/show.templ` with quick actions, category grid, and recent pages sections.

**What to build:** A configurable dashboard using a widget-block system (inspired by Kanka's dashboard widgets).

**Data model:**
```go
// New column on campaigns table
// Migration: ALTER TABLE campaigns ADD COLUMN dashboard_layout JSON DEFAULT NULL;

type DashboardLayout struct {
    Rows []DashboardRow `json:"rows"`
}

type DashboardRow struct {
    ID      string            `json:"id"`
    Columns []DashboardColumn `json:"columns"`
}

type DashboardColumn struct {
    ID     string           `json:"id"`
    Width  int              `json:"width"` // 1-12 grid
    Blocks []DashboardBlock `json:"blocks"`
}

type DashboardBlock struct {
    ID     string         `json:"id"`
    Type   string         `json:"type"` // See block types below
    Config map[string]any `json:"config,omitempty"`
}
```

**Dashboard block types (minimal set — expand later):**
| Block Type | Description | Config Options |
|------------|-------------|----------------|
| `welcome_banner` | Campaign name + description hero | background_color |
| `category_grid` | Quick-nav grid of entity types | columns (2-5) |
| `recent_pages` | Recently updated entities | limit (4-12) |
| `entity_list` | Filtered entity list by category | entity_type_id, sort, limit |
| `text_block` | Custom rich text / markdown | content (HTML string) |
| `pinned_pages` | Pinned entities grid | entity_ids |

**New migration: `000021_add_dashboard_layout.up.sql`**
```sql
ALTER TABLE campaigns ADD COLUMN dashboard_layout JSON DEFAULT NULL;
```

**Rendering logic:**
- If `dashboard_layout` is NULL, render the current hardcoded default layout (backwards compatible)
- If set, iterate rows → columns → blocks and render each block type via a Templ switch
- Each block type gets its own Templ component in `internal/plugins/campaigns/templates/dashboard_blocks/`

**Widget:**
- New: `static/js/widgets/dashboard_editor.js` — Reuses drag-and-drop patterns from `template_editor.js`
- Palette shows dashboard block types; canvas shows the current layout
- Save endpoint: `PUT /campaigns/:id/dashboard-layout`

**Files to create/modify:**
- `db/migrations/000021_add_dashboard_layout.up.sql` — Migration
- `internal/plugins/campaigns/model.go` — DashboardLayout, DashboardRow, etc.
- `internal/plugins/campaigns/repository.go` — GetDashboardLayout, UpdateDashboardLayout
- `internal/plugins/campaigns/service.go` — Dashboard layout CRUD + validation
- `internal/plugins/campaigns/handler.go` — Dashboard layout endpoints + customize page
- `internal/plugins/campaigns/show.templ` — Render from layout JSON or fallback to default
- New: `internal/plugins/campaigns/templates/customize.templ` — Customization hub page
- New: `internal/plugins/campaigns/templates/dashboard_blocks.templ` — Block components
- New: `static/js/widgets/dashboard_editor.js` — Dashboard editor widget

### 1C. Categories Tab

**What exists:** Entity type management grid on campaign settings page. Each type links to `/campaigns/:id/entity-types/:etid/config` (unified config page with Layout, Attributes, Dashboard, Nav Panel tabs).

**What to build:** The Categories tab in the customization hub will display the entity type grid with "Configure" links (similar to what's on campaign settings now) plus a "Create New Category" button. This is mostly a relocation of existing UI.

**Files to modify:**
- `internal/plugins/campaigns/templates/customize.templ` — Include category management grid

### 1D. Category Dashboard Editor Tab

**What exists:** Category dashboards are hardcoded in `internal/plugins/entities/category_dashboard.templ` with header, description, pinned pages, and entity grid.

**What to build:** Per-category dashboard layout customization, similar to campaign dashboard but scoped to entity types.

**Data model:**
```sql
-- Migration: add to existing entity_types table
ALTER TABLE entity_types ADD COLUMN dashboard_layout JSON DEFAULT NULL;
```

**Rendering logic:**
- If `dashboard_layout` is NULL on the entity type, render current hardcoded default
- If set, render from JSON layout (same DashboardBlock types plus category-specific ones)

**Additional category block types:**
| Block Type | Description |
|------------|-------------|
| `category_header` | Type icon + name + page count |
| `category_description` | Rich text description |
| `pinned_pages` | Pinned entity grid |
| `entity_grid` | All entities in this category |
| `entity_table` | Table view of entities |
| `tag_filter` | Tag filter bar |

**Files to create/modify:**
- `db/migrations/000021_add_dashboard_layout.up.sql` — Add column to entity_types too
- `internal/plugins/entities/model.go` — Add DashboardLayout field to EntityType
- `internal/plugins/entities/repository.go` — GetCategoryDashboardLayout, UpdateCategoryDashboardLayout
- `internal/plugins/entities/handler.go` — Category dashboard layout endpoints
- `internal/plugins/entities/category_dashboard.templ` — Render from layout or fallback

### 1E. Customization Hub Page Structure

**Route:** `GET /campaigns/:id/customize`
**Permission:** `RoleOwner` only

**Template structure:**
```
┌─────────────────────────────────────────────┐
│  Campaign Customization                     │
│  ─────────────────────────────────────────  │
│  [Navigation] [Dashboard] [Categories]      │
│  [Category Dashboards]                      │
│                                             │
│  ┌─────────────────────────────────────┐   │
│  │                                     │   │
│  │  (Tab content rendered here)        │   │
│  │                                     │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

**Alpine.js tabs** (same pattern as entity type config page):
```html
<div x-data="{ tab: 'navigation' }">
  <button @click="tab = 'navigation'" :class="...">Navigation</button>
  <button @click="tab = 'dashboard'" :class="...">Dashboard</button>
  <button @click="tab = 'categories'" :class="...">Categories</button>
  <button @click="tab = 'category-dashboards'" :class="...">Category Dashboards</button>

  <div x-show="tab === 'navigation'">... nav editor widget ...</div>
  <div x-show="tab === 'dashboard'">... dashboard editor widget ...</div>
  <!-- etc -->
</div>
```

**Sidebar link:** Add "Customize" link to CampaignSidebarNav (visible to owners only), positioned above Settings.

---

## Workstream 2: Player Notes Overhaul

### Goal
Transform the per-user quick-notes floating panel into a shared, collaborative rich-text note system with edit locking, version history, and the ability to mount as a template block on entity pages.

### 2A. Data Model Changes

**New migration: `000022_notes_collaboration.up.sql`**
```sql
-- Add collaboration columns to notes table
ALTER TABLE notes
  ADD COLUMN is_shared BOOLEAN DEFAULT FALSE,
  ADD COLUMN last_edited_by CHAR(36) DEFAULT NULL,
  ADD COLUMN locked_by CHAR(36) DEFAULT NULL,
  ADD COLUMN locked_at TIMESTAMP NULL DEFAULT NULL,
  ADD COLUMN entry JSON DEFAULT NULL,
  ADD COLUMN entry_html TEXT DEFAULT NULL;

-- Note version history table
CREATE TABLE note_versions (
    id          CHAR(36) PRIMARY KEY,
    note_id     CHAR(36) NOT NULL,
    user_id     CHAR(36) NOT NULL,
    content     JSON NOT NULL,
    entry       JSON DEFAULT NULL,
    entry_html  TEXT DEFAULT NULL,
    created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_note_versions_note (note_id),
    FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
);

-- Add index for lock cleanup
CREATE INDEX idx_notes_locked ON notes(locked_by, locked_at);
```

**Updated Note model:**
```go
type Note struct {
    ID           string     `json:"id"`
    CampaignID   string     `json:"campaign_id"`
    UserID       string     `json:"user_id"`       // Creator
    EntityID     *string    `json:"entity_id"`
    Title        string     `json:"title"`
    Content      []Block    `json:"content"`        // Legacy block content
    Entry        *string    `json:"entry"`          // ProseMirror JSON (rich text)
    EntryHTML    *string    `json:"entry_html"`     // Rendered HTML
    Color        string     `json:"color"`
    Pinned       bool       `json:"pinned"`
    IsShared     bool       `json:"is_shared"`      // NEW: visible to all campaign members
    LastEditedBy *string    `json:"last_edited_by"` // NEW: user ID
    LockedBy     *string    `json:"locked_by"`      // NEW: user ID holding edit lock
    LockedAt     *time.Time `json:"locked_at"`      // NEW: when lock was acquired
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}

type NoteVersion struct {
    ID        string    `json:"id"`
    NoteID    string    `json:"note_id"`
    UserID    string    `json:"user_id"`
    Content   []Block   `json:"content"`
    Entry     *string   `json:"entry"`
    EntryHTML *string   `json:"entry_html"`
    CreatedAt time.Time `json:"created_at"`
}
```

### 2B. Edit Locking System

**Approach: Pessimistic locking** (simpler than real-time merge, aligns with LegendKeeper's page locking)

**Flow:**
1. Player clicks "Edit" on a shared note
2. Backend attempts `UPDATE notes SET locked_by = ?, locked_at = NOW() WHERE id = ? AND (locked_by IS NULL OR locked_at < NOW() - INTERVAL 5 MINUTE)` (auto-expire stale locks after 5 min)
3. If lock acquired: return edit mode with TipTap editor. Note card shows
   "Editing..." badge with the editor's display name visible to all viewers
4. If lock denied: show "Currently being edited by **[Player Name]**" with
   the lock holder's display name, avatar, and time since lock acquired.
   Other players see who is editing at a glance without clicking anything.
5. Heartbeat: client sends `POST /notes/:id/heartbeat` every 60s to keep
   lock alive (updates `locked_at`). Response includes current lock holder
   name for UI freshness.
6. On save or "Done": release lock `UPDATE notes SET locked_by = NULL,
   locked_at = NULL`. The "Editing..." badge disappears for all viewers.
7. **Owner override:** Campaign owner can `POST /notes/:id/force-unlock`
   to kick an editor (RoleOwner only). Shows confirmation dialog first.
8. **"Last edited by"** display: every note card shows "Last edited by
   **[Player Name]** · 2 hours ago" in the card footer. Populated from
   `last_edited_by` + `updated_at` columns.

**API endpoints (new):**
```
POST /campaigns/:id/notes/:noteId/lock       — Acquire edit lock
POST /campaigns/:id/notes/:noteId/unlock     — Release edit lock
POST /campaigns/:id/notes/:noteId/heartbeat  — Keep lock alive
POST /campaigns/:id/notes/:noteId/force-unlock — Owner kicks editor (RoleOwner)
GET  /campaigns/:id/notes/:noteId/versions   — List version history
GET  /campaigns/:id/notes/:noteId/versions/:vid — Get specific version
```

### 2C. Rich Text Integration

**Reuse** the existing TipTap editor from `static/js/widgets/editor.js`:
- Same formatting toolbar (bold, italic, underline, headings, lists, @mentions)
- Same ProseMirror JSON storage format
- Same `entry` + `entry_html` dual-column pattern (entities use this too)
- Same autosave mechanism
- Same view/edit toggle pattern

**Implementation:**
- When editing a note in the expanded panel, mount a TipTap instance
- Toolbar is compact (subset of full editor toolbar — fits in 320-400px panel width)
- Save serializes ProseMirror JSON to `entry` and rendered HTML to `entry_html`
- Legacy notes with only `content` blocks continue to work (render as simple text)
- New notes created via quick-add start as simple text; can be "upgraded" to rich text by clicking Edit

### 2D. Version History

**On every save:**
1. Create a `NoteVersion` record with the previous content snapshot
2. Limit to 50 versions per note (delete oldest when exceeded)
3. Store who made the change (`user_id`)

**UI:**
- "History" button in note card header (clock icon)
- Opens a version list panel showing: username, timestamp, preview snippet
- Click a version to view its content
- "Restore" button to revert to that version (creates a new version first)

### 2E. Template Block Mount

**Goal:** Player Notes can be added as a block in the template editor, so notes appear on entity profile pages.

**Implementation:**
- Add `player_notes` to the template editor palette as a new content block type
- When rendered on an entity page, shows the shared notes scoped to that entity
- Uses the same notes widget JS but in "embedded" mode (no floating panel, inline)
- Block config: `{ type: "player_notes", config: {} }`

**Files to create/modify:**
- `db/migrations/000022_notes_collaboration.up.sql`
- `internal/widgets/notes/model.go` — Add new fields, NoteVersion struct
- `internal/widgets/notes/repository.go` — Lock/unlock, versions CRUD, shared note queries
- `internal/widgets/notes/service.go` — Lock logic, version management, force-unlock
- `internal/widgets/notes/handler.go` — New endpoints (lock, unlock, heartbeat, versions)
- `internal/widgets/notes/routes.go` — Register new routes
- `internal/widgets/notes/service_test.go` — Add tests for locking, versions, shared notes
- `static/js/widgets/notes.js` — Rich text mode, lock UI, version panel, embedded mode
- `static/js/widgets/template_editor.js` — Add `player_notes` to palette

---

## Workstream 3: Admin Panel Flickering Fix + QoL

### 3A. Admin Panel Flickering — Root Cause & Fix

**Root cause:** When navigating between pages (including admin pages), the full page including sidebar is re-rendered. This causes:
1. Alpine.js on `AdminSidebarNav` reinitializes, briefly reading localStorage and re-applying state
2. The CSS `.admin-slide` transition from `0fr` to `1fr` replays
3. `sidebar_drill.js` reinitializes all event listeners

**Fix approach:** Use HTMX `hx-boost` + `hx-target` to swap only the main content area instead of the full page.

**Implementation:**
1. Add `hx-boost="true"` and `hx-target="#main-content"` and `hx-swap="innerHTML"` to sidebar navigation links
2. Add `hx-push-url="true"` to maintain browser history
3. Modify admin handlers to detect `HX-Request` header and return just the content (without layout wrapper) when it's an HTMX request
4. Add `hx-preserve` attribute to the sidebar element to prevent it from being touched during swaps
5. Alternative simpler fix: Add `x-cloak` to the admin-slide div and ensure Alpine.js state is applied before first paint

**Simpler immediate fix (do this first):**
```html
<!-- In AdminSidebarNav, add x-cloak to prevent flash of unstyled content -->
<div class="admin-slide" :class="open && 'expanded'" x-cloak
     :style="open ? 'grid-template-rows: 1fr' : 'grid-template-rows: 0fr'">
```
And add to CSS:
```css
[x-cloak] { display: none !important; }
```
This prevents the element from being visible until Alpine.js has initialized and applied the correct state.

**Full fix (hx-boost):**
- `internal/templates/layouts/app.templ` — Add `hx-boost` attributes to sidebar links
- `internal/plugins/admin/handler.go` — Check `IsHTMX()` and return fragment vs full page
- `static/css/input.css` — Add `[x-cloak]` rule

**Files to modify:**
- `internal/templates/layouts/app.templ` — `x-cloak` on admin-slide, `hx-boost` on links
- `static/css/input.css` — Add `[x-cloak]` rule if not present
- `internal/plugins/admin/handler.go` — HTMX fragment detection (optional, for hx-boost)

### 3B. Other QoL Issues to Address

1. **Active link highlighting during HTMX navigation** — When using hx-boost, the sidebar active states won't update because the sidebar isn't re-rendered. Need to add JS that updates active class based on `htmx:afterSettle` event and current URL.

2. **Sidebar drill-down state preservation** — When navigating within a category, the drill-down panel should stay open. Check if `sidebar_drill.js` properly detects the current URL on init.

3. **Notes panel z-index** — Verify notes panel doesn't overlap modals or dropdowns unexpectedly.

---

## Implementation Order

### Sprint 1: Quick Wins
1. **Admin flickering fix** (x-cloak + CSS) — 1 file change
2. **Customization hub route + page shell** — New templ page with 4 empty tabs
3. **Navigation tab** — Move sidebar config widget into hub, extend with sections/links

### Sprint 2: Dashboard Editor
4. **Migration 000021** — `dashboard_layout` on campaigns + entity_types
5. **Dashboard editor widget** — New JS widget (based on template_editor.js patterns)
6. **Dashboard block rendering** — Templ components for each block type
7. **Campaign dashboard renders from layout** — Fallback to hardcoded default

### Sprint 3: Category Dashboards
8. **Category dashboard editor** — In the "Category Dashboards" tab (reuses dashboard_editor.js)
9. **Category dashboard renders from layout** — Fallback to hardcoded default

### Sprint 4: Player Notes Overhaul
10. **Migration 000022** — Notes collaboration columns + note_versions table
11. **Edit locking backend** — Lock/unlock/heartbeat/force-unlock endpoints
12. **Rich text integration** — TipTap in notes panel, entry/entry_html storage
13. **Shared notes** — is_shared flag, campaign-wide visibility
14. **Version history** — Backend + UI for viewing/restoring versions
15. **Template block** — Player notes as draggable block in template editor

### Sprint 5: Polish
16. **hx-boost sidebar navigation** — Prevent full page reloads
17. **"View as player" toggle** — Owner preview mode
18. **Testing** — Unit tests for new service methods, integration tests for locking

---

## Key Files Summary

### New Files
| File | Purpose |
|------|---------|
| `db/migrations/000021_add_dashboard_layout.up.sql` | Dashboard layout columns |
| `db/migrations/000021_add_dashboard_layout.down.sql` | Rollback |
| `db/migrations/000022_notes_collaboration.up.sql` | Notes collab columns + versions table |
| `db/migrations/000022_notes_collaboration.down.sql` | Rollback |
| `internal/plugins/campaigns/templates/customize.templ` | Customization hub page |
| `internal/plugins/campaigns/templates/dashboard_blocks.templ` | Dashboard block components |
| `static/js/widgets/dashboard_editor.js` | Dashboard layout editor widget |

### Modified Files
| File | Changes |
|------|---------|
| `internal/plugins/campaigns/model.go` | NavSection, NavLink, DashboardLayout structs |
| `internal/plugins/campaigns/handler.go` | Customize handler, dashboard layout endpoints |
| `internal/plugins/campaigns/routes.go` | New routes |
| `internal/plugins/campaigns/repository.go` | Dashboard layout CRUD |
| `internal/plugins/campaigns/service.go` | Dashboard layout validation |
| `internal/plugins/campaigns/show.templ` | Render from layout JSON |
| `internal/plugins/entities/model.go` | DashboardLayout on EntityType |
| `internal/plugins/entities/handler.go` | Category dashboard layout endpoints |
| `internal/plugins/entities/repository.go` | Category dashboard layout CRUD |
| `internal/plugins/entities/category_dashboard.templ` | Render from layout JSON |
| `internal/templates/layouts/app.templ` | Custom nav sections/links, x-cloak fix, hx-boost |
| `static/css/input.css` | x-cloak rule, dashboard editor styles |
| `static/js/widgets/sidebar_config.js` | Extend with sections/links |
| `static/js/widgets/template_editor.js` | Add player_notes block type |
| `internal/widgets/notes/model.go` | Shared, lock, version fields |
| `internal/widgets/notes/repository.go` | Lock/unlock, versions, shared queries |
| `internal/widgets/notes/service.go` | Lock logic, version management |
| `internal/widgets/notes/handler.go` | New endpoints |
| `internal/widgets/notes/routes.go` | New routes |
| `static/js/widgets/notes.js` | Rich text, locking UI, version panel |

---

## Verification Plan

### Admin Flickering Fix
1. Navigate between admin pages — sidebar admin section should NOT flicker
2. Toggle admin section open/closed — verify state persists across navigation
3. Test in both light and dark mode

### Customization Hub
1. Navigate to `/campaigns/:id/customize` as campaign owner
2. **Navigation tab:** Reorder entity types, add custom section, add custom link, verify sidebar updates
3. **Dashboard tab:** Drag blocks from palette, rearrange, save. Navigate to campaign dashboard — verify layout renders
4. **Dashboard tab (null layout):** Delete saved layout — verify default hardcoded dashboard renders
5. **Categories tab:** Verify entity type grid shows, "Configure" links work
6. **Category Dashboards tab:** Select a category, customize its dashboard, verify rendering

### Player Notes
1. Enable player-notes addon for a campaign
2. Create a shared note as player — verify other players can see it
3. Click Edit on shared note — verify lock is acquired
4. As a different player, try to edit same note — verify "locked by [name]" message
5. As owner, force-unlock the note — verify lock is released
6. Edit a note with rich text (bold, lists, @mentions) — verify formatting saves
7. Check version history — verify versions are recorded with correct user
8. Restore a previous version — verify content reverts
9. Wait 5 minutes without heartbeat — verify stale lock auto-expires
10. Add player_notes block to entity template — verify notes appear on entity page

### Run Tests
```bash
make test-unit    # All existing + new tests pass
make lint         # No linting errors
make build        # Binary builds successfully
```

---

## AI Docs to Update After Implementation
- `.ai/status.md` — Update current phase, document what was built
- `.ai/todo.md` — Mark items complete, add new follow-ups
- `.ai/decisions.md` — ADR for dashboard widget system, ADR for pessimistic locking choice
- `.ai/data-model.md` — Document new columns and tables
- New: `internal/plugins/campaigns/templates/.ai.md` — Document customize page
