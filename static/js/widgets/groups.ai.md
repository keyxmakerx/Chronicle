# groups.js

## Purpose

Owner-only management UI for campaign groups: create, rename, delete groups
and add or remove members. Groups are the subjects of per-entity permission
grants (see `permissions.js`).

## Mount

`data-widget="groups"`, loaded by a `<script defer>` tag in
`layouts/base.templ`. Mounted once, in `plugins/campaigns/groups.templ`
(`GET /campaigns/:id/groups/manage`, Owner role).

| Attribute | Meaning |
|---|---|
| `data-campaign-id` | Campaign ID |
| `data-groups-endpoint` | `/campaigns/:id/groups` |
| `data-members-json` | JSON array of campaign members (`user_id`, `display_name`, `email`, `role`) for the add-member select |
| `data-csrf` | Read into a variable but unused; `Chronicle.apiFetch` attaches the CSRF header |

## Data it calls

Via `Chronicle.apiFetch`, all in `routes_snapshot.txt` and all
`RequireRole(RoleOwner)`:

- `GET /groups` (response `{groups: [...]}`), `GET /groups/:gid` (`members`)
- `POST /groups`, `PUT /groups/:gid`, `DELETE /groups/:gid`
- `POST /groups/:gid/members` (`{user_id}`), `DELETE /groups/:gid/members/:uid`

## Events

None emitted. A single delegated click listener on the mount element handles
every action; `destroy()` removes it. Delete uses a native `confirm()`.

## Gotchas

- Description is sent as `null` on rename when cleared, omitted on create.
- Members already in a group are filtered out of the add-member select
  client-side from `data-members-json`, which is a snapshot from page render.
