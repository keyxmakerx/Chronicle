# Sprint V-2: Backlinks Panel & Entity Aliases — Implementation Plan

## Overview

Two complementary features that strengthen Chronicle's "discovery" story:
1. **Backlinks enhancements** — Redis caching, context snippets, dedicated HTMX endpoint for lazy-loading
2. **Entity Aliases** — multiple canonical names per entity for auto-linking, search, and mentions

## Current State

### Backlinks (already partially implemented)
- `repository.go:FindBacklinks()` — LIKE search on `entry_html` for `data-mention-id`
- `service.go:GetBacklinks()` — delegates to repo
- `handler.go:Show()` — fetches backlinks synchronously in page load (line 426)
- `show.templ:blockBacklinks()` — renders "Referenced by" with entity chips
- **Missing:** Redis caching, context snippets, lazy-load API endpoint

### Entity Aliases (not started)
- No `entity_aliases` table
- `ListNames` and `Search` only query `entities.name`
- Auto-linker only matches exact entity names

---

## Implementation Steps

### Step 1: Migration 000061 — `entity_aliases` table

```sql
-- 000061_entity_aliases.up.sql
CREATE TABLE entity_aliases (
    id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
    entity_id  CHAR(36)     NOT NULL,
    alias      VARCHAR(200) NOT NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_alias_entity (entity_id, alias),
    INDEX idx_alias_campaign (entity_id),
    FULLTEXT INDEX ft_alias (alias),
    CONSTRAINT fk_alias_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

```sql
-- 000061_entity_aliases.down.sql
DROP TABLE IF EXISTS entity_aliases;
```

### Step 2: Model — Alias types

In `model.go`, add:
- `EntityAlias` struct: `ID int`, `EntityID string`, `Alias string`, `CreatedAt time.Time`
- Add `Aliases []string` field to `EntityNameEntry` for auto-linker consumption
- `SetAliasesInput` struct: `Aliases []string` (max 10 per entity, each 2-200 chars)

### Step 3: Repository — Alias CRUD + query integration

In `repository.go`, add to `EntityRepository` interface:
- `ListAliases(ctx, entityID) ([]EntityAlias, error)` — all aliases for one entity
- `SetAliases(ctx, entityID string, aliases []string) error` — replace all aliases (DELETE + batch INSERT)
- `FindByAlias(ctx, campaignID, alias string) (*Entity, error)` — exact match lookup

Modify existing methods:
- **`ListNames()`** — LEFT JOIN `entity_aliases` and return aliases alongside names. The auto-linker needs alias entries as separate `EntityNameEntry` rows pointing to the same entity ID, so it can match either name or alias.
- **`Search()`** — UNION with alias FULLTEXT search: `SELECT ... FROM entity_aliases ea JOIN entities e ON ea.entity_id = e.id WHERE MATCH(ea.alias) AGAINST(? IN BOOLEAN MODE)`. Deduplicate results by entity ID.

### Step 4: Service — Alias business logic

In `service.go`, add:
- `GetAliases(ctx, entityID) ([]EntityAlias, error)`
- `SetAliases(ctx, entityID string, aliases []string) error` — validate max 10, length 2-200, no duplicates. Invalidate entity-names Redis cache.
- Modify `Delete()` — aliases cascade via FK, but invalidate caches

### Step 5: Handler — Alias API endpoints

In `handler.go`, add:
- `GetAliasesAPI(c) error` — `GET /campaigns/:id/entities/:eid/aliases` → JSON array
- `SetAliasesAPI(c) error` — `PUT /campaigns/:id/entities/:eid/aliases` → accepts `{aliases: ["name1", ...]}`, requires Scribe+ role

In `routes.go`, register in the authenticated entity routes group.

### Step 6: Backlinks — Redis caching

In `handler.go`, enhance backlink fetching:
- Add `backlinksCacheTTL = 5 * time.Minute`
- Cache key: `backlinks:<entityID>:<role>:<userID>`
- Check Redis before calling `service.GetBacklinks()`
- Store serialized result in Redis
- **Invalidation:** When `UpdateEntry()` is called (entity content changed), delete all `backlinks:*` keys for entities mentioned in the old AND new content. Simplest approach: invalidate on `UpdateEntry` by deleting the cache key for the entity being edited (since its mentions may have changed).

### Step 7: Backlinks — Context snippets

Enhance `FindBacklinks()` to return context around the mention:

In `model.go`, add:
- `BacklinkEntry` struct: `Entity Entity`, `Snippet string` (text around the mention)

In `repository.go` or `service.go`:
- After fetching backlink entities, extract snippet from `entry_html`:
  - Find `data-mention-id="<entityID>"` in the HTML
  - Extract surrounding plain text (strip tags), ~100 chars before/after
  - This can be done in the service layer post-query using a simple HTML text extractor

Update `show.templ`:
- Modify `blockBacklinks()` to show snippet text below each entity chip
- Truncate to ~120 chars with ellipsis

### Step 8: Backlinks — HTMX lazy-load endpoint

Add a dedicated API endpoint so backlinks load asynchronously (they involve a LIKE scan which can be slow):

In `handler.go`:
- `BacklinksAPI(c) error` — `GET /campaigns/:id/entities/:eid/backlinks`
- Returns HTMX fragment (check `HX-Request` header) or JSON
- Uses Redis cache from Step 6

In `show.templ`:
- Replace inline `blockBacklinks()` call with an HTMX placeholder:
  ```html
  <div hx-get="/campaigns/{id}/entities/{eid}/backlinks"
       hx-trigger="load" hx-swap="innerHTML">
    <span class="text-fg-muted text-sm">Loading references...</span>
  </div>
  ```
- Move `blockBacklinks()` to be the HTMX fragment response

In `handler.go:Show()`:
- Remove synchronous `GetBacklinks()` call (line 426) — backlinks now load async

### Step 9: Alias UI — Entity show page widget

Create `static/js/widgets/aliases.js`:
- Small widget mounted on entity show/edit pages
- Displays current aliases as removable tags/chips
- "Add alias" input field (Scribe+ only)
- Calls `GET/PUT /campaigns/:id/entities/:eid/aliases`
- On alias change, call `Chronicle.invalidateAutoLinkCache(campaignId)` to refresh auto-linker

In `show.templ`:
- Add `data-widget="aliases"` div in the entity header area (below name, before content)
- Only render for Scribe+ roles (Players see aliases as read-only or hidden)

### Step 10: Auto-linker integration

In `editor_autolink.js`:
- `fetchEntityNames()` already returns `EntityNameEntry` objects
- Backend `ListNames()` (Step 3) now returns alias entries as separate rows
- **No JS changes needed** — the auto-linker already matches names by regex; alias entries will match naturally since they appear as separate name entries pointing to the same entity ID

### Step 11: Search & mention integration

In `editor_mention.js`:
- The mention popup calls `/campaigns/:id/entities/search?q=...`
- Backend `Search()` (Step 3) now includes alias matches
- **No JS changes needed** — results already show entity name; alias matches just appear in results

### Step 12: Tests

- **Repository tests:** alias CRUD, ListNames with aliases, Search with aliases, FindBacklinks
- **Service tests:** SetAliases validation (max 10, length limits, dedup)
- **Handler tests:** alias API endpoints, backlinks caching, HTMX fragment response
- **Snippet extraction:** unit test for HTML → plain text snippet generation

### Step 13: Cache invalidation wiring

In entity event handlers or service methods:
- `UpdateEntry()` → invalidate `backlinks:*` for the edited entity's mentioned entities
- `SetAliases()` → invalidate `entity-names:*` for the campaign
- `Delete()` → invalidate both backlinks and entity-names caches
- `Create()` / `Update()` (name change) → invalidate `entity-names:*` for the campaign

---

## File Changes Summary

| File | Change |
|------|--------|
| `db/migrations/000061_entity_aliases.up.sql` | **NEW** — create table |
| `db/migrations/000061_entity_aliases.down.sql` | **NEW** — drop table |
| `internal/plugins/entities/model.go` | Add `EntityAlias`, `BacklinkEntry`, `SetAliasesInput`, modify `EntityNameEntry` |
| `internal/plugins/entities/repository.go` | Add alias methods, modify `ListNames`, `Search` |
| `internal/plugins/entities/service.go` | Add alias service methods, cache invalidation |
| `internal/plugins/entities/handler.go` | Add alias + backlinks API endpoints, Redis caching |
| `internal/plugins/entities/routes.go` | Register new routes |
| `internal/plugins/entities/show.templ` | HTMX lazy-load backlinks, alias widget mount, snippets |
| `static/js/widgets/aliases.js` | **NEW** — alias management widget |
| `internal/plugins/entities/entities_test.go` | New tests |
| `internal/plugins/entities/.ai.md` | Update docs |

---

## Execution Order

1. Migration + model (Steps 1-2)
2. Repository alias CRUD + query changes (Step 3)
3. Service alias logic (Step 4)
4. Handler alias endpoints + routes (Steps 5, 9)
5. Backlinks Redis caching (Step 6)
6. Backlinks context snippets (Step 7)
7. Backlinks HTMX lazy-load (Step 8)
8. Aliases widget JS (Step 9)
9. Cache invalidation wiring (Step 13)
10. Tests (Step 12)
11. Documentation updates

## Risks & Notes

- **LIKE scan performance:** `FindBacklinks` uses `LIKE '%data-mention-id="..."'%` on `entry_html`. For large campaigns this could be slow. Redis caching (Step 6) mitigates this. A future optimization could be a dedicated `entity_mentions` junction table populated on save, but that's over-engineering for now.
- **Alias uniqueness:** Aliases should be unique per campaign (not just per entity) to prevent ambiguity in auto-linking. Consider adding a campaign-scoped uniqueness check in the service layer.
- **Migration safety:** The new table is additive-only, no schema changes to existing tables.
