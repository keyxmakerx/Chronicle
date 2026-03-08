# Feature Parity & Completeness Audit

<!-- ====================================================================== -->
<!-- Category: REFERENCE                                                      -->
<!-- Purpose: Comprehensive audit of feature parity, test coverage, JS        -->
<!--          consistency, export gaps, and documentation completeness.       -->
<!-- Created: 2026-03-05                                                     -->
<!-- Update: When fixing audit items, mark them [x] and note the batch.      -->
<!-- ====================================================================== -->

## 1. Test Coverage

**23 test files, ~530 test functions. ALL service-layer mock-based tests.**
No handler tests, no repository tests, no integration tests exist anywhere.

### Plugins

| Plugin | Test File(s) | Tests | Notes |
|--------|-------------|-------|-------|
| campaigns | service_test, import_test, group_service_test | 90 | Best coverage |
| entities | service_test | 50 | |
| settings | service_test | 36 | |
| addons | service_test | 33 | |
| syncapi | service_test | 31 | |
| auth | service_test | 29 | |
| media | service_test, clamav_test | 28 | |
| audit | service_test | 12 | |
| calendar | day_test, week_test | 10 | Domain-logic only; 23+ endpoints untested |
| timeline | connection_test | 3 | Domain-logic only; 20+ endpoints untested |
| **maps** | — | **0** | 27+ endpoints, HIGH priority |
| **sessions** | — | **0** | 8+ endpoints, HIGH priority |
| **admin** | — | **0** | 12+ endpoints, MEDIUM priority |
| **smtp** | — | **0** | 3 endpoints, LOW priority |

### Widgets

| Widget | Tests | Notes |
|--------|-------|-------|
| notes | 32 | |
| tags | 30 | |
| relations | 25 | |
| posts | 13 | |
| attributes | 0 | Frontend-only |
| editor | 0 | Frontend-only |
| mentions | 0 | Frontend-only |
| title | 0 | Pure Templ |

### Core/Infrastructure (zero tests)

- [ ] `internal/apperror/` — error types and `validate.go` helpers
- [ ] `internal/config/` — configuration loading
- [ ] `internal/app/` — bootstrap, route wiring
- [ ] `internal/websocket/` — WebSocket handler
- [ ] `internal/sanitize/` — CDR pipeline, MIME validation
- [ ] `internal/middleware/` — only IDOR has 3 tests; auth, CSRF, role, addon, security middleware all untested

### CI gaps

- [ ] CI runs `go test ./... -v -short` but no test implements `testing.Short()` skip — the flag has no effect
- [ ] `make test-int` target exists but zero `TestIntegration*` functions in codebase
- [ ] No integration test stage in CI pipeline

---

## 2. JS Widget Consistency

### Chronicle.apiFetch() Migration

**Done (8 widgets):** dashboard_editor, sidebar_config, sidebar_nav_editor,
entity_type_config, calendar_widget, map_widget, shop_inventory, timeline_widget

**Not migrated:**

| Widget | Raw fetch() calls | Priority |
|--------|------------------|----------|
| `notes.js` | 10 | HIGH |
| `attributes.js` | 5 | HIGH |
| `relations.js` | 6 | HIGH |
| `tag_picker.js` | 6 | MEDIUM |
| `permissions.js` | 2 | MEDIUM |
| `editor.js` | 2 | MEDIUM |
| `image_upload.js` | 1 | LOW |
| `entity_posts.js` | 1 (local wrapper) | LOW |
| `editor_autolink.js` | 1 | LOW |
| `editor_mention.js` | 1 | LOW |
| `entity_tooltip.js` | 1 | LOW |
| `entity_type_editor.js` | 1 | LOW |
| `groups.js` | 1 (local wrapper) | LOW |
| `relation_graph.js` | 1 | LOW |
| `template_editor.js` | 1 | LOW |
| `timeline_viz.js` | 1 | LOW |
| `search_modal.js` | 1 | LOW |

### Error Handling (User Feedback)

**Only 2 widgets show toast on error:** entity_posts.js, calendar_widget.js

**Console-only errors (no user feedback):**
- [ ] `notes.js` — all catch blocks log to console only
- [ ] `tag_picker.js` — Promise.all().catch() console-only
- [ ] `timeline_viz.js` — fetch() without .catch()
- [ ] `entity_tooltip.js` — fetch() without .catch()
- [ ] `editor_autolink.js` — catch logs only
- [ ] `editor_mention.js` — catch logs only
- [ ] `template_editor.js` — catch logs only
- [ ] `relation_graph.js` — catch logs only
- [ ] `search_modal.js` — catch logs only

