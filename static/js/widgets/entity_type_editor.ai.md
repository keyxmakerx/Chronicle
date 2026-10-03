# entity_type_editor.js

## Purpose

Inline editor for an entity type's custom fields (label, type, section,
order) and, optionally, its name, plural name, icon and colour. Saves the
whole definition with one PUT.

## Mount

`data-widget="entity-type-editor"`, loaded by a `<script defer>` tag in
`layouts/base.templ`. Mounted twice in `plugins/entities/entity_type_config.templ`
(the field-management tabs), both with `data-fields-only="true"`.

| Attribute | Meaning |
|---|---|
| `data-endpoint` | `PUT /campaigns/:id/entity-types/:etid` |
| `data-entity-type-id` | Type ID (not read by the script) |
| `data-name`, `data-name-plural`, `data-icon`, `data-color` | Current values |
| `data-fields` | JSON array of field definitions |
| `data-fields-only` | `"true"` hides name/icon/colour; they are sent back unchanged |
| `data-csrf-token` | Present on the element; the script does not read it (`Chronicle.apiFetch` attaches the CSRF header) |

## Data it calls

`PUT <data-endpoint>` with `{name, name_plural, icon, color, fields}`; the
route exists in `routes_snapshot.txt`. Fields with an empty key or label are
dropped before sending. On error the response's `error` string is shown inline.

## Behaviour

- Field key is regenerated from the label on every keystroke
  (lowercase, non-alphanumerics to `_`), so editing a label of an existing
  field changes its key.
- Field types offered: text, number, textarea, select, checkbox, url. There
  is no UI for `select` options; new fields start with an empty list.
- Full mode (not fields-only) has a 38-icon picker and reloads the page 500 ms
  after a successful save. No current template uses full mode.

## Events

None emitted or listened to.

## Gotchas

- Fields-only mode sends the name, icon and colour captured at mount time; a
  rename made elsewhere on the page after load is overwritten by the save.
- The editor builds its markup with raw Tailwind `gray-*` / `blue-*` classes
  rather than the theme tokens used by sibling widgets.
