# Obsidian-Style Notes Plan

## Overview

Obsidian excels at fast, friction-free note-taking with powerful linking and
discovery features. Chronicle already has rich entity pages, but they're
designed for wiki-style long-form content. Adding Obsidian-style "quick notes"
would complement the existing system by providing a fast-capture, interlinked
note-taking experience alongside the structured entity pages.

## Feature Analysis

### 1. Quick Capture / Daily Notes
**What Obsidian does:** One-click daily note creation with today's date as the
title. Append-to-daily-note from anywhere. Quick inbox for fleeting thoughts.

**Chronicle adaptation:**
- Add a "Session Journal" or "Quick Note" button in the topbar/sidebar
- Creates a timestamped note in the player notes system (already exists)
- Quick-capture modal (Ctrl+Shift+N) that creates a note with a single input
- Option to append to today's journal entry rather than creating new notes

**Complexity:** Low - Uses existing notes system, just needs new UI entry points
**Priority:** HIGH - Fast capture is essential for DMs during sessions

### 2. Backlinks Panel
**What Obsidian does:** Shows all pages that link to the current page, with
context snippets around the link.

**Chronicle adaptation:**
- Entity pages already track @mention links in `entry_html`
- Add a "Backlinks" section to entity show pages (or as a layout block type)
- Query: scan all entities' `entry_html` for links containing the current
  entity's ID (via `data-mention-id` attribute)
- Display as collapsible panel with source entity name + context snippet
- Cache results in Redis (invalidate on entity save)

**Complexity:** Medium - Needs backlink scanning query + template + caching
**Priority:** HIGH - Backlinks are incredibly useful for worldbuilding to see
how a character/location/item is referenced across the world

### 3. Quick Switcher (Ctrl+K)
**What Obsidian does:** Fuzzy-search popup to jump to any note by typing
part of its name. Instant navigation.

**Chronicle adaptation:**
- Chronicle already has Ctrl+K quick search! This is implemented.
- Could enhance with: recent notes, fuzzy matching, keyboard navigation
- Current implementation searches entities, timelines, maps, calendar events,
  sessions. Could add player notes to the search.

**Complexity:** Low - Already mostly implemented
**Priority:** DONE (minor enhancements possible)

### 4. Linking Unlinked Mentions
**What Obsidian does:** Finds text in notes that matches other note titles
but isn't yet linked. Offers one-click linking.

**Chronicle adaptation:**
- Already implemented! The "Auto-link Entities" feature (Ctrl+Shift+L) in the
  editor scans text for entity names and creates @mention links.
- Works via the entity-names API with Redis caching.

**Complexity:** Already done
**Priority:** DONE

### 5. Block References / Transclusion
**What Obsidian does:** Embed a specific paragraph or section from one note
inside another. The embedded content updates when the source changes.

**Chronicle adaptation:**
- Add an "embed entity" block type in the editor (similar to @mentions but
  renders a preview card inline)
- Use HTMX or JS to fetch and render the target entity's summary/entry
- Could support embedding specific sections via anchor IDs
- Entity posts (sub-notes) could serve as embeddable blocks

**Complexity:** High - Needs new editor extension, rendering pipeline, and
circular reference detection
**Priority:** MEDIUM - Useful but not essential for MVP

### 6. Note Templates
**What Obsidian does:** Pre-defined note structures that can be applied when
creating a new note. Templates with variables (date, title, etc.).

**Chronicle adaptation:**
- Entity types already serve as "templates" with custom fields and layouts
- Could add "content templates" that pre-fill the editor with structured content
  (e.g., a "Session Recap" template with headings for Summary, Key Events, NPCs
  Encountered, Loot, Notes for Next Session)
- Template picker shown in entity create flow or as an editor insert option
- Store templates per campaign or globally

**Complexity:** Medium - Templates table, picker UI, variable substitution
**Priority:** MEDIUM - Very useful for session recaps and consistent note format

### 7. Canvas / Whiteboard
**What Obsidian does:** Free-form spatial canvas where notes are placed as cards
on an infinite 2D surface. Cards can be connected with arrows. Mix notes with
images and embedded content.

**Chronicle adaptation:**
- Maps already provide spatial layout for geographic content
- A "Storyboard" or "Conspiracy Board" feature could use a similar approach:
  - Cards (entities) placed on a canvas
  - Arrows between cards (relations visualized)
  - Free-form text annotations
  - Group cards into clusters
- Could use a library like Excalidraw, tldraw, or custom canvas with D3

**Complexity:** Very High - Essentially a new plugin
**Priority:** LOW for MVP, HIGH for long-term appeal (DMs love conspiracy boards)

### 8. Graph View (Enhanced)
**What Obsidian does:** Force-directed graph of all notes and their links.
Color-coded by folder/tag. Filter by tags. Click to navigate.

**Chronicle adaptation:**
- Chronicle already has a relations graph! (`relation_graph.js` with D3)
- Enhancements for Obsidian-style experience:
  - Include @mention links (not just explicit relations)
  - Filter by entity type, tag, or search query
  - Local graph view (show only entities within N hops of current entity)
  - Cluster by entity type or tag
  - Show orphan entities (no links)

**Complexity:** Medium - Extends existing graph, needs mention-link extraction
**Priority:** MEDIUM - Good for understanding world structure

## Implementation Roadmap

### Phase 1: Quick Wins (LOW effort, HIGH value)
1. **Quick Capture Modal** (Ctrl+Shift+N) - Creates a note instantly
2. **Backlinks Panel** - Show "Referenced By" on entity pages
3. **Add player notes to quick search** results

### Phase 2: Enhanced Notes (MEDIUM effort, MEDIUM-HIGH value)
4. **Content Templates** - Pre-fill editor with structured content
5. **Enhanced Graph** - Include @mentions, local graph, type filtering
6. **Full-page Notes** - Make player notes feel more like first-class pages
   with their own URL, breadcrumbs, and full-width editor

### Phase 3: Advanced (HIGH effort, MEDIUM value)
7. **Block References** - Embed entity summaries in other entities
8. **Canvas/Storyboard** - Spatial note arrangement with connections

## Key Design Principles

- **Notes are NOT entities.** Keep them separate. Notes are personal, quick,
  ephemeral. Entities are shared, structured, permanent. Don't force notes
  through the entity pipeline.
- **Cross-linking is key.** Notes should link to entities (and vice versa).
  The value comes from the web of connections.
- **Speed over structure.** Notes should be faster to create than entities.
  Minimal required fields, instant creation, auto-save.
- **DM session workflow.** Optimize for the DM who needs to jot things down
  during a live session: quick capture, append-to-journal, tag for follow-up.
