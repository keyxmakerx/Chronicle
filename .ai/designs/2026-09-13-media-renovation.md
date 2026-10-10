# Media and file handling — renovation design (2026-09-13)

**Status:** DRAFT, for operator sign-off. Nothing here is implemented.
**Authorization on file:** *"The media files i'm for you renovating, as long as
you use the migration tool that starts up on docker boot to migrate whatever
file formats into whatever you decide. Feel free to look up proper enterprise
solutions so long as they are open source. Feel free to add libraries that can
assist."*
**Answers the standing operator question** *"Whether media access should be
re-architected further than ADR-058 goes."*

---

## Plain-language summary

### The problem, in one paragraph

Chronicle stores every picture, map and audio clip in one place, and asks one
question before handing it to a visitor: *"are you a member of the campaign this
file belongs to?"* That is the only question it can ask, because the file itself
has no record of which **pages** it appears on. A file knows its campaign. It
does not know it is the portrait on a secret NPC, or the background of a map
only you are supposed to see. So the answer is the same for everyone in the
campaign, whatever their role. This is the equivalent of an access list that
only checks which network you are on and never which host you asked for.

Work already shipped (ADR-058) patched the worst of this by **asking the
question backwards at request time**: when someone requests a picture, Chronicle
searches the page records for anything that mentions that picture, and grants
access only if at least one of those pages is visible to that person. It works.
But it is a search, not a record — it looks in three places out of nine, one of
those searches is a full text scan of every page in the campaign, and it can
only find references written in one particular format. Anything it cannot find
falls back to the old, role-blind answer.

### Why the search cannot be made correct by improving the search

References to a file are recorded in **three incompatible formats**, and nothing
in the database tells you which format a given field uses:

| Format | Example of where | What is stored |
|---|---|---|
| The file's ID | a map's background image | `b7c17bb1-6563-…` |
| A location on disk | a campaign's backdrop | `2026/09/b7c17bb1-….png` |
| Buried inside a settings blob | the top-bar image | the same disk location, nested in JSON |

Worse, two of the fields with **"path" in their name actually hold an ID**, and
one field that looks like a media reference is not one at all — it is a free-text
string that gets used directly as an image address, and may point anywhere,
including at a website outside your server.

This ambiguity is not a tidiness problem. It is the bug generator. Two separate
faults found on this branch in the last two days were both caused by it: campaign
backdrops and top-bar images that **had never once loaded** in the product's
history (fixed in `efc020fe`), and — still live — the fact that the permission
check can only match one of the two formats, so a reference stored in the other
format is invisible to it and quietly falls back to the old role-blind answer.

### The choice

Build the record instead of running the search. Add one small table that says,
plainly, *"file X is used by page Y, in position Z"*, written at the moment
something is saved, and rebuilt from scratch on startup for everything that
already exists. Then the permission question becomes a direct lookup instead of
a scan, it covers all nine places a file can be referenced instead of three, and
it stops depending on which of the three formats somebody happened to write.

The one thing we deliberately do **not** do is rename all the confusing columns.
That would touch roughly sixty files across ten feature areas, and — the decisive
reason — it would not remove the need to understand the old formats anyway,
because every backup file anyone has ever taken still contains them. So instead
of renaming sixty places, we write **one** piece of code that understands all
three formats, and route everything through it.

### What you will be able to see

Today, when Chronicle does background repair work on startup, it prints two
lines into the log and that is the entire story. For a large picture library
that is not good enough — you asked for a migration that runs on Docker boot,
and you should be able to watch it. The plan adds a panel under
**Admin → Storage** showing, per category, how far it has got, how many records
it wrote, how many errors it hit and what the last one said, plus a retry
button. Those numbers survive a restart, so a container that reboots halfway
through resumes where it stopped rather than starting over.

### What this will cost you

- **Every profile avatar uploaded before this ships is already gone.** Not
  "will be lost" — gone, on any instance that has ever restarted. They were
  written outside the Docker volume. Nothing can recover them; people will need
  to re-upload. This is fixed as the very first slice, but the existing files
  are not recoverable.
- **Some people will lose sight of a picture they can see today.** That is the
  point of the change, it was already booked as a consequence in ADR-058, and
  this plan widens it from "pictures on pages" to "pictures on maps, notes and
  bestiary entries too". It needs a release note, not a silent deploy.
- **Slightly more disk used over time.** Chronicle merges identical uploads into
  one file. It now refuses that merge when the two uses have different
  permissions. Covering more places means more refusals, which means more
  genuine duplicates. That is the correct trade — the alternative is a secret
  map's artwork silently becoming reachable from a public page — but it does
  cost storage.

### Three findings from checking the ground truth

1. **Chronicle ships no built-in token artwork at all.** The map-token image
   field was believed to often hold built-in art like `wolf.png`. The entire
   `static/` tree contains five images and all five belong to the mapping
   library. `wolf.png` exists only in a test fixture. So that field is not "art
   we ship" — it is an unvalidated free-text string used verbatim as an image
   address. A token pointing at an outside address would make every viewer's
   browser call that outside server. Small, separate, and worth closing.
2. **The permission check has a live hole in it right now.** It matches page
   references only in the ID format. The code that saves a page image accepts
   either format and does not convert. Any page whose image was written in the
   disk-location format — which an import or a restored backup can do — is
   invisible to the check, and the file falls through to the old role-blind
   answer. This is the same class of leak ADR-058 was written to close, still
   open through format ambiguity.
