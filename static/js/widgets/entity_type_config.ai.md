# entity_type_config.js

## Purpose

Unified sidebar configuration list for entity types: drag to reorder,
eye toggle to hide from the sidebar, colour swatch to recolour, and a link to
each type's page-template editor.

## Status: loaded but mounted by nothing

`layouts/base.templ` loads the script and it registers `entity-type-config`,
but no templ, Go or JS file renders `data-widget="entity-type-config"`.
`plugins/entities/entity_type_config.templ` (the config page) mounts
`template-editor`, `layout-editor` and `entity-type-editor` instead. The
script is dead until something mounts it or it is removed.

## Intended mount

| Attribute | Meaning |
|---|---|
| `data-sidebar-endpoint` | Sidebar config endpoint (required) |
| `data-layout-base` | Base URL for entity-type routes, `/campaigns/:id/entity-types` (required) |
| `data-entity-types` | JSON array of entity types (`id`, `name`, `name_plural`, `icon`, `color`) |

Without the first two attributes it logs an error and renders nothing.

## Data it would call

- `GET` / `PUT <sidebar-endpoint>` with `{entity_type_order, hidden_type_ids}`;
  the campaigns plugin serves `/sidebar-config` for both verbs.
- `PUT <layout-base>/:etid/color` with `{color}`; exists in the entities
  routes.
- Links to `<layout-base>/:etid/template` (a `GET` route).

## Events

None. Drag-and-drop uses native HTML5 drag events on the row headers.

## Gotchas

- The sidebar save is fire-and-forget: HTTP failures are only logged to the
  console, and the UI keeps its optimistic state.
- Colour change re-renders the whole list; a failed save is not rolled back.
- Order is stored as entity-type IDs; types missing from the saved order are
  appended in server order.