**Widgets with proper error feedback:** entity_posts, calendar_widget,
image_upload, relations, attributes, permissions, groups, shop_inventory

### Utility Duplication

- [ ] `groups.js` defines local `escHtml()` + `escAttr()` — should use `Chronicle.escapeHtml()` / `Chronicle.escapeAttr()`
- [ ] `relation_graph.js` defines local `escHtml()` — should use `Chronicle.escapeHtml()`
- [ ] `groups.js` defines local `apiFetch()` wrapper — should use `Chronicle.apiFetch()`

---

## 3. Feature Parity

### Visibility/Permission Controls

| Content Type | Controls | Status |
|-------------|----------|--------|
| Entities | Everyone / DM Only / Custom (user/role/group grants) | Best |
| Calendar events | `dm_only` flag | OK |
| Timeline events | `visibility` field | OK |
| Map markers | DM-only flag | OK |
| Tags | `dm_only` flag | OK |
| Posts | `dm_only` toggle | OK |
| Relations | `dm_only` flag (migration 000052) | OK |
| Sessions | Campaign membership only | By design |
| Notes | Per-user ownership + locking | By design |

### Search

| Content Type | Ctrl+K | In-Context | Status |
|-------------|--------|-----------|--------|
| Entities | Yes | Full-text | Best |
| Calendar events | Yes | — | Gap: no filter in calendar views |
| Timelines | Yes | — | Gap: no search within timeline |
| Maps | Yes | Client-side marker | OK |
| Sessions | Yes | — | Gap: no filter on sessions list |
| **Notes** | — | — | **No search at all** |
| **Posts** | — | — | **No search at all** |

### Other Parity

- [x] Empty states — all plugins audited (batch 31)
- [x] Breadcrumbs — maps, timelines, sessions, calendar, entities all have them
- [x] HTMX fragment detection — all plugins use `middleware.IsHTMX()` or `HX-Request`
- [x] CSRF on state-changing routes — all plugins
- [x] RequireAddon gating — calendar, maps, sessions, timeline, media-gallery all gated
- [x] Error types — all 249 echo.NewHTTPError replaced with apperror

---

## 4. Export/Import Gaps

Campaign export covers entities (types, entities, tags, relations), calendar (full),
sessions (partial), maps (full), addons, media manifest.

### Data NOT exported

| Missing Data | Table(s) | Impact | Priority |
|-------------|----------|--------|----------|
| Entity permissions | `entity_permissions` | Custom visibility grants lost (TODO in code) | **HIGH** |
| Campaign groups | `campaign_groups`, `campaign_group_members` | Group-based permissions lost | **HIGH** |
| Entity posts | `entity_posts` | Sub-notes completely lost | **HIGH** |
| Timeline event connections | `timeline_event_connections` | Visual arrows/lines between events lost | **MEDIUM** |
| Timeline entity groups | `timeline_entity_groups` | Swim lane organization lost (type defined in export.go but never populated) | **MEDIUM** |
| Entity parent refs (import) | `entities.parent_id` | Exported as `ParentSlug` but NOT reimported — hierarchies flattened | **MEDIUM** |
| Session attendees | `session_attendees` | RSVP tracking lost | **LOW** |
| Media file bytes | Disk files | JSON-only; no file backup | **MEDIUM** |

### Data correctly exported (verified)

- Campaign metadata (name, desc, public, settings, sidebar, dashboard) ✓
- Entity types (all fields including layout, colors, dashboard) ✓
- Calendar event categories ✓
- Calendar events (all fields) ✓
- Map layers, drawings, tokens, fog ✓
- Timeline definitions + standalone events ✓

### Intentionally excluded

- Player notes (per-user; NoteService lacks `ListSharedByCampaign`)
- Note folders (covered under notes exclusion)
- Foundry sync metadata (external system coupling)

---

## 5. Documentation Gaps

### Missing .ai.md files

**Go widgets:**
- [ ] `internal/widgets/posts/` — no `.ai.md`

**JS widgets (16 files missing):**
- [ ] `calendar_widget.js`
- [ ] `map_widget.js`
- [ ] `relation_graph.js`
- [ ] `entity_type_config.js`
- [ ] `entity_type_editor.js`
- [ ] `groups.js`
- [ ] `permissions.js`
- [ ] `shop_inventory.js`
- [ ] `sidebar_config.js`
- [ ] `timeline_widget.js`
- [ ] `entity_posts.js`
- [ ] `recent_entities.js`
- [ ] `notifications.js`
- [ ] `shortcuts_help.js`

**Editor sub-extensions (consolidate into editor.ai.md):**
- [ ] `editor_autolink.js`
- [ ] `editor_secret.js`

---

## 6. Route Security (206 routes audited)

