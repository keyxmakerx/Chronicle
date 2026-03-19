# sidebar_layout_editor.js

## Purpose

Unified drag-and-drop editor for the campaign sidebar layout. Replaces
the separate `sidebar_config.js` (category ordering) and
`sidebar_nav_editor.js` (custom sections/links) with a single editor
that controls ALL sidebar items.

## Architecture

**Chronicle.register widget** — mounts on `data-widget="sidebar-layout-editor"`
in the Customize Hub Navigation tab.

### Data Model

The editor works with the `SidebarConfig.Items` array — a unified, ordered
list of sidebar items. Each item has:

```json
{
  "type": "dashboard|addon|category|section|link|all_pages",
  "visible": true,
  "slug": "notes",        // addon slug (for type=addon)
  "type_id": 5,           // entity type ID (for type=category)
  "id": "sec_abc",        // unique ID (for sections/links)
  "label": "Resources",   // display label (for sections/links)
  "url": "https://...",   // link URL (for type=link)
  "icon": "fa-globe"      // FontAwesome icon (for type=link)
}
```

### Default Generation

If the campaign has no `items` array yet (legacy config), the editor
auto-generates defaults from:
- Dashboard link
- Known addons (notes/Journal, npcs/NPCs, armory/Armory)
- All entity types from `data-entity-types` attribute
- "All Pages" link

## Features

- **Drag-and-drop reorder**: any item can be dragged to any position
- **Visibility toggle**: eye icon shows/hides items from sidebar
- **Type badges**: shows item type (addon, category, section, link)
- **Edit sections/links**: pencil icon opens prompt for label/URL/icon
- **Delete sections/links**: trash icon with confirmation
- **Add Section**: prompt for label, generates unique ID
- **Add Link**: prompts for label + URL, generates unique ID

## API

- `GET /campaigns/:id/sidebar-config` — load current config
- `PUT /campaigns/:id/sidebar-config` — save `{ items: [...], hidden_entity_ids: [...] }`

## Mount Attributes

| Attribute | Purpose |
|-----------|---------|
| `data-endpoint` | Sidebar config API URL |
| `data-campaign-id` | Campaign ID |
| `data-entity-types` | JSON array of `{id, name, name_plural, icon, color}` |