3. **The existing startup repair job cannot be stopped.** It is handed a
   never-cancelled timer, so its own shutdown check can never fire. On shutdown
   it keeps working against a closing database. Harmless today; not harmless for
   a job that walks an entire media library.

Everything below this line is for implementers.

---

## 1. The usage table

### Schema

Core migration (`db/migrations/000031_media_usages.up.sql`). `media_files` is a
core table, so its companion must be core too.

```sql
CREATE TABLE IF NOT EXISTS media_usages (
    media_id      CHAR(36)    NOT NULL,
    campaign_id   CHAR(36)    NULL,
    subject_kind  VARCHAR(32) NOT NULL,
    subject_id    CHAR(36)    NOT NULL,
    field         VARCHAR(48) NOT NULL,
    source        VARCHAR(16) NOT NULL DEFAULT 'write',
    first_seen_at TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
                              ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (media_id, subject_kind, subject_id, field),
    INDEX idx_media_usages_subject (subject_kind, subject_id),
    INDEX idx_media_usages_campaign (campaign_id, media_id),
    CONSTRAINT fk_media_usages_media
        FOREIGN KEY (media_id) REFERENCES media_files(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**Why polymorphic (`subject_kind` + `subject_id`) rather than a real FK per
subject.** Not a preference — a hard constraint from `.ai/conventions.md`
§"Migration Safety Rules" #7. Core migrations run *before* plugin migrations, so
a core table cannot carry a foreign key to `maps` or `bestiary_publications`; it
would crash on a fresh database. The only alternatives are one usage table per
plugin (which defeats the purpose — the access check would have to union nine
tables again) or a core table with no subject FK. This is that.

**Consequence, stated honestly:** without an FK, a usage row can outlive its
subject. Two mitigations, both in §3: the reconciler prunes, and the access check
re-verifies the single row that produced a "yes".

`source` records how a row arrived (`write`, `backfill`, `import`) purely so the
admin panel can say "the backfill wrote these 4,102 and normal saves wrote these
17".

### The nine reference sites and how each is populated

| # | Location | Shape today | Populated by |
|---|---|---|---|
| 1 | `entities.image_path` | **ID** (name lies) | entities service on `UpdateImage` |
| 2 | `entities.cover_image_path` | **ID** (name lies) | entities service on `UpdateCoverImage` |
| 3 | `entities.entry_html` | embedded URL | entities service on content save, via HTML parse |
| 4 | `maps.image_id` | ID (real FK) | maps service on map create/update |
| 5 | `map_tokens.image_path` | free text — see below | maps service on token create/update |
| 6 | `campaigns.backdrop_path` | **disk path** | campaigns service on backdrop set/clear |
| 7 | `campaigns.settings → topbar_style.image_path` | **disk path, in JSON** | campaigns service on `UpdateTopbarStyle` |
| 8 | `note_attachments.file_path` | **disk path** | notes widget on attachment create |
| 9 | `users.avatar_path` | disk path *(becomes an ID in slice M0)* | auth service on avatar set/clear |
| — | `bestiary_publications.artwork_media_id` | ID, never written | nothing — see below |

**All three shapes converge on one function.** `media.ResolveReference(value)
(mediaID string, ok bool)` is the single place any of the three shapes is
interpreted. It is what `layouts.normalizeMediaID` does today — strip the date
directories, strip the extension — **plus the check that helper is missing**: the
result must exist in `media_files`. Without that check, `"wolf.png"` resolves to
a media id of `"wolf"` and gets a usage row for a file that does not exist.

- **ID columns** (1, 2, 4, and bestiary): value passes through unchanged, after
  the existence check.
- **Disk-path columns** (6, 7, 8, 9-before-M0): the basename-minus-extension is
  the id, because the uploader uses the same UUID for the database row and the
  on-disk filename. This is the property the 2026-09-13 backdrop fix already
  relies on; it is now written down in one function instead of being rediscovered.
- **The JSON blob** (7): the campaigns service already owns `ParseSettings()` and
  `TopbarStyle`. It records the usage there. The backfill reads it with
  `JSON_EXTRACT(settings, '$.topbar_style.image_path')`.
- **Embedded HTML** (3): parsed with **goquery, already a direct dependency**
  (`github.com/PuerkitoBio/goquery v1.9.2`) — no new library. Collect `img[src]`,
  `source[src]`, `a[href]`, `[style*=url(]`; keep values whose path begins
  `/media/`; strip any query string (a stored signed URL carries `?expires&sig`);
  run the remainder through `ResolveReference`. This replaces the
  `entry_html LIKE CONCAT('%/media/', ?, '%')` scan entirely.

### `map_tokens.image_path` — corrected

**[spot-check — the earlier premise does not hold.]** It was believed this column
"is OFTEN built-in token art like `wolf.png`, NOT a `media_files` upload at all".
The first half is not true of the shipped product: `find static -name '*.png'`
returns exactly five files, all of them Leaflet's own marker and layer sprites.
There is no bundled token-art library, no `static/tokens/` directory, and no code
that references one. `wolf.png` / `dire-wolf.png` appear only in
`internal/plugins/maps/drawing_partial_update_test.go`.

What the column actually is: an unconstrained `VARCHAR(500)` that
`static/js/widgets/map_widget.js:432` assigns **verbatim** to a Leaflet
`iconUrl`. So its real value set is "whatever anyone ever put there", including
a relative filename that resolves against the current page (and 404s), and
including an absolute external URL that makes every viewer's browser fetch from
a third-party host.

Handling, therefore, is in two parts:

- **In the usage table:** run the value through `ResolveReference`. If it yields
  a known media id, write a usage row of kind `map_token`. If it does not, write
  **nothing** — it is not a media reference and must not be forced to look like
  one. No usage row means the file is untouched by the rule, which is correct.
- **Separately (slice M7):** constrain the column on write to one of exactly two
  shapes — a resolvable media id, or a path under an allowlisted
  `/static/tokens/` prefix that we would need to actually ship. Reject
  everything else at the service boundary.

### `bestiary_publications.artwork_media_id` — corrected

**[spot-check — "DEAD, no read/write path exists" is slightly off, and the
difference matters.]** The column *is* in the repository's `SELECT` list
(`repository.go:107`) and in its `INSERT` (`repository.go:139`), scanning into
and out of `Publication.ArtworkMediaID`. What does not exist is anything that
ever **sets** that field. So the plumbing is complete and the value is
unconditionally `NULL`.

That is better news than a truly dead column: the usage writer needs no new SQL,
only a caller. Recommendation — leave it `NULL` and give it a usage kind anyway,
registered with a resolver, so that the day somebody wires bestiary artwork it is
covered by construction rather than by a later audit. Cost: about fifteen lines
and one test.

### Visibility resolvers, and the guard that keeps them honest

A usage row is worthless unless something can answer *"is this subject visible to
this viewer?"*. One registry, keyed on `subject_kind`:

| `subject_kind` | Resolves via | Exists today? |
|---|---|---|
| `entity` | `entities.FilterViewableEntityIDs` (the existing `EntityVisibilityFilter` seam) | yes |
| `map` | maps visibility | **no — build in M5** |
| `map_token` | the token's map, plus `is_hidden` | **no — build in M5** |
| `note_attachment` | note ownership / shared flag | **no — build in M5** |
| `campaign_chrome` (backdrop, topbar) | can the viewer see the campaign at all | trivial |
| `user_avatar` | see Open Question 2 | trivial |
| `bestiary_publication` | publication visibility | **no — build in M5** |

Reuse the existing seams. `entity` must go through the **same**
`entityVisibilityFilterAdapter` instance that `internal/app/routes.go` already
wires for sessions / npcs / armory / media — never a fourth copy of the
predicate. That discipline is what ADR-058 bought and it should not be spent.

**The guard.** A registered writer with no registered resolver is a silent
authorization hole. Pin it mechanically: a test that inventories every
`subject_kind` any writer can emit and fails if one has no resolver. With that
guard the runtime "unknown kind" branch is genuinely unreachable and can safely
fail closed.

---

## 2. Unifying the reference columns onto IDs

**Recommendation: unify the *answer*, not the *columns*.**

The usage table is the single ID-shaped truth. The nine existing columns keep
their current values and their current meaning; every interpretation of them goes
through `ResolveReference`; every render goes through `layouts.MediaURL`, which
already normalizes. No column is renamed, no column's semantics change.

### The cost of doing it properly, measured

Non-test, non-generated Go files that read or write one of these fields: **39**.
Templ and JavaScript files: **20**. Total **~59 files across ten plugins**
(`entities`, `campaigns`, `maps`, `media`, `armory`, `npcs`, `syncapi`, `admin`,
`auth`, `bestiary`) plus `internal/app/routes.go` and
`internal/app/export_adapters.go`.

### Three reasons that is the wrong spend

1. **It does not remove the need for the resolver.** `internal/plugins/campaigns/
   export.go` carries `image_path` verbatim in the export JSON. Every backup file
   any operator has ever taken contains old-shape values. A restore in 2028 will
   still hand us a disk path. So the code that understands all three shapes has
   to exist and keep working **whatever we do to the columns** — which means a
   rename buys tidiness and no correctness.
2. **Chronicle's migrations are append-only.** A rename is `ADD COLUMN` →
   backfill reconciler → dual-write for a release → `DROP COLUMN` in a later
   one. Three releases minimum, per column, with a live-data window in each.
   Against nine columns that is a programme, not a slice.
3. **It breaks a published wire contract.** `image_path` appears in the syncapi
   JSON the Foundry module consumes. Renaming it is a cross-repo coordination
   with a module that ships on its own cadence.

### What we do instead, so the ambiguity stops generating bugs

- `ResolveReference` is the **only** place a shape is interpreted.
- A contract test freezes the inventory of media-bearing columns, the way
  `internal/wire/routes_snapshot.txt` freezes routes: adding a tenth reference
  site forces you to classify it out loud and register a resolver, or the build
  fails.
- Every column gets a comment above its Go struct field saying which shape it
  holds and why it is named what it is named. `entities.ImagePath` has carried a
  misleading name since migration 000001; the name stays, the lie gets a
  footnote.

If the operator wants the full rename anyway, it is a clean follow-on programme
after M5 lands and should be scoped separately. It is not a prerequisite for any
of the safety work.

---

## 3. What changes in access control

ADR-058 decision 1 — *"a picture is readable if at least one page using it is
visible to the viewer"* — **does not change**. Neither does the fail-closed
posture, the 60-second per-(file, viewer) cache, the site-admin bypass, the
signed-URL trust rule, or the rejection of anonymous callers on private
campaigns. What changes is the quality of the input.

| | Today (derived by search) | After (recorded) |
|---|---|---|
| Reference lookup | 3-way `UNION`, one arm a `LIKE '%…%'` on `entry_html` across the campaign | one indexed read on `media_usages` |
| Sites covered | 3 of 9 | 9 of 9 |
| Formats matched | ID only | all three |
| Files falling through to role-blind membership | maps, tokens, notes, backdrops, top-bars, bestiary artwork, **plus any page whose image was stored in path form** | in-flight uploads and true orphans only |
| Cost of a cache miss | full scan | indexed lookup + one verify read |

Four behaviour changes worth calling out, because each will surprise someone:

1. **The path-shape hole closes.** `entities.UpdateImage` validates against
   traversal and otherwise accepts any string; `entities.Create` takes whatever
   it is handed, including from an import or restore. `FindReferences` matches
   `image_path = <mediaID>` exactly. Any row holding `2026/09/<uuid>.png` matches
   nothing → looks unreferenced → decision 3 → role-blind. That is live today.
   Treat it as a finding, not a nicety.
2. **HTML-embedded references stop being pattern-matched.** `LIKE '%/media/<id>%'`
   matches a picker-inserted URL (which carries the id) but not a path-shaped one
   (`/media/2026/09/<uuid>.png` — the `<id>` never appears after `/media/`).
   goquery finds both. Expect this to newly *deny* some files that decision 3 was
   quietly permitting.
3. **`entry_html` stops being scanned on the request path.** ADR-058's own
   Consequences said: *"If it proves too slow, the answer is an explicit
   media-to-entity link table, not relaxing the rule."* This is that table.
4. **Stale rows can never grant access.** Because there is no FK to the subject,
   a "yes" must be verified: after finding a usage row that says a visible
   subject uses this file, re-read that subject's own column and confirm it still
   does. One extra indexed read, on the allow path only, on a cache miss only.

### The standing invariant

**The column-derivation fallback is never removed.** On a usage-table miss the
check derives the answer from the columns for that one media id, returns the
correct answer, and enqueues a repair. This is what makes the table a **cache of
a derived fact** rather than a second source of truth, and what makes it safe to
deploy the reader before the backfill finishes, to restore an old backup, or to
import a campaign.

Write it down as an invariant with a test, because "the backfill is done, we can
drop the slow path now" is exactly the optimization a future session will
propose.

---

## 4. Unreferenced files

**ADR-058 decision 3 survives, unchanged in mechanism, much smaller in
population.** Its reasoning still holds: a file that is on no page is being shown
to nobody, and failing it closed breaks avatars, backdrops and every upload that
has not been attached yet — in ways that are near-undiagnosable from a browser.

Avatars, backdrops, top-bar images and bestiary artwork all acquire usage rows
and leave decision 3 for a resolver of their own. What remains:

- **In-flight uploads** — a file created seconds ago, not yet attached.
- **True orphans** — the page that used it was deleted, or it was abandoned.

Recommended refinement: split those two by age. A file with no usage row younger
than 24 hours takes the plain-membership path silently. Older than that, it takes
the same path **and** is listed in **Admin → Data Hygiene** as purgeable.
`admin/hygiene_service.go` already has `OrphanedMediaItem` with a `Referenced`
flag computed the old way; point it at the usage table and it becomes accurate
for the first time.

Avatars are the one genuine widening — see **Open Question 2**.

**Not proposed:** failing orphans closed. It converts a storage-hygiene problem
into an availability problem, and the recovery is not user-discoverable.

---

## 5. The boot backfill

### There is no separate migration tool, and that is fine

The authorization says *"use the migration tool that starts up on docker boot"*.
There is no such separate tool. Chronicle is one binary, one process:

```
config → MariaDB → core migrations (FATAL on error) → startup health checks
       → foundry reconcilers → plugin migrations (per-plugin failure degrades
         only that plugin) → Redis → app.New
       → RegisterRoutes  ← every reconciler and backfill is wired here,
                            synchronously, before the listener starts
       → Start
```

So the instruction maps onto a real thing: **a reconciler wired in
`RegisterRoutes`, running detached, on every boot.** That satisfies it exactly.
Chronicle's own rule (`.ai/conventions.md` #10) forbids a one-time data fix in a
migration: migrations are schema-only and append-only, data fixes are idempotent
reconcilers. The new table's DDL is a migration; everything that fills it is a
reconciler.

### Shape

The precedent is `mediaService.BackfillContentHashes` (`internal/app/routes.go:1809`)
— detached goroutine, `LIMIT 100` batches. Two flaws to fix, one property the
new job **cannot** copy:

- **Fix 1 — the dead cancellation.** Seeded from `context.Background()`, so its
  `select { case <-ctx.Done(): }` is unreachable. Give `App` a cancellable
  context, cancel it in the existing shutdown handler, seed both backfills from
  it. Fix the content-hash job in the same PR — three lines, same bug.
- **Fix 2 — visibility.** Below.
- **Cannot copy — idempotency by predicate.** `BackfillContentHashes` is
  idempotent purely through `WHERE content_hash IS NULL`, which works because the
  work *mutates the predicate*. Extracting usages does not modify the subject, so
  there is no self-clearing predicate and no way to know where a restart left
  off. Hence a **real checkpoint table**.

```sql
CREATE TABLE IF NOT EXISTS media_usage_backfill (
    subject_kind    VARCHAR(32) NOT NULL PRIMARY KEY,
    last_subject_id CHAR(36)    NULL,
    state           VARCHAR(16) NOT NULL DEFAULT 'pending',
        -- pending | running | complete | failed
    total_estimate  BIGINT      NOT NULL DEFAULT 0,
    scanned         BIGINT      NOT NULL DEFAULT 0,
    usages_written  BIGINT      NOT NULL DEFAULT 0,
    error_count     BIGINT      NOT NULL DEFAULT 0,
    last_error      TEXT        NULL,
    generation      INT         NOT NULL DEFAULT 0,
    started_at      TIMESTAMP   NULL,
    updated_at      TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP
                                ON UPDATE CURRENT_TIMESTAMP,
    finished_at     TIMESTAMP   NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

**Per kind, two phases:**

1. **Extract.** Keyset pagination — `WHERE id > :last_subject_id ORDER BY id
   LIMIT :batch` — never `OFFSET`, which degrades badly and skips rows when the
   underlying set shifts. Resolve every reference in the batch, `INSERT … ON
   DUPLICATE KEY UPDATE last_seen_at = NOW()`, then commit the checkpoint.
2. **Prune.** Only after a *complete* clean pass for that kind: delete usage rows
   of that kind whose `last_seen_at` predates `started_at` for the current
   `generation`. A half-finished pass never deletes anything.

**Pacing.** `MEDIA_BACKFILL_BATCH_SIZE` (default 200) and
`MEDIA_BACKFILL_BATCH_PAUSE` (default 200ms). A self-hosted instance shares one
small container with MariaDB and Redis; a job that walks 50,000 rows as fast as
it can is indistinguishable from an outage. Slow and visible beats fast and
suspicious.

**Recurrence.** Every boot, not once. A clean pass on an already-indexed instance
is nine `COUNT`s and nine empty batches, and it is what heals drift from a failed
write, a restore, or an import.

### Progress and failure visibility — the required part

No existing reconciler has any. This is the operator's explicit ask.

**Surface 1 — `Admin → Storage → Media index`**, one row per subject kind:

```
Entities            ████████████████░░░░  81%    40,512 / 50,000    8,204 uses    0 errors   running   4m12s
Maps                ████████████████████ 100%       312 / 312         289 uses    0 errors   complete  0m03s
Map tokens          ████████████████████ 100%     1,904 / 1,904        12 uses    3 errors   complete  0m11s
  last error: token 8f2a… — image_path "http://example.net/w.png" is not a media reference
Notes               ░░░░░░░░░░░░░░░░░░░░   0%         0 / 2,140         0 uses    0 errors   pending   —
```

Backed by `GET /admin/storage/media-index`, an HTMX fragment on
`hx-trigger="load, every 3s"` while any kind is `running`, once otherwise. One
new route → **regenerate `internal/wire/routes_snapshot.txt` in the same PR**
(`UPDATE_ROUTES_SNAPSHOT=1 go test ./internal/wire/...`). Every number comes from
the checkpoint table, so the panel is correct across a restart.

**Surface 2 — the admin dashboard.** A banner while any kind is `running`, and a
**persistent** one if any kind finished with `error_count > 0` or
`state = 'failed'`, carrying a **Retry** button. A job that fails into silence is
the thing being fixed here.

**Surface 3 — logs.** One `INFO` at phase start with the estimate, one per 10%,
one at phase end with totals, one `WARN` per 1,000 row errors (not per error — a
systematically broken kind must not fill the disk), one `ERROR` on a batch
failure. Every line carries `subject_kind` so `grep` works.

### Failure semantics

- **Row-level error:** counted, `last_error` updated, batch continues.
- **Batch-level error:** `state = 'failed'`, that kind **stops**. No retry loop.
  The banner shows it; the operator retries.
- **Shutdown:** context fires, current batch finishes, checkpoint commits, exit.
  Next boot resumes.
- **Never fatal to boot.** The listener starts regardless, because §3's fallback
  means a half-built index is slow, not wrong.

---

## 6. Format handling

Checked against the live package index in September 2026, not recalled.

### The build constraint that decides most of this

`Dockerfile` stage 2 builds `CGO_ENABLED=0 GOOS=linux`; stage 3 is `alpine:3.20`
at ~30 MB carrying one static binary. Any cgo dependency ends the static build
and requires native libraries in *both* stages.

### Adopt

**`github.com/gabriel-vasile/mimetype`** — MIT, pure Go, **zero dependencies**,
v1.4.15 published 2026-07-23. Replaces the hand-rolled `validateMagicBytes` table
at `internal/plugins/media/service.go:862`.

Not a tidy-up. That table has a real defect: `audio/webm` is validated by testing
the four-byte EBML header `1A 45 DF A3`, which is the Matroska container magic
shared by `.mkv`, WebM **video**, and WebM audio. Any Matroska file — including a
video of arbitrary size — currently passes as `audio/webm`, skips the image
re-encode path (guarded on `image/` prefix), and is stored and served verbatim.
`mimetype` distinguishes them by inspecting track entries. It also fixes the
loose MP3 check (a two-bit frame-sync test many binaries satisfy by accident).

**`github.com/google/uuid`** — already a direct dependency, already used in seven
packages. The media plugin is the outlier hand-rolling UUIDv4 from `crypto/rand`
at `service.go:916`, complete with a `panic()` on the request path. Delete it,
call `uuid.NewString()`.

### Adopt decode support, not encode

**`github.com/gen2brain/avif`** — MIT, v0.6.0 published 2026-07-05. The
interesting finding: it is **CGo-free**. libavif with dav1d and aom are compiled
to WebAssembly and run on **wazero — which Chronicle already carries as a direct
dependency** (`github.com/tetratelabs/wazero v1.9.0`, via extism). So AVIF works
under `CGO_ENABLED=0` with no Dockerfile change. That contradicts the usual
assumption that AVIF means cgo.

**Take the decoder, decline the encoder, for now.**

- **Decode** closes a real user-facing gap. Modern iPhones shoot HEIC and recent
  Android shoots AVIF-adjacent formats; Chronicle's allowlist rejects both, so
  players hit "unsupported file type" on photographs from their own phones.
  Accepting them and re-encoding to JPEG — exactly what the pipeline already does
  to WebP — fixes that with no change to the security model. (`gen2brain/heic` is
  the companion decoder; same author, same licence, same approach.)
- **Encode** is the part to defer. aom-on-WASM encoding is seconds per megapixel.
  Uploads today are synchronous inside the HTTP request. Adopting AVIF as an
  output format is therefore an **asynchronous transcode queue** decision, not a
  format decision.

### Optional, small

**`github.com/HugoSmits86/nativewebp`** — MIT, pure Go, v1.3.0 published
2026-05-10, supports animation through `EncodeAll`. One limitation decides how it
is used: **it encodes lossless (VP8L) only**, which on a photograph is usually
*larger* than a quality-92 JPEG. So it is the wrong universal output format and
exactly the right tool for one job: **preserving animated GIFs**, which the
current pipeline destroys (`sanitize.go:60` re-encodes only the first frame).
Decode all frames with stdlib `image/gif.DecodeAll`, re-encode with
`nativewebp.EncodeAll`. Frame disposal methods and per-frame palettes make this
fiddlier than it reads.

### Decline

**govips / libvips.** Both open source, both maintained (govips MIT v2.18.0
2026-03-31; libvips LGPL-2.1-or-later). Neither licence is an obstacle.

**On the GLib memory-fragmentation claim — verified, and partly corrected.** The
claim is real and documented: libvips allocates through GLib, GLib creates one
malloc arena per thread, and heavily-threaded programs show RSS growing without
Go heap growth. imgproxy documents it with a reported case of ~150 images driving
RSS to 1.6 GB. **But it is mitigable**, with `MALLOC_ARENA_MAX` or jemalloc.
imgproxy runs libvips in production at scale doing exactly that. So "documented
fragmentation" alone is **not** a sufficient reason to refuse it, and the prior
lean should not rest there.

**The sufficient reason is the Dockerfile.** govips requires cgo and libvips
8.14+ in both stages. On Alpine/musl that means installing vips and its native
stack — glib, expat, libjpeg-turbo, libpng, libwebp, giflib, lcms2, orc — twice,
losing the static binary, and growing the runtime image roughly five-fold.
Against that, Chronicle's entire image workload is two thumbnails per upload on a
single-instance self-hosted box. A 4–8× faster resize is imperceptible at that
volume, while the costs — larger image, slower CI, a new class of native CVE, the
loss of "one static binary" as a deployment property, and two allocator
environment variables the operator must never accidentally drop — are permanent
and all paid by the operator.

**Also decline `github.com/disintegration/imaging`** — widely recommended, last
release 2021, effectively unmaintained. `golang.org/x/image/draw` with CatmullRom
is already in use and is the right answer.

| Library | Licence | Last publish | cgo? | Verdict |
|---|---|---|---|---|
| `gabriel-vasile/mimetype` | MIT | 2026-07-23 | no | **Adopt** — M6 |
| `google/uuid` | BSD-3 | already present | no | **Adopt** — M6 |
| `gen2brain/avif` (+ `heic`) | MIT | 2026-07-05 | no (wazero, already present) | **Adopt decode only** — M6 |
| `HugoSmits86/nativewebp` | MIT | 2026-05-10 | no | **Optional** — M6b, animated GIF only |
| `davidbyttow/govips` + libvips | MIT / LGPL-2.1+ | 2026-03-31 | **yes** | **Decline** — Dockerfile cost |
| `disintegration/imaging` | MIT | 2021 | no | **Decline** — unmaintained |

**Not proposed as new, because it already exists:** EXIF/metadata stripping.
`media/sanitize.go` already does full decode → re-encode on every image upload,
which strips EXIF, IPTC, XMP and ICC and destroys polyglots. The 10,000-pixel
dimension cap is already there. SVG stays blocked. Any proposal claiming to add
these is describing work already done.

---

## 7. Slicing

The governing rule is **one slice,
one PR**. Every slice must be safe to ship half-done and safe to review alone.

| Slice | What lands | Depends on | Reviewable alone? |
|---|---|---|---|
| **M0** | Avatars join the media pipeline | — | yes |
| **M1** | `ResolveReference` + the column inventory guard | — | yes |
| **M6** | mimetype, `google/uuid`, HEIC/AVIF decode | — | yes |
| **M7** | Token-URL constraint, dead-reconciler cleanup | — | yes |
| **M2** | `media_usages` + writers (nothing reads it) | M1 | yes |
| **M3** | Boot backfill + admin visibility | M2 | yes |
| **M4** | The access check reads the table | M3 | yes |
| **M5a…e** | One subject kind per PR: maps, tokens, notes, bestiary, chrome | M4 | yes, each |

Four slices are mutually independent. The chain is only three deep before it pays
off.

### M0 — Avatars join the media pipeline · *do this first*

Today `auth/handler.go:499` writes to `filepath.Join("uploads", "avatars")` — a
path **relative to the working directory**, which is `/app`. The Docker volume is
mounted at `/app/data`. So avatars land at `/app/uploads/avatars/…`, outside the
volume, and vanish on the next container rebuild. Separately, the stored web path
is `/uploads/avatars/<name>` and **no route serves `/uploads/*`** — `app.go:122`
registers only `e.Static("/static", "static")`. So every avatar 404s even before
it is deleted. And the handler bypasses `mediaService.Upload`: no magic-byte
validation beyond `http.DetectContentType`, **no EXIF stripping, no re-encode, no
polyglot destruction**, no quota, no disk-space check, `0644` permissions where
media uses `0640`.

Three faults, one cause, one fix: route the handler through `mediaService.Upload`
with `usage_type = "avatar"` and no campaign, store the returned media id in
`users.avatar_path`, render through `layouts.MediaURL`. Delete the dead
`/uploads/` path. Add a small reconciler that relocates any surviving
`./uploads/avatars/*` file into the media store and rewrites the column — on most
instances it will find nothing, because the files are already gone.

First because it is smallest, because it is a user-visible fix the operator can
confirm in thirty seconds, and because it turns avatars into ordinary media files
so M2 has one fewer special case.

### M1 — One resolver, one shape

`media.ResolveReference`, the existence check `normalizeMediaID` lacks, every read
site routed through it — **including `static/js/widgets/appearance_editor.js:795`,
which still builds `'url(/media/' + image_path + ')'` for the live preview and was
missed by commit `efc020fe`**. Plus the frozen column inventory. No schema change.

### M2 — The usage table and its writers

Core migration for both tables. **Bump `ExpectedCoreMigrationVersion` in
`internal/database/migrate_state.go` or `TestExpectedCoreMigrationVersion_MatchesMax`
fails.** Idempotent DDL. The `UsageRecorder` interface, the resolver registry, the
CI guard, and writers on every site. **Nothing reads the table.** Pure write-side,
inert, individually reviewable.

### M3 — The boot backfill and its admin surface

The reconciler, the checkpoint, the cancellable context (fixing
`BackfillContentHashes` in the same PR), the admin panel, the dashboard banner,
the retry, the routes-snapshot regen. Still nothing reads the table for access
decisions. **This is the slice the operator should personally watch run against
their real library.**

### M4 — The access check reads the table

`checkEntityScopedAccess` becomes `checkUsageScopedAccess`. Usage lookup →
per-kind resolution → verify-the-winning-row on allow → fall back to column
derivation on miss. Retire the `entry_html LIKE`. Ship behind `MEDIA_USAGE_AUTHZ`
(default on, `0` reverts) for exactly one release, because this is the slice that
can take away access somebody has today. Release note required.

### M5 — Coverage widening, one kind per PR

Maps, tokens, notes, bestiary, chrome. Each PR: one resolver, its tests, its
backfill phase, and **a reachability test in the style of
`internal/plugins/media/existence_oracle_reachability_test.go`** — construct the
path, do not assert the guard. A kind not yet done simply has no usage rows and
keeps today's behaviour, because M4 already handles the miss.

### M6 / M7 — Formats and cleanups

M6 as in §6. M7 folds in `packages.ReconcileOrphanedInstalls`
(`internal/plugins/packages/service.go:1218`), which is declared, implemented, and
**called by nothing** — verified across the whole tree. Either wire it into the
boot reconciler set with the same admin visibility, or delete it. Do not leave a
third option. M7 also carries the `map_tokens.image_path` shape constraint and
points Data Hygiene's `Referenced` flag at the usage table.

---

## 8. What could go wrong

Ordered by how much it would hurt, not by likelihood.

1. **Avatars are already gone and nothing brings them back.** Not a risk — a
   fact. M0 stops the bleeding; it does not recover a file. Release note.
2. **M0's reconciler is the only genuinely destructive write in this plan.** It
   rewrites `users.avatar_path`. Everything else only INSERTs into new tables.
   Mitigation: write the new value only after verifying the file is present, and
   log the old value on every row. Recovery is the pre-migration backup. **Not
   recoverable:** rows whose old value already pointed at a deleted file — which
   is the common case, which is the whole problem.
3. **A wrong resolver is a leak, not an error.** If `map` resolves to "any
   campaign member" while the map is DM-only, M4 makes that file *more* readable
   than today, for nothing, and the tests all pass. This is why every M5 kind
   needs a constructed reachability test. All nine findings tested in the
   2026-09-12 audit were reachable; do not assume a new one is theory.
4. **M4 takes away access people have today.** Booked in ADR-058; widened here to
   maps, notes and bestiary. The kill switch buys one release to discover this;
   it does **not** make the change reversible once data has moved on.
5. **`entry_html` parsing widens the blast radius beyond what ADR-058 suggests.**
   goquery finds references the `LIKE` never matched, so files comfortably in
   decision 3's permissive path become subject to decision 1 and some will newly
   deny.
6. **The index can be wrong in both directions.** Stale-positive is closed by
   verify-on-allow. Stale-negative degrades to column derivation — correct but
   slow — and immediately after M3 deploys on a large library a burst of misses
   can be slow enough to *look* like an outage. The 60-second cache and the
   admin panel's plain-words status are the mitigations.
7. **The backfill runs against live production data on a live instance.** It only
   reads subjects and writes its own two tables, and never blocks boot. But it
   competes for the same MariaDB as every request on a single small container.
   That is what the batch pause is for, and why the first real run should be
   watched rather than assumed.
8. **Dedup refusals increase, and so does disk.** With nine reference sources
   instead of three, more matches have referencing pages, so more merges are
   refused and more genuine duplicates are stored. Correct, intended, and it
   costs storage. Worth watching in `GET /admin/storage`.
9. **Export and import do not carry usage rows.** An imported or restored
   campaign has an empty index and lives on the fallback until the next boot.
   That is correct, and it is **the reason the fallback can never be removed**.
   If a future change removes it as "dead code now the backfill is done", every
   restored campaign silently reverts to role-blind access. Pin the invariant.
10. **Two mechanical CI guards will bite.** `ExpectedCoreMigrationVersion` must be
    bumped with the new migration, and `routes_snapshot.txt` regenerated for the
    admin endpoint. Both have broken this repo before.
11. **`media_files.campaign_id` is `ON DELETE SET NULL`, and a NULL campaign is
    treated as public.** `checkMediaAccess` returns `nil` immediately at
    `handler.go:350` for any file with no campaign. The primary delete path is
    safe — `campaignService` calls `DeleteCampaignFiles` first — but if that
    cleanup is ever partial or unwired, the surviving rows become world-readable.
    Out of scope; recorded because it interacts with §4.
12. **None of this fixes a signed link leaving the building.** ADR-058 decision 6
    binds a signed URL to a viewer; it remains a bearer token for that viewer for
    up to an hour. The usage table changes who may *mint* a link, not what a
    leaked one does.

---

## OPEN QUESTIONS FOR THE OPERATOR

Four decisions. Each changes what gets built.

### 1. How far should the rule reach?

Right now, "who may see this picture?" is answered properly only for pictures on
**entity pages**. Pictures on maps, on map tokens, as note attachments, or as
bestiary artwork fall back to: anyone in the campaign, any role.

- **Narrow** — fix the entity case (make it fast, close the format hole), leave
  the other five. Cheaper, three slices, and leaves a DM-only map's background
  readable by every Player in the campaign.
- **Wide** — cover all nine, one per PR. Roughly five more slices, each small,
  each may take a picture away from somebody who can see it today.

**Recommendation: wide, staged one kind per PR.** "Narrow" is not a stable
resting place — the whole point of ADR-058 is that a picture inherits the
permissions of what it is on, and a map is exactly the sort of thing that is
deliberately hidden. Staging means each change is separately revertible.

### 2. Who should be able to see somebody's profile picture?

Avatars have no campaign, so "are you in this campaign?" is the wrong question.

- **Any signed-in user.** Simple, matches nearly every application, member lists
  always show the right face. Cost: someone who shares no campaign with you can
  fetch your profile picture if they learn its address.
- **Only people who share a campaign with you.** Tighter. Cost: an extra database
  lookup on every avatar shown, and a class of "why is this person's picture a
  grey blob" support question that is genuinely hard to diagnose.

**Recommendation: any signed-in user.** A profile picture is something you chose
to publish to the instance you joined; the leak is low-value; the tighter option
adds a per-image lookup to the most frequently rendered image on the site. The
resolver is one function, so revisiting it later is small.

### 3. Should Chronicle accept photographs taken on a phone?

Today: JPEG, PNG, WebP, GIF. Modern iPhones save HEIC and recent Android saves
AVIF-family formats. Both are **rejected outright** — so a player photographing
their character sheet hits a wall and most will not know to convert it first.

- **Accept them.** Add decoders (MIT, and — verified — needing no Docker change,
  because they run on a WebAssembly engine Chronicle already ships). Incoming
  HEIC/AVIF converts to JPEG on upload, exactly as WebP already does. Cost: a few
  MB of embedded decoder, and decoding a large phone photo takes longer.
- **Keep rejecting them.** Nothing to build. Users convert files themselves, or
  give up.

**Recommendation: accept them, decode only.** It is the one item here that makes
the product visibly better rather than merely safer, it costs no change to how
images are stored or secured, and "convert it yourself first" is exactly the
friction that makes people stop uploading. Note what this does *not* include:
producing AVIF as an output format, which needs a background queue and is a
separate, larger decision.

### 4. Once the index is built, should Chronicle trust it, or keep double-checking?

The index can go out of date — a page is deleted, a picture swapped, a backup
restored from before the index existed.

- **Trust it.** Fastest. One read per request; when the index is wrong the answer
  is wrong, which in the bad direction means showing a picture whose page no
  longer exists.
- **Double-check.** When the index says yes, re-read the one page that produced
  that answer and confirm it still uses the picture. One extra read, only on a
  yes, only on a cache miss. And when the index has no entry, work the answer out
  the slow way rather than assuming "not used".

**Recommendation: double-check, permanently.** It is what makes the index a
*speed-up* rather than a second source of truth, and what makes it safe to turn
on before the rebuild finishes, to restore an old backup, or to import a
campaign. The cost is one indexed read on the permissive path. The alternative's
failure mode is a picture served to someone who should not have it — the failure
this whole piece of work exists to prevent.

---

## References

- `.ai/decisions.md` §ADR-058 — the rule this implements, and its "Rejected"
  section, which names this table as the fallback if query cost bites. It bit.
- `.ai/conventions.md` §"Migration Safety Rules" #7 and #10 — why `media_usages`
  is polymorphic and why the backfill is a reconciler.
- Commit `efc020fe` — the shape-ambiguity bug that kept campaign backdrops and
  top-bar images from ever loading, and the test that passed for months against a
  fixture no upload path could produce.