**Overall: EXCELLENT.** 100% of routes properly protected. No critical vulnerabilities.

### Verified patterns (all passing)

- [x] CSRF on all POST/PUT/DELETE routes (constant-time double-submit cookie)
- [x] API routes correctly exempt from CSRF (use Bearer token auth)
- [x] WebSocket exempt from CSRF (separate auth)
- [x] All campaign-scoped routes use `RequireCampaignAccess` middleware
- [x] Role hierarchy enforced: Player < Scribe < Owner
- [x] Admin bypass does NOT elevate campaign role (correct)
- [x] Addon-dependent routes gated by `RequireAddon` middleware
- [x] Public routes use `OptionalAuth + AllowPublicCampaignAccess`
- [x] Media files protected by HMAC-signed URLs + rate limiting
- [x] API keys scoped to single campaign with IP/fingerprint controls

### Minor gaps

- [ ] **Rate limiting on mutations** — Auth (10/min), media (30/min), and API routes have rate limits, but campaign/entity/widget mutations have none. Authenticated user could exhaust server resources.
- [ ] **No integration tests for CSRF rejection** — Global middleware should handle it, but no test verifies DELETE requests are rejected without CSRF token.

---

## 7. Template/UI Consistency (53 .templ files audited)

**Overall: GOOD.** Consistent layout structure, HTMX patterns, and role-based visibility.

### Verified patterns (all passing)

- [x] All pages use Base → App → Content 3-layer structure
- [x] Flash messages auto-dismiss (5s success, 8s error) with Alpine.js
- [x] Role-based visibility checks consistent across 12 files (52 checks)
- [x] ARIA roles on alerts (role="alert", role="status")
- [x] Confirmation dialogs on all destructive actions (14 files)
- [x] Reusable pagination component available and used

### Inconsistencies found

- [ ] **Alert styling mixed** — `login.templ` and `entities/form.templ` use inline Tailwind classes instead of `alert-success`/`alert-error` component classes
- [ ] **Admin pagination inline** — `admin/users.templ` and `admin/campaigns.templ` have hand-rolled pagination instead of using `components.Pagination`
- [ ] **Modal approach mixed** — Sessions uses `<dialog>` element; calendar/other modals use Alpine.js custom HTML. Should standardize.
- [ ] **Button sizing inconsistent** — Some add `py-2.5`, `text-sm`, `w-full`; no standard size variants
- [ ] **Card padding varies** — `p-4`, `p-6`, `p-8` used inconsistently across similar contexts
- [ ] **Empty state styling** — Only 4 files use `.empty-state` class; others use inline cards
- [ ] **Form autocomplete** — Login form has proper `autocomplete` attributes; entity forms don't

---

## 8. Module System

| Module | manifest.json | handler.go | data/ | Routes | Tooltip API | Pages |
|--------|:---:|:---:|:---:|:---:|:---:|:---:|
| dnd5e | ✓ | ✓ | 6 categories (87 items) | ✓ | ✓ (category-specific) | ✓ (browsable reference) |
| drawsteel | ✓ | stub | empty | — | — | — |
| pathfinder2e | ✓ | stub | empty | — | — | — |

D&D 5e module completed in Sprint M-1. Draw Steel and Pathfinder 2e remain scaffold-only.

---

## Priority Summary

### Must Fix (data correctness / user-facing bugs)

1. Export: entity_permissions, campaign_groups, entity_posts adapters
2. Export: timeline_event_connections + timeline_entity_groups (defined but never populated)
3. Export: entity parent_id reimport (exported as ParentSlug but import ignores it)
4. ~~Relations: add visibility controls~~ — Done: `dm_only` flag added (migration 000052)
5. JS error handling: notes.js, tag_picker.js (most-used widgets with silent failures)

### Should Fix (consistency / quality)

6. Migrate notes.js, attributes.js, relations.js to Chronicle.apiFetch()
7. Remove utility duplication in groups.js and relation_graph.js
8. Add service tests for maps and sessions plugins
9. Add service tests for calendar and timeline (beyond domain-logic tests)
10. Standardize alert styling (inline → component classes)
11. Admin pages: use reusable Pagination component
12. Rate limiting on campaign/entity mutation endpoints

### Nice to Have (polish / documentation)

13. Posts widget .ai.md
14. Add toast error feedback to remaining widgets
15. Add .ai.md to 16 undocumented JS widgets
16. Fix CI `-short` flag (either add Short() checks or remove flag)
17. Create actual integration test infrastructure
18. Standardize modal approach (dialog vs Alpine.js)
19. Define consistent button size / card padding CSS variants
20. Add autocomplete attributes to entity forms
