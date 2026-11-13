# Player-eyes crawl

Proves hidden things stay hidden. `run.mjs` registers an owner, two players and
a scribe on a scratch instance, seeds one campaign with a unique marker string
in everything a viewer must not see, then walks every campaign GET route (the
list comes from `internal/wire/routes_snapshot.txt`, as HTML, JSON and HTMX)
as each viewer and fails if a marker appears in a response. A socket is held
open for each viewer while the owner edits hidden things.

Two guards keep a clean result honest: every marker must be seen by the owner,
and every viewer must see the visible control text. A walk that cannot see
what it should is reported as blind and fails.

```
make player-crawl CRAWL_BASE=http://localhost:8080   # scratch instance only
```

Add a seed in `seed.mjs` for each new access bug before fixing it. A page's
Director-only part, a hidden event, anything: one `m('KEY', 'what it is')`.
Things a Scribe may see by design pass `{ scribe: true }`.
