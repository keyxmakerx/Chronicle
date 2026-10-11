# permissions.js

## Purpose

Per-entity visibility and grants. A "Permissions" trigger opens a card with a
visibility mode (Everyone / DM Only / Custom) and, in Custom mode, grant rows
per role, user and group. Also shows read-only tag-derived grants so an Owner
sees when a tag has widened access.

## Mount

`data-widget="permissions"`, loaded on sight from
`coreWidgets` in `internal/app/routes.go` (ADR-063). The page's own mount is the top-right visibility
control (`effectiveVisibilityBadge` in `plugins/entities/visibility_glance.templ`),
rendered for the Owner only:

| Mount | Attributes | Behaviour |
|---|---|---|
| Page corner | `data-layout="corner"`, `data-endpoint`, `data-editable` | Reuses the server-rendered `[data-perm-corner-trigger]` chip; the editor unfolds from it as a popover. Outside click closes it only when nothing is saving or failed, else a "Not saved yet" warning shows; Escape always closes |

Two more mounts in `plugins/entities/form.templ`:

| Mount | Attributes | Behaviour |
|---|---|---|
| Edit form | `data-endpoint=/campaigns/:id/entities/:eid/permissions`, `data-layout="inline"`, `data-editable`, `data-csrf` | Loads and saves via the endpoint; inline panel expands in place |
| Create form | `data-mode="draft"`, `data-draft-target="#is_private_draft"`, `data-editable="true"` | No endpoint; writes `true`/`false` into the hidden `is_private` input; Custom mode is disabled until the entity exists |

Without `data-layout="inline"` the card is a right-edge slide-in with a
backdrop and body-scroll lock. `data-editable` must be `"true"` to save.

## Data it calls

- `GET <endpoint>` returns `visibility`, `is_private`, `members`, `groups`,
  `permissions` and `tag_grants`.
- `PUT <endpoint>` sends `{visibility, is_private, permissions: [{subject_type,
  subject_id, permission}]}`; `subject_type` is `role`, `user` or `group`.

Both are `/entities/:eid/permissions` routes in `routes_snapshot.txt`, Owner
only. A save aborts any save still in flight.

## Events

Emits none. Listens for `keydown` on `document` (Escape closes the card);
the handler is stored on the element and removed in `destroy()`.

## Gotchas

- A failed load (for example a 403 for a non-Owner) sets `loadFailed` so the
  trigger and body never present the untouched defaults as "Everyone".
- Role grants use the numeric role (`ROLE_OWNER` is 3 and must match the Go
  constant); owners are listed separately and cannot be restricted.
- Styles are injected once into `#perm-widget-styles` and use the campaign
  theme CSS variables with fallbacks.
- `data-csrf` is not read; the CSRF header comes from `Chronicle.apiFetch`.
