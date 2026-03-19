# sidebar_tag_filter.js

## Purpose

Adds tag filter chips to the sidebar drill panel, allowing users to
filter the entity tree by tags. Uses AND logic — selecting multiple
tags shows only entities that have ALL selected tags.

## Architecture

**IIFE module** — injects tag chips below the search input after the
drill panel loads via HTMX.

## How It Works

1. After HTMX loads the drill panel content (`htmx:afterSettle`),
   `injectTagFilter()` finds the search div and appends tag chips.
2. Tags are fetched from `GET /campaigns/:id/tags` (cached in memory).
3. Clicking a chip toggles it active/inactive and calls `applyFilter()`.
4. `applyFilter()` modifies the `hx-get` URL on the search input to
   append `&tags=slug1,slug2` and triggers an HTMX re-fetch.

## Backend

The `?tags=` parameter is handled by `SearchAPI` in the entities handler.
It splits comma-separated slugs and passes them as `opts.TagSlugs` to
the repository, which uses a subquery with `HAVING COUNT` for AND logic:

```sql
AND e.id IN (
  SELECT et2.entity_id FROM entity_tags et2
  INNER JOIN tags t2 ON t2.id = et2.tag_id
  WHERE t2.slug IN (?, ?)
  GROUP BY et2.entity_id
  HAVING COUNT(DISTINCT t2.slug) = ?
)
```

## State

- `activeTags` — array of currently selected tag slugs
- `tagCache` — fetched tags (cached for session)

## API

- `GET /campaigns/:id/tags` — returns `[{id, name, slug, color, dmOnly}]`
