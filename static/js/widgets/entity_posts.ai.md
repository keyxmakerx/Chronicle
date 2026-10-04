# entity_posts.js

## Purpose

Entity "posts" (sub-notes): named, collapsible content sections shown below an
entity's main entry. Scribes can create, rename, edit (rich text), toggle
private, delete and drag-reorder them; players see the visible ones.

## Mount

`data-widget="entity-posts"`, loaded by a `<script defer>` tag in
`layouts/base.templ`. Mounted by `blockPosts` in `plugins/entities/show.templ`.

| Attribute | Meaning |
|---|---|
| `data-endpoint` | `/campaigns/:id/entities/:eid/posts` |
| `data-entity-id`, `data-campaign-id` | IDs |
| `data-editable` | Set when the viewer is Scribe or above |
| `data-csrf` | Forwarded to the nested editor mounts as `data-csrf-token` |

## Data it calls

All in `routes_snapshot.txt` (posts widget routes; reads are public-capable,
writes need Scribe):

- `GET <endpoint>`
- `POST <endpoint>` (`{name, isPrivate}`), `PUT <endpoint>/:pid`,
  `DELETE <endpoint>/:pid`
- `PUT <endpoint>/reorder` (`{postIds}`)

## Composition

Each editable post renders a child `data-widget="editor"` element
(`data-autosave="30"`, `data-compact="true"`, endpoint `<endpoint>/:pid`);
the script calls `Chronicle.mountWidgets(el)` shortly after a post expands.
If `Chronicle.hydrateNoteLinks` exists it is applied to the mount.

## Events

None emitted or listened to beyond DOM clicks and native drag events.
A document click listener closes the per-post menu; `destroy()` removes it and
the note-link hydration.

## Known defect: reads the response as parsed JSON (TODO(#1052))

`Chronicle.apiFetch` returns a raw `Response`. This widget treats the resolved
value as the parsed body (`state.posts = posts`, `post.id`, `updated`) and
never checks `res.ok` or calls `res.json()`, so by reading the code the list
does not populate, creates cannot expand the new post, and HTTP failures do not
reach `.catch`. It also sends `JSON.stringify(...)` bodies, which skip the
JSON `Content-Type` that `apiFetch` adds for object bodies. The notes widget
(`entity_notes.js`) shows the working pattern. Verify in a browser.

## Gotchas

- Post HTML (`entryHtml`) is inserted unescaped, so it must be sanitised
  server-side.
- Reorder failures are only logged to the console.
