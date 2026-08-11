# Project Status

<!-- ====================================================================== -->
<!-- Category: DYNAMIC — thin index, not a session log                        -->
<!-- Purpose: Cross-cutting project state + index of per-plugin .ai.md files. -->
<!-- Update: When release status / branch state / cross-cutting work changes. -->
<!-- ====================================================================== -->

## For humans

### What this file is

A thin index. It documents Chronicle's current high-level state (release version, active phase, cross-cutting items) and points at where per-plugin status lives — each plugin owns its own `.ai.md` per the convention in `cordinator/reports/chronicle/2026-05-21-c-hygiene-audit.md §0.5 D2=(c)` + `2026-05-23-c-plugin-isolation-audit.md §2.3`.

### What this file is NOT

It is no longer a chronological session-recap log. Pre-2026-05-23 session recaps (51 numbered entries spanning ~135 KB) live in `.ai/archive/status-2026-04-25-pre-shrink.md`. Going forward, per-session deliverables are tracked by the dispatch workflow (`cordinator/decisions/2026-05-19-dispatch-workflow.md`): one report per dispatch in `cordinator/reports/chronicle/`, plus the PR itself.

If you're an AI session looking for "what shipped last week", read the Cordinator repo's `main` branch and grep `reports/chronicle/` by date — books and reports live on cordinator `main` now, not a working branch. If you're looking for plugin-specific architecture, footguns, or recent work on plugin X, read `internal/plugins/<X>/.ai.md`.

## For AI sessions

### Chronicle can now fingerprint ITSELF — `host.build` / `host.runtime` (2026-08-11)

Chronicle could hash every file of every installed system package and could say
nothing whatsoever about its own binary. On 2026-08-11 that cost an hour of
shell archaeology and produced a conclusion that had to be retracted: the
container image's labels (`org.opencontainers.image.revision=33f4cb07`,
`created=2026-02-19`) were read as evidence that the running binary was six
months stale. It had been built minutes earlier. The labels were accurate about
a February image still sitting in the deploy host's local image store — they
simply were not about that process. `docker inspect <tag>` answers for whichever
image holds the tag *now*, never for the image a running container was created
from.

**The rule this encodes: only the process can testify about itself.**

- `internal/hostinfo` (new leaf package, stdlib only) reads build identity from
  inside the process — `debug.ReadBuildInfo()`, `os.Executable` + `os.Stat`,
  hostname, pid, a package-init start timestamp — and pairs every field that can
  be absent with a "do we know?" flag, so "not stamped" is printable as a fact.
- `host.build` (first entry in the operator diagnostic catalog, ahead of every
  `system.*`/`packages.*` check, because those are worthless if the binary is
  not the one you think it is) renders all of it plus a standing note naming the
  Docker-label trap and the `//go:embed` grep trap from the same incident.
- `host.runtime` is the cheap uptime / goroutines / memory / GC snapshot.
- **Measured, and the reason `host.build` leads with the executable mtime:**
  images built by CI carry **no** VCS stamps. `golang:1.24-alpine`'s only
  `apk add` is `ca-certificates`, so the builder has no `git`, and Go then skips
  stamping *silently* (exit 0, no warning). A local `go build` here **is**
  stamped. Absent stamps mean "never recorded", never "old".
- `GET /api/version` now falls back env → VCS revision → main module version →
  `unknown`. It had returned the literal `"unknown"` on every image ever
  shipped, because `CHRONICLE_VERSION` is set by no Dockerfile, compose file,
  Makefile or workflow. `docs/deployment.md` claimed otherwise and is corrected.

~~Open: nothing wires `CHRONICLE_VERSION` or gives the builder `git`~~ —
**closed the same day by the deploy-hygiene change below.** Dockerfile stage 2
now installs `git`, so the toolchain stamps `vcs.revision` by itself and
`/api/version` reaches the revision branch of that fallback instead of
`unknown`. Verified by building the real `internal/hostinfo` chain: with the
env var unset, `hostinfo.Version()` returns `b1fe69e0a1a7-dirty`; with
`CHRONICLE_VERSION=v1.2.3` set, it returns `v1.2.3`.

### Deploy hygiene — the four things that made a correct deploy look like a failed one (2026-08-11)

Companion to the two sections above. Those made Chronicle able to *answer*
"what am I running"; this made the deploy path stop *producing* the question.
No migration, no Go code — `Dockerfile`, `Makefile`, `docker-compose.yml`,
`docker-compose.build.yml` (new), `.github/workflows/ci.yml`,
`docs/deployment.md`.

- **The OCI labels were never wrong — the inference was.** Verified against
  `docker/metadata-action`'s own `src/meta.ts`: `revision` is `context.sha`
  (the commit built) and `created` is `new Date()` in the `Meta` constructor
  (real build time). Both correct. Overriding `created` with a commit
  timestamp — the obvious-looking "fix" — would have made it wrong for the
  first time. What was missing was that a *locally* built image carried **no**
  `org.opencontainers.*` labels at all (`metadata-action` labels only what CI
  builds; `alpine:3.20`'s `config.Labels` is null, so nothing is inherited).
  The Dockerfile now sets an ARG-driven label floor for every image however
  built, and both label sites carry the "a label describes an image, never a
  process" warning where the mistaken reader is already looking.
  **Do not put `{{ }}` in a custom label passed to `metadata-action`** — it
  compiles them through Handlebars (`setGlobalExp`), so the `docker inspect
  --format` template lives in the docs instead.
- **The binary now names its own commit.** `apk add git` in stage 2 — no
  `-ldflags`, no version variable, deliberately no second source of a fact
  `internal/hostinfo` already resolves. `safe.directory` ships with it because
  installing git converts a class of git error from "no stamp" into **"no
  image"**: measured, a foreign-owned checkout gives `error obtaining VCS
  status: exit status 128` and a failed build, and the exception clears it.
  CI passes `CHRONICLE_VERSION` **only for `v*` tag builds** — on `main` the
  metadata version is the literal `latest`, which would be a downgrade from
  the SHA. A new CI step greps the pushed image (addressed by digest) for the
  commit SHA, so if anyone drops the `apk add git`, the silent failure becomes
  a loud one.
- **`make build` was not a production build.** On a clean clone it exited 2
  (`*_templ.go` is generated and gitignored) **and left a previously-built
  `./bin/chronicle` byte-identical on disk** — measured by planting a fake
  binary in a fresh clone. It now depends on `templ` and `rm -f`s the target
  first, so a failure cannot leave a plausible artifact behind.
- **`docker compose up -d` was not an upgrade.** The `chronicle` service
  declared both `image:` and `build:`, giving one tag two producers, and `up`
  neither rebuilds nor re-pulls when the tag already exists locally. `build:`
  moved to `docker-compose.build.yml`, which tags local builds
  `chronicle:local` so they can never masquerade as the published image;
  the base file gained `pull_policy: always`. `make docker-all-local` is the
  from-source path. Two places in `docs/deployment.md` that told operators to
  run `docker compose build --no-cache chronicle` — i.e. to walk into the trap
  — now point at `pull` or the override.

### …and can now account for its own ASSETS — `host.assets` / `host.embedded` (2026-08-11)

The same incident had a second wrong turn: a `grep` of `/app/static` for a
calendar feature's assets came back empty and was read as missing code. It was
not. **Chronicle serves its front-end from two storage mechanisms that look
nothing alike from a shell**, and only one of them is reachable by `ls`:

- the **on-disk static root** — a bare relative `static` resolved against the
  process working directory (`/app/static` in the container, `<repo>/static` in
  dev). There is no config field and no environment variable for it anywhere in
  the tree; `internal/app/app.go` and `internal/templates/layouts/assets.go`
  hardcode the same literal independently.
- each plugin's **`//go:embed`-ed FS**, which exists only *inside the binary*.
  No `ls`, no `grep`, no `find` over the container filesystem can ever see those
  bytes. Measured: 12 embedded files across exactly two plugins (11 calendar,
  1 entities). The calendar's only CSS lives there, so grepping `/app/static`
  for it will **always** return empty — the expected result, not a finding.

Four diagnostics, in `internal/systems/operator_diag_assets.go`, registered
directly after `host.build`/`host.runtime`:

- `host.assets [<path-substring>]` — walks the on-disk root (**resolved**, never
  hardcoded, and it prints the working directory it resolved against, so "the
  app is looking in a directory that isn't there" is visible at all) and reports
  per file size, sha256[:16], mtime, and **the exact `?v=` token
  `layouts.AssetURL` emits** — by calling that function, not by reimplementing
  its hashing. `FullDump` because the no-arg listing measured 89 rows / ~15 KB;
  the gate applies to batches only, so the admin Run button is unaffected.
- `host.asset-contains <relative-path>:<markers>` — the marker check for one
  on-disk file. Verified end-to-end: `css/calendar-block.css:moonpick` → FOUND,
  52 occurrences.
- `host.embedded [<slug>]` / `host.embedded-contains <slug>:<path>:<markers>` —
  the same for bytes that have no path to point a grep at. The plugin registry
  arrives by dependency inversion (`SetEmbeddedAssetsProvider`, mirroring
  `SetInstalledPackagesProvider`); an unwired provider says **"provider not
  wired"** and explicitly *not* "no embedded assets", because for someone
  hunting code that looks missing those are opposite conclusions.

**The `?v=` column is the load-bearing part, and it is a comparison, not a
readout.** Digests are memoised for the process lifetime (correct for caching,
a trap for a diagnostic), so the token the app serves and the sha256 of the
bytes on disk *right now* can disagree — and the disagreement is the finding.
Three verdicts, measured apart: `✓ matches` (verified live — `?v=fdbff5f7d5` is
exactly the first 10 chars of sha `fdbff5f7d5038cdc`), `STALE` (file changed
under a running process), and `BUILD-TOKEN FALLBACK` (the app cannot resolve the
file at all, though it is right there on disk).

Running the real thing caught a defect the unit tests would not have: the shared
fallback verdict advised "check the working directory", which is meaningless for
bytes compiled into a binary. Embedded assets have no working directory — a
fallback there means the plugin FS was mounted for serving but never registered
for hashing. The verdict now states the fact and each diagnostic explains its own
cause; a test pins that the embedded output never gives the on-disk advice.

### …and can now show its own recent ERRORS — `host.errors` (2026-08-11)

Third gap from the same incident, and the one the operator asked for by name.
An error that fired at 2am left **no trace an admin could reach**: it went to
`slog`, `slog` went to stdout, stdout went to the container's log driver. So
"what broke overnight?" meant getting onto the host, finding the container, and
running `docker logs` — the same shell dependency the rest of `host.*` removes.
The audit plugin cannot fill this: it records USER ACTIONS, is DB-backed, and is
scoped to both a campaign and a user, so a 500 on `/healthz` or an anonymous
request has neither key it needs.

- **`internal/observability`** (new leaf package, stdlib only — writers are
  `internal/app` and `internal/middleware`, reader is `internal/systems`, so a
  leaf with no Chronicle imports cannot cycle with any of them). A 256-slot
  mutex-guarded ring, allocated at package init so it is safe from the first
  request — an error during boot is exactly the one you most want.
- **`host.errors [<count>]`** — newest first (default 25, max 100). Each line:
  timestamp **plus its age** ("2m0s ago" — the first question anyone asks a
  stack of errors is "was that just now?"), status, method, route, provenance,
  message.
- **`host.errors-summary`** — the same ring grouped by route + status with
  counts and first/last seen, sorted by frequency. Three repeats of one failure
  become one line, so the single unrelated 503 underneath is visible instead of
  scrolled off.

**Three decisions that are load-bearing, each named and commented in code:**

1. **Only 5xx and recovered panics are recorded** (`observability.ShouldRecord`).
   Not for volume — for **eviction**. The ring evicts oldest-first, so admitting
   the routine 4xx background of any public server (crawlers, stale bookmarks,
   expired CSRF) would let a 404 storm quietly evict the one 500 you came for,
   while the ring still *looked* full and healthy. Both renders state the policy
   unconditionally, because an absence of 4xx here says nothing about whether
   4xx are happening.
2. **Paths are route TEMPLATES, never the requested URL** (`observability.PathFor`).
   Chronicle really does route `/rsvp/:token`, `/proposals/respond/:token`,
   `/calendar-rsvp/:token` and `/join/:code` — a concrete path would put a live
   credential into output whose entire purpose is to be pasted into a chat
   window. Templates also group perfectly, which is what makes the summary work.
   The concrete path is used only when the router matched nothing, and that case
   is a 404, which is not recorded.
3. **A panic is hooked SEPARATELY from the error handler, because it never
   reaches it.** `middleware/recovery.go` writes its 500 with `c.String` and
   returns `nil`, so Echo sees no error and `app.errorHandler` is never called.
   Hooking only the error handler — the obvious single hook — would have left
   the most valuable error class invisible while the diagnostic looked like it
   was working. The panic *value* is stored; the stack stays in the log.

**Empty must never read the same three ways.** "Provider not wired", "wired and
holding zero", and "the ring wrapped and evicted 407 earlier errors" are three
different situations with three different renders; the header always prints
capacity, hold count and total-since-start, so "only 4 errors" is visibly
different from "nothing is recording". That is the 2026-08-11 lesson applied
directly: an absence of evidence got read as evidence and cost an hour.

The recording is additive only — `app.errorHandler` records and then delegates
to its existing behaviour completely unchanged, and
`internal/app/error_handler_record_test.go` pairs every ring assertion with a
wire assertion (status, `Content-Type`, `HX-Retarget`, `HX-Trigger`/`HX-Reswap`)
so a later change cannot quietly move a production error response.

Limits stated in the output itself: in memory, this process only — a restart
empties it and each replica keeps its own.

### …and can now name its WIDGETS and PLUGINS — `host.widgets` / `host.plugins` / `host.deploy-check` (2026-08-11)

The operator's actual words were "calendar (and other widget) versions", and
Chronicle could not answer that in any form. Three additions close it, plus a
rewritten probe library.

- **`host.widgets [<name-substring>]`** — one line per widget: name, which
  storage mechanism it came from, size, sha256[:16], mtime where one exists, and
  the `?v=` this process serves, with the same token verdicts `host.assets`
  uses. Companion stylesheets (`js/<name>.js` ↔ `css/<name>.css`) get their own
  sub-line, because a deploy that changes only the CSS leaves the script's hash
  untouched and would otherwise read as a no-op.
- **`host.plugins`** — per plugin: static mount + URL prefix + embedded asset
  count and aggregate size, whether it contributed migrations, and applied-vs-
  available schema version taken from the migration runner's own record rather
  than recounted.
- **`host.deploy-check [<marker,…>]`** — the composite. Build identity in three
  lines, the bellwether assets, a marker search across **both** storage
  mechanisms reported **separately**, and `packages.installed-vs-loaded`
  delegated verbatim. This is the "paste this one thing after a deploy" check.

**Three things had to be said rather than assumed, and each is printed on every
run:**

1. **Widgets have no version number.** Nothing declares one, nothing stores one,
   and there is no server-side widget registry — a widget is a JS file that
   registers itself with `boot.js` when the browser mounts a `data-widget`
   element. So `host.widgets` says plainly that the honest answer is a content
   fingerprint plus a build time, states that it WALKED two known directories
   because there was no registry to consult, and says that the Go widget
   packages under `internal/widgets/` cannot be enumerated from a deployment at
   all (the source tree does not ship). Inventing a version number would have
   been the easy, wrong answer.
2. **Chronicle has no plugin loader.** Every plugin is compiled in and registers
   its routes unconditionally; the two registries `host.plugins` reads are
   *opt-in metadata* (a plugin joins one only if it needs a static mount or a
   health hook, the other only if it owns tables). Most plugins appear in
   neither. Without that standing note a missing row reads as a missing feature.
3. **One plugin, two spellings.** `foundry_vtt` registers as `foundry-vtt` in
   the metadata registry (that spelling is also its static URL prefix) and as
   `foundry_vtt` in the schema runner and health registry. Merged literally it
   would render as two rows — one apparently migration-less, one apparently
   unregistered. `App.hostPluginRows()` folds `-`/`_` for the merge key only,
   and the row prints both spellings so the alias is taught rather than hidden.

**The probe library is now half warnings.** Both wrong turns of 2026-08-11 came
from commands whose output was truthful about what it measured and misleading
about what the reader wanted. Those probes are kept and annotated, never deleted:

- `image-digest` is retitled **TRAP**, now compares the image the container RUNS
  against the image the tag points at NOW, and states that these labels have
  been observed six months wrong about a binary built minutes earlier — use
  `host.build`.
- `plugin-asset-grep` is a new **TRAP** entry: the empty grep of `/app/static`
  for a plugin asset is the *expected* result, because those bytes are
  `//go:embed`-ed into the executable — use `host.embedded` / `host.widgets`.
- New: `container-restart-time` (`docker compose up -d` neither rebuilds nor
  re-pulls when the tag already resolves locally, and compose declares both
  `image:` and `build:` for the same service), `binary-in-container` (executable
  mtime + container clock from outside the process), `page-asset-tokens`,
  `plugin-schema-versions`.
- Probes a `host.*` check now answers better (`package-version-dirs`,
  `package-file-marker`, `chronicle-logs`, `served-widget-version`) say which
  one to prefer, in their own text. Deleting them would leave an operator who
  remembered the command running it with no note attached.

### `host.*` INTEGRATED — one catalog, not five bolt-ons (2026-08-11)

The five sections above landed as five separate stages on one branch. This is
the pass that made them a single tool, and it found the defects that only exist
*between* stages — the kind no stage's own tests can see.

- **A stage-4 diagnostic was telling operators to run a command stage 5 had
  deliberately broken.** `host.deploy-check`'s remedy section and the
  `container-restart-time` probe both said Chronicle's compose file "declares
  BOTH `image:` and `build:`" and told the reader to run `docker compose build`.
  Stage 5 had removed `build:` from the `chronicle` service and added
  `pull_policy: always`, so that command now **fails by design**. Both texts now
  describe the shipped compose file, tell the reader to verify their own file
  matches (a lingering `build:` block means they are on the old shape), and
  point at the `docker-compose.build.yml` override for source builds. This is
  the highest-stakes prose in the feature — it is the remedy an operator reads
  at the worst moment — and it had gone stale within one commit.
- **Every provider is now PROVEN wired, not reasoned wired.** All five stages
  recorded the same gap: `systems.Set*Provider` wiring was compile-checked only,
  because no unit test executes `RegisterRoutes`. An unwired provider is worse
  than a missing diagnostic — `host.plugins` would report no plugins and it
  reads like a finding. `internal/app/operator_diag_wiring_test.go` walks the
  **AST** of both packages and fails if any declared setter has no non-test call
  site, or if that call site is not reachable from `RegisterRoutes`. Measured:
  all 8 providers resolve to `RegisterRoutes()`, which `cmd/server/main.go:157`
  calls. **The first version of that test was itself broken** — it grepped raw
  source, so commenting out the wiring line still passed. Walking the AST is not
  fastidiousness; it is the difference between a guard and a decoration.
- **The catalog is a menu again.** The 11 `host.*` entries arrived carrying
  their incident narrative in `Desc` — up to 588 bytes against the pre-existing
  catalog's 177-byte mean — which pushed the menu an AI reads to choose from
  8.9 KB to 13 KB of functions spec. `Desc` is documented on the struct as
  *one-line*. Every trimmed caveat was verified to already exist, usually more
  fully, in the diagnostic's own output, so nothing was lost from where a reader
  acts on it. Now capped at 450 bytes by test.
- **`tools/check-plugin-isolation.sh` is green again.** It had been red since
  stage 2 (commit `64dec0f5`), so `make verify` could not pass on this branch.
  The four flagged files are operator-diagnostic *test fixtures* using real
  plugin slugs as data — the guard's own remedy text names that case as
  legitimate. Allowlisted by exact filename, with the reasoning recorded:
  renaming the fixtures was rejected because the incident being guarded was
  specifically about the **calendar** plugin's embedded assets.
- New catalog-wide guards in `internal/systems/operator_diag_catalog_test.go`:
  every `host.*` diagnostic runs with an empty arg (none panics, none returns
  empty, each output self-identifies), names are unique, the family forms one
  uninterrupted block ahead of the rest, and `FullDump` is asserted **in both
  directions** by name.
- Observability recorded as **ADR-051** (what is recorded, what is deliberately
  not, and why the ring is in-memory rather than persisted).
  `docs/operator-diagnostics.md` gains the `host.*` family and its probe table
  is corrected — it was still listing the pre-stage-4 probe set.

**Not verified:** no Docker daemon and no database in this environment, so
nothing here was observed on a real container. `./tools/check-plugin-isolation.sh`
could not be re-executed after the fix (the sandbox refused the invocation); the
change is verified by `bash -n` plus exercising the guard's own matching logic
against the four paths, not by a clean run of the guard itself.

### The operator's first look — six confirmed defects, all fixed (2026-08-10)

Five in this repo, one in `Chronicle-Foundry-Module`. Each was measured before it
was touched; the measurements are in the commits and in the tests, and every fix
has a test proven red without it.

1. **The sky's moon discs painted nothing at any density.** `.ph` is an `<i>`,
   so `inline-size/block-size: 100%` were dead declarations and the disc laid out
   at 0×0 with both pseudo-elements positioned into a zero-size containing block.
   The wrapper was correct the whole time, which is why every shipped guard was
   green. One declaration: `display: block`.
2. **The sky's close rendered not one frame.** `<details>` gives
   `::details-content` `content-visibility: hidden` the instant `open` is
   removed. `content-visibility` is amended into `[SKY-3]`'s "seven and no
   eighth" BY NAME, in the sheet and in the guard, mutation-tested both ways.
3. **`Open in the Ledger` was a no-op wherever the Ledger is docked** — the
   day's own `.dsel` label had already selected the day. Conditioned on the
   stacked layout, from a measured rect rather than a restated breakpoint.
4. **Every month step was a full document load.** The nav trio had no `hx-boost`
   ancestor and its doc comment claimed the sidebar's covered it; boost is
   resolved from the clicked link's ancestors and does not travel.
5. **"Builder →" opened the settings page**, beside the control that opens the
   builder. One string.
6. **The Foundry structure-mismatch banner printed a remedy nobody could
   follow.** Fixed module-side; the Chronicle debt behind it is booked below.

**OPEN CHRONICLE DEBT from (6): there is no way to choose which calendar
Chronicle serves a campaign.** `POST /api/v1/campaigns/:cid/calendar` 409s
whenever the campaign has ANY calendar; `CreateCalendar` marks only the FIRST
calendar `is_default`; the module is served `is_default DESC, sort_order ASC
LIMIT 1`; and `SetDefaultCalendar` exists on the service interface with no route,
no handler and no control. So an authored replacement is invisible across the
wire. See `.ai/todo.md` § 0-fix-R1 for the two candidate fixes.

### An emptied Year field moved the world to year zero — FIXED (2026-08-09)

A driven parity sweep, measuring in a browser against a real server, found that
**clearing the Year input on the GM's Set date control and submitting silently
moved the world to year 0** — 200, stored, no error. It reproduced in the legacy
V2 console AND in the v4 Bench date-verb row shipped by `C-CALV4-GAMEREADY`
(PR #588), both of which read the field with a `parseInt` fallback of 0. Every
other coordinate was range-checked; year alone was not.

**Year 0 is a legitimate year** on a fantasy calendar (this plugin says so in two
places, and a negative year is already driven end-to-end in its tests), so the
fix distinguishes ABSENT from ZERO rather than banning zero. Three places, all
in the calendar plugin: `PUT /api/v1/…/calendar/date` (`apiDate.Year` is now a
`*int`; an omitted year is refused — this was the server-side half no client fix
reaches, and it is live for the Foundry module), the world-state PUT (its five
date/time coordinates decode through `worldStateCoord`, which refuses a BLANK
coordinate by name), and both browser writers (`coordOrNull` — refuse and say
which box, never substitute). `UpdateCalendar` now range-checks the year against
the `INT` column, a storage bound and explicitly not a `year >= 1` floor.

Pinned by `internal/plugins/calendar/year_absent_test.go`, including two
**headless-Chromium** probes over the real rendered surfaces and the real shipped
JS. Full detail: `internal/plugins/calendar/.ai.md` § "An emptied Year field
moved the world to year zero".

### calendar-v4 round 2 CLOSED — the three parity slices, and the one number that did not move (2026-08-08)

**Read this before any other calendar-v4 entry.** R2-3 (`C-CALV4-THEATER`),
R2-4 (`C-CALV4-V2SUNSET`) and R2-5 (`C-CALV4-SKY`) are the reveal pass's last
three slices. Each has its own section below. This one exists because the three
share an end state that none of them states alone.

**ROUND 2 REMOVED NOTHING.** `internal/wire/routes_snapshot.txt` is
**byte-identical at 727 lines** across every commit of all three slices — no
route added, none removed, no migration, no file deleted. The V2 shell is still
registered, still served and still reachable by URL. R2-4 stopped the product
*linking* to it; that is a different claim from retiring it, and the difference
is the whole of `C-CALV4-SHELL-REMOVAL`.

**THE SHELL-REMOVAL GATE READS ONE OF FOUR.** The end state is signed by the
operator ([VS-1], 2026-08-07 — *"delete it, but build the replacements first"*);
what is outstanding is order, never outcome. Precise state, and the reason the
third box is the interesting one:

| Entry condition | State 2026-08-08 |
|---|---|
| every door swept by R2-4 | **MET.** Done, and held by `TestSunset_NoLiveDoorRemains`, which fails CI on a new one |
| R2-5 (`C-CALV4-SKY`) MERGED | **NOT MET — and not for a build reason.** The sky *shipped*, fix round included. The box says MERGED and PR #588 is open, so this reads NOT MET until it lands. Do not tick it from the branch |
| `C-CALV4-WEEKDAY-VIEWS` MERGED | **NOT MET, not started.** A prerequisite, not a booking — and R2-4 made it sharper, because `/calendars/:calId/week` and `/day` now 301 to a month |
| `C-CALV4-GM-CONSOLE` MERGED | **NOT MET, not started.** A rehousing job, not a backend one: `PUT /calendar/world-state` survives any sunset |

Arithmetic for that slice, pre-computed so it never regenerates until green:
**727 → 722**. State removals and additions separately — a net count hides a
swap. `.ai/todo.md` §0b holds the derivation and the traps.

**ONE SLICE STOPPED WITHOUT BUILDING, WAS RE-SIGNED, AND THEN SHIPPED.** R2-3's
first pass stopped at [TH-14] on a refuted premise, not on difficulty (below) —
sixteen of its seventeen rulings undelivered, the seventeenth (the B4 scope
glob) shipped alone. The coordinator **re-signed [TH-14] on 2026-08-08**,
replacing its third constraint against the measurement the stop produced, and
the second pass built the slice in five stages. **A slice that stops at a signed
STOP-AND-FLAG has done its job; a slice that improvises past one has not** — and
the record here is that stopping is what got the block corrected.

**WHAT A GM CAN DO NOW THAT THEY COULD NOT.** Reach the v4 calendar of a public
campaign while logged out; land on the Bench from every door in the product
instead of the legacy shell; read the sky — in-world time, moon phases and
tonight's celestial register — on the Bench's Primary Block; and **expand any
entity page's calendar embed into a full-tier surface over the page they are
on**, without navigating anywhere and without changing what the embed looks like
tomorrow. **What V2 still
solely owns:** the week view, the day view, and the GM world-state console. All
three are the gate above, and all three are why the shell is still standing.

### A real MariaDB was available the whole time (2026-08-08)

**The belief that this build cannot test against a database was false, and it
cost real coverage across at least four slices.**

`make docker-up` cannot work here — there is no Docker daemon, and that much is
true and was measured correctly many times. The error was the inference drawn
from it. **The MariaDB *server binary* is installed** (`/usr/sbin/mariadbd`,
10.11.14) and runs directly against a scratch datadir; Docker was never the only
way to get one. Two small things hid it, and both read as something else:

- `mariadbd` **refuses to start as root** unless given `--user=root`, aborting
  with *"Please consult the Knowledge Base to find out how to run mysqld as
  root!"* — which reads like a permissions wall rather than a missing flag.
- A **Unix socket path over ~107 characters** fails with a *truncated path* in
  the error message rather than a length complaint, so a socket placed in a long
  scratch directory looks like the server never started.

**MEASURED 2026-08-08.** Server up in ~3s; `SELECT VERSION()` returns
10.11.14-MariaDB; DDL and DML round-trip normally; and
`TestFreshDatabase_EveryPluginSchemaApplies` — which had only ever been reasoned
about or run once by a verifier — **RUNS AND PASSES in 1.41s against a real
schema migrated from zero.** So the fresh-install crash fix is proven against a
database, not against a fake.

**The recipe is now `tools/start-test-db.sh`** (start / `--stop` / `--clean`),
with `make test-db-up`, `make test-db-down` and `make test-int-local`. It listens
on **13306, never 3306**, so it cannot collide with or be mistaken for a dev
server, and it holds only disposable schemas.

**What this unblocks.** Every "proven against fakes, not the database" caveat in
the R3/R4 books is now a gap that can be closed rather than an environmental
limit: the notes export round trip, the import failure tally, the sync cursor
walk, the `calendar_active` cascade fix, and the RSVP flows. The integration
tests that have been skipping silently — they SKIP rather than FAIL when no
server answers, which is why nobody noticed — now run.

**The lesson worth keeping.** Every one of those "no database here" notes was
honest about what it had NOT proven, which is why this was recoverable at all.
But an environmental limit asserted once gets quoted forward by every later
slice without being re-measured, and this one was quoted for months. **Re-measure the environment when a limit is
load-bearing, not just the code.**

### The sky header — C-CALV4-SKY (R2-5), shipped 2026-08-08

The last slice of calendar-v4 round 2. The operator thawed the parked sky arc as
a **redesign, not a restoration**, and recorded a verdict on the old
implementation in three counts so the redesign could not inherit it. Four facts
from this build are worth carrying forward.

**1. The placeholder was reserving a seat on the wrong Block, and nothing could
have told us.** The dashed `.skyband` strip rendered only on the **real-world**
Block (`block.templ`'s `if d.IsRealWorld`), and that Block's Almanac is empty for
**two independent reasons**: the register's build gate named the Shelf alone
while the Bench builds that Block `noShelf`, and `CreateCalendar`'s real-life
path seeds no moons, no seasons and no eras. A sky there would have carried a
gradient and a clock and nothing else, forever. One leg would be a configuration
accident; two is a product fact. The sky seats on the **Primary** Block — one
sky per surface — and the placeholder is deleted from all three hosts.

**2. Its deletion was invisible to the entire battery.** A repo-wide grep for
the placeholder's copy and its function name across every `*_test.go` returned
**zero**. It could have been deleted, nothing shipped in its place, and every
suite stayed green. The guard is therefore **two-directional**: absence of the
retired class AND presence of the sky's own summary. A guard that only proves an
absence goes green on a slice that deleted the thing and shipped nothing.

**3. The one un-ruleable question was a collision between two operator
signatures.** The signed stills seat the band INSIDE the Block's box; the signed
disclosure register's clause 4 says the Block's interior stays still, enforced
twice, in two packages, mutation-tested. Every factual leg was measured and none
disagreed — **every candidate home was closed by a signed guard**. That is not a
gap in the measurement, and it is not a dev call. The operator amended clause 4
**by name, with one sentence**, and the monopoly guard gained a **per-class
exemption** rather than a relaxed rule: its forbidden-ancestor list is
byte-identical, so **every other rule in the product stays exactly as
constrained as it was**. The guard is renamed
`TestBenchCSS_TheNamedCarveOutsAreExactlyTwo` — a guard whose name still claims
a monopoly it lost is how the next hand learns the wrong law.

**4. The counts are numbers, measured against the SHIPPED element.** The closed
band is **40 / 40 / 32px** against a 44 / 44 / 36 budget, read twice
independently (the custom property that sizes it, and the laid-out box);
anchored-edge travel across a full open is **0.0px** for all three facts at all
three widths; the discs grow **13→40px** and **11→32px** with **33.8%** and
**32.8%** below the horizon; **3** controls. The drawing pass's own history is
why: five of its seven fix rounds were caught by measurement rather than by
looking, including a **65px** sideways slide of the clock that every property
being correctly declared could not have revealed.

**And one still cannot be reproduced under the guards, which is reported rather
than approximated.** The mock's open pane leads with *"Sunset 19:58"*. Chronicle
persists no daylight boundary — the importer parses `sunriseTime`/`sunsetTime`
from two foreign formats and **drops** them, and no column and no migration
exists — so the line states the day, the month and the register's own audited
arithmetic instead. Deriving a plausible 06:00/18:00 would be inventing world
data on a worldbuilding platform, which is the defect `WorldStateSun.Tint`
already refuses by shipping null.

### C-CALV4-SKY — the fix round, and the register that should have shipped with it (2026-08-08)

The slice was verified adversarially and the finding worth carrying is not any
one of the defects. It is that **the build checked itself against the
enumerated half of a ruling and called that the ruling.**

[SKY-5] says *"THE STILLS BIND THE RESULT"* and then lists what to measure:
band heights, the C1/C3 density switch, disc growth, a third of each disc below
the horizon, the seal's sweep, the trio, grayscale disc identity, reduced-motion
identity. **Every item of that list was met and probe-measured in a real
browser.** And the open pane still shipped with **two visible differences from
the signed still** that the list does not name — a muted sub-head line that was
absent, and a four-column per-moon row that had been collapsed into one merged
sentence. Both were reproducible under every guard: markup and static CSS, no
motion, no token, no transition. Neither was reportable instead of buildable.
**An enumeration inside a ruling is a floor, not the gate.**

**The second finding is that neither divergence was written down anywhere** —
not in nine commit messages, not in this file, not in `.ai/todo.md`, not in the
plugin's `.ai.md`. The mandate governing this arc says undisclosed divergence is
a FAIL, and it is right to: a difference visible only by diffing a PNG against a
template is a difference nobody finds. The sunset — the one divergence that
*was* disclosed — was disclosed **four times over**, and that asymmetry is the
tell. **What gets written down is what somebody decided was interesting, and the
things you didn't notice are exactly the things that need a register rather than
a decision.**

**Both are now built** (stage 10) and the register lives in
`internal/plugins/calendar/.ai.md` → *"The sky pane's DIVERGENCE REGISTER against
the signed stills"*. It carries the two closed items, the **two** standing
still-not-reproducible cases — the sunset **and now a second one**: the mock's
"in shadow" window reads an eclipse-node flag its demo rig invents, and
`calendar.Moon` has no node, no inclination and no ascending-node column, so
deriving one would be the sunrise defect wearing different words — the
deliberate subtractions, and the two shape deviations that were disclosed in
their commit bodies but never travelled into the handoff claim list
(`SkyGradient` being a gradient string rather than [SKY-6]'s "fraction", and
`helpers.go` being off the Files-you-own list because the named
`block_geometry.go` does not exist in that package).

**A disclosure that does not travel with the claim is half a disclosure.** Both
of those were reported honestly, in the right place, at the right time — and the
verifier still had to re-derive them from the diff, because the summary handed
forward did not carry them. The commit body is where a decision is *justified*;
it is not where the next reader *looks*.

### calendar-v4 R2-4 — C-CALV4-V2SUNSET: the legacy calendar stops showing up (2026-08-08)

**The operator's complaint was one sentence — "legacy calendar still shows up" —
and it was not a bug. It was what the product still linked to.** R2-4 re-pointed
about twenty live doors and six redirect targets at the Bench, stopped two labels
naming the version at the user, and moved `GET /apps/calendar` onto the
public-capable group so a logged-out visitor to a PUBLIC campaign reaches it
instead of `/login`.

**IT REMOVED NOTHING.** Zero routes, zero templates, zero JS assets, zero files,
no migration. `internal/wire/routes_snapshot.txt` is **byte-identical at 727
lines** across all four commits. The V2 shell is still registered, still served,
still reachable by URL — just not by click.

**THE SHELL'S DELETION IS SIGNED BY THE OPERATOR** ("delete it, but build the
replacements first", 2026-08-07) and belongs to **`C-CALV4-SHELL-REMOVAL`**,
behind four boxes: `C-CALV4-WEEKDAY-VIEWS` merged · `C-CALV4-GM-CONSOLE` merged ·
R2-5 merged · every door swept by R2-4 (done). **As of 2026-08-08 exactly ONE is
met** — the door sweep. R2-5 shipped but PR #588 is open, and that box says
MERGED, so it does not tick from the branch; the other two are unstarted. The
per-box state is tabulated in the round-2 close-out entry at the top of this
section. The pre-computed arithmetic for
that slice is **727 → 722**. See `.ai/todo.md` §0b and
`internal/plugins/calendar/.ai.md` for the deletion order and its traps.

**THE ROUTE MOVE IS INVISIBLE TO THE WIRE ORACLE, and that is the most important
sentence in the PR.** The snapshot records METHOD, PATH and defining file and
nothing about middleware, so a group change plus a guard swap leaves it
byte-identical beside an authorisation change. The oracle here is
`bench_anonymous_test.go` and `routes_test.go` — nineteen signed assertions,
twelve of them against a real MariaDB.

**ONE GATE MOVED FROM THE ROUTE INTO THE PRODUCER, because the route's Player
floor was load-bearing.** The Bench's RSVP panel prints every member's display
name, role and zone; measured before the guard was written, an anonymous render
carried two real member names. `benchRsvpResolve` now skips the roster below
`RolePlayer`, so the absence is in the payload rather than in a template branch.

**Eight tests inverted, none softened, none deleted.** Seven were predicted by
the dispatch; the eighth (`TestCreateCalendar_RealLifeRedirectsToV2`) pinned a
door the dispatch moves by name. **A stated feature loss rides with them:**
`/calendars/:calId/week` and `/day` now land on a month, because the only week
and day views in the product live inside the shell — that is what
`C-CALV4-WEEKDAY-VIEWS` exists to fix, and it is a PREREQUISITE of the removal.

### calendar-v4 R2-3 — C-CALV4-THEATER: stopped, re-signed, SHIPPED (2026-08-08)

**Read the first paragraph even if you only want to know what the feature does,
because the sequence is the point.** All seventeen coordinator rulings were
signed 2026-08-07. [TH-14] required the scaffold to sit outside every
`.cal-block-host`, inside a `cal-bench` root **and outside any HTMX-swappable
region**, and ruled that if no position satisfies all three, that is *"a host
restructure, not a placement, and not this slice's to take."* **Measured, the
condition was met** — `calendar_widget_type.go:152` wraps the ENTIRE output of
`EntityCalendarBlock` in `widgetbindings.BlockHost`, and `picker.templ` targets
that wrapper with `hx-swap="outerHTML"` on three live Scribe+ paths, so the
swappable region is the whole component, one level above anything the slice can
emit; [TH-14]'s own measurement cited the picker's inline `innerHTML` slot.
**The first pass stopped and flagged.** The coordinator re-signed [TH-14] on
2026-08-08, **replacing constraint 3**, and the second pass built the slice.

**WHAT THE RE-SIGN RULED.** The scaffold MAY sit inside the swappable region:
opener and scaffold share that subtree, so they die and revive together and
[TH-12]'s `htmx:afterSettle` re-init rewires both — there is no state in which a
live opener points at a dead scaffold. **Its required counterpart shipped with
it:** an `htmx:beforeSwap` listener that closes the theater immediately when the
swap target contains it, because a `<dialog>` removed from the DOM mid-modal
strands the top layer, keeps the document scroll-locked and drops focus on a
detached node. Constraints 1 and 2 unchanged.

**WHAT THE FEATURE IS.** An `Expand` button beside the embed's header anchor
opens a top-layer `<dialog>` carrying a **second render of the same
projection**, seeded with the Bench's five layer keys. The embed behind it is
byte-unchanged and stays glanceable. **One calendar shown two ways** — the depth
[BR2-8] removed comes back in the theater rather than in the embed, which is the
slice's thesis rather than a side effect.

**THE THREE FACTS A LATER SLICE NEEDS.**

1. **There are two things called "full tier."** The CSS tier is a container
   query and widening a box buys all of it; the ZONE SET is the producer's and
   widening buys none of it. A wide overlay around the embed's Block would be a
   *stretched glanceable month*, not a full-tier Block.
2. **One projection, two renders.** `Layers` is assigned post-hoc
   (`entity_calendar_block.go`), and the host passes neither `LedgerHidden` nor
   `ShelfHidden`, so the projection the page already holds contains every mark
   the theater prints. The theater's Block is a struct COPY — zero extra service
   work — and the no-wider law is pinned STRUCTURALLY by a reflect assertion
   over every exported field of `BlockData` rather than by a mark-set oracle,
   which would have compared a struct to itself and passed forever.
3. **The DOM re-namespace is core, not polish.** Every id and radio-group name
   the Block emits is a pure function of `(CalendarSlug, Viewer.HostEntity)`, so
   two Blocks for one calendar emit identical ones and the theater's tie toggle
   would be visibly dead while pressing it re-inked the embed behind the
   backdrop. The copy sets `HostEntity` to a distinct token AFTER the projection
   has run, so no widget file is opened.

**TWO EXISTING TESTS WERE RE-SCOPED RATHER THAN SOFTENED**, and it is the same
move both times: `TestEntityCalendarBlock_HostLayerSet` and
`TestLedgerGeometry_WithoutLedgerThereIsNoLedgerDOMToLeaveAVoid` asserted
whole-page that no `data-zone` rendered. That claim was always about the EMBED;
the page now carries two Blocks. Each negative is now scoped to the embed's
subtree **and paired with the positive in the theater's** — absent here, present
there — which is strictly stronger than the single negative it replaces.

**`/schedule` WAS NOT THE PRECEDENT [TH-3] WAS DRAFTED ON**, and this is worth
carrying because it nearly shipped a silent nothing. It carries
`class="cal-bench cal-schedule"` and links the sheet, but it has no `<details>`,
no `.disc` and no `::details-content`, and its own guard BANS `--disc-open`
there. It inherits the register's TOKENS and consumes zero of its RULES. Copying
it faithfully would have given the theater three duration tokens and no motion
at all — `calendar-theater.css` is forbidden to declare a transition — with
every guard in the tree green. The real precedent is the DAY CARD, and the
theater's two rules sit by name inside the ONE register section beside it.

**NUMBERS.** `internal/wire/routes_snapshot.txt` **unchanged at 727 lines**; no
migration; `internal/widgets/calendar_block/**` untouched; `internal/app/routes.go`
**one added line** (the `pluginBodyScripts` registry entry — [TH-12], because
`tools/page-script-allowlist.txt` pins the templ at exactly one page-side script
and the ratchet fails above as well as below). The theater's total horizontal
inset around `.cal-block-host` is **48px** against [TH-1]'s 60px budget, giving
**976px** at a 1024 viewport and **1156px** at the 1180 cap — both over the 900px
full-tier floor, and both computed by a test from the sheet's own numbers rather
than asserted. `static/css/calendar-bench.css` is **96,811 bytes** and is now
linked on a third page.

**THE §4 COST MEASUREMENT IS A FINDING, NOT A SHRUG.** [TH-2] says the
wall-clock delta of `EntityCalendarBlock` "should be at the noise floor, and if
it is not, that is a finding." **It is not.** On the ten-day fixture the entity
page goes **19,834 → 47,339 bytes (+138.7%)** and the build+render goes
**510 µs → 1,399 µs (+174%)**. The attribution, stated so it can be judged
rather than taken: **this is TEMPLATE-RENDER cost, not service cost.**
`spine.Block` is called **once** (pinned at the source by
`TestEntityCalendarBlock_OneProjectionAndNoSecondViewerFilter`, which also
asserts no second viewer-filter call site and no new role middleware), and the
cost tracks BYTES at a flat rate — 25.7 µs/KB before, 29.6 µs/KB after — because
the theater's Block renders a Ledger and a Shelf the embed does not. The byte
delta is the slice's biggest single cost and is the price of Option A's
zero-route, zero-fetch, instant-open story; the lever, if it is judged too high,
is Option B (a Block-fragment route) and that needs a signed route.

**NO SCREENSHOTS.** §14's visual gate could not be executed in a headless
container — the repo's own browser probe was already failing at Step 0. Every
geometry number above is derived from the sheet BY A TEST rather than
photographed, which is the strongest thing available and is not the same thing
as a still. The visual, mobile, player and keyboard-trace set is itemised in
`.ai/todo.md` §0d.

**Split out and booked:** `C-CALV4-THEATER-DAYCARD` (the day card inside the
theater — it would be the first surface with two Ledgers on one page, which is
where `C-CALV4-CARD-CROSSBLOCK-LEDGER` lives) and `C-CALV4-DASH-BLOCK-V4` (the
campaign dashboard's calendar block is still the V1 embed, so there is no v4
Block there to expand). `C-CALV4-DAYPICK-A11Y` **stays open**: the theater's
five-key seed makes days focusable INSIDE itself, which is a genuine and
unplanned accessibility improvement, but the embed behind it still has the gap.

### calendar-v4 — C-CALV4-GAMEREADY CLOSED: the calendar survives a real session (2026-08-08, stage 23)

**The playability slice is done.** It existed for one reason: the operator
starts a real tabletop game in under two weeks, and a five-lane readiness audit
measured 33 findings that hit the table. **This bought PLAYABILITY, not
polish**, and it is closed at twenty-three stages on
`claude/coordinator-handoff-stage-3-3d3s4w` (PR #588), base `c573a9cc`.

**The bill, in one line each:**

| § | what the GM could not do before | state |
|---|---|---|
| §1 | See any month but the one the campaign was standing in | **shipped** ([GR-1]/[GR-2]) |
| §1b | Author an event into a month other than the rendered one | **[GR-3] BLOCKED AS SIGNED** — reported, not improvised |
| §2 | Advance the in-world date without opening a page being deleted | **shipped** ([GR-SIGN-A]/[GR-4]) |
| §3 | See a multi-day event on any day but its first | **shipped** ([GR-5]) |
| §4 | Turn on "Collect RSVPs" at all from calendar-v4 | **shipped** ([GR-6]/[GR-10]) |
| §5 | Four RSVP paths that dead-ended players and silenced the GM | **shipped** ([GR-7]/[GR-8]/[GR-9]) |
| §6 | Repeat a festival yearly; the API said 201 to junk | **shipped** ([GR-11]/[GR-12]) |
| §7 | Survive one mis-tap beside Save without destroying an event | **shipped** ([GR-13]) |
| §8 | Follow two links that had never resolved, shipped to users | **shipped** ([GR-14]/[GR-15]/[GR-16]) |
| §9 | Edit the month list without silently re-dating the year | **shipped** ([GR-17]/[GR-18]) |
| §10 | (the anonymous visibility bypass) | **VERIFIED already fixed — not re-fixed** ([GR-19]) |
| lane 3 | Use any of it on the phone they will be holding | **shipped** (C-CALV4-MOBILE, 12 blocks) |

**THE ONE LESSON WORTH CARRYING OUT OF THIS SLICE: the project spent months
believing it had no database, and the fakes lied.** `make test-db-up` runs
MariaDB 10.11 on 13306 without Docker. Twelve DB-backed test functions now ride
in this plugin, and **three findings were green against a fake and dead against
a database**:

- **[GR-11]** — `yearly` expanded perfectly in memory while three repository
  queries carried a hand-typed `recurrence_type IN (…)` that never loaded the
  row. Shipped, the feature would have been **completely inert**. The clause is
  now derived from the constant block.
- **[GR-5]** — `ListEventsForMonth` selected on the *stored* month, so a
  festival crossing a month boundary was never LOADED while the second month
  rendered. The in-month fix alone would have left the identical lie for the
  commonest festival shape there is.
- **[GR-17]** — the §9 finding was `[READ]`, not measured. The ruling was
  *reproduce first*. It reproduced on a real MariaDB: inserting an intercalary
  month at position 5 moved **3 of 4** events a month later and stranded
  **zero** — which is why the shipped warning is a **before/after comparison**
  and not the `month > len(months)` bounds check that would have reported 0 and
  certified the damage.

**What was NOT built, and why that is the correct outcome:**

- **[GR-3]** — the editor's cross-month roll. **BLOCKED AS SIGNED and flagged
  rather than improvised**, on two measurements: the day-card payload carries
  no month list or `MonthDays`, and the day-key namespace is
  `slug + "-" + day` — **not month-qualified** — and is pinned in both
  directions. Day 1 of next month mints the same key as day 1 of this month, so
  a date outside the rendered month **is not addressable at all**. Both fixes
  live inside `internal/widgets/calendar_block/`, which this slice's Bounds
  close (`data.go` byte-pinned r54). §1's first half already closes the
  START-date half. Full measurements in `.ai/todo.md` §0a.
- **[MOB-S1]** — the one `[COORDINATOR TO SIGN]` block. **Reported UNSIGNED and
  shipped as answer A (nothing changes).** It is the operator's, and the next
  hand must not answer it either.

**Bounds held, and they are checkable:** `internal/wire/routes_snapshot.txt` is
**727 lines and byte-identical** to base — **zero new routes** across
twenty-three stages. `internal/widgets/calendar_block/data.go` byte-identical.
Zero migrations, zero data writes, zero new page scripts
(`check-page-scripts.sh` green *unedited*, 15 files remaining),
`check-calendar-v4-lints.sh` green unedited, the ONE motion register in
`calendar-bench.css` unchanged.

**Correction landed with these books:** `.ai/decisions.md` asserted in three
places that *"there is no ADR-049"*. That rule always meant *calendar-v4 does
not fork an ADR of its own*, and it stayed true — but the **number** was
legitimately claimed by C-SWEEP-R4 stage 9 on 2026-08-07 for an unrelated
subject, and ADR-049 is live at `.ai/decisions.md:3901`. The flat wording cost
a GAMEREADY stage a false STOP-AND-FLAG against a citation that was correct.
All three now say what they mean. (Two historical entries below, at the 2026-07
dates, still carry the old flat phrasing in their own session context; they are
left as written because this file's later sections are a dated record.)

Books: `.ai/todo.md` §0a · `internal/plugins/calendar/.ai.md`
§"C-CALV4-GAMEREADY" · `reports/chronicle/2026-08-08-C-CALV4-GAMEREADY.md`.

### calendar-v4 — C-CALV4-MOBILE SHIPPED: the phone the GM will actually be holding (2026-08-08)

**GAMEREADY's lane 3, the only one of five the readiness audit returned
NOT-READY.** CSS plus one JS module: no markup restructuring, no template
branch, no route, no migration, no producer field, no new page script.

The founding measurement, and the one to remember: at **390x664**, on the
calendar page, `.cal-block-host .lrows` — the list of what is actually
happening — was **41 pixels tall against 220 of content, with ZERO of its five
rows fully visible**. At 360 it was 24. A second Block sheared its empty-state
sentence mid-word. It is now **132 of 220 with three rows fully inside, at 390
and 375 and 360**, and the desktop tier is byte-unchanged (209/240, 4 of 5,
min-height 176).

Seven other things a table would have hit:

- **Save was under the keyboard.** The editor sheet carried an inline
  `top: 106px` written once at open time; shrink the viewport to 390x380 as a
  software keyboard does and the box did not move — Save at `y[426..456]`
  against a 380px viewport, in a `position: fixed` box no gesture can scroll.
  The geometry now lives in CSS and re-resolves itself; Save measures inside the
  viewport in nine arms (three widths x open/keyboard/rotation).
- **The page scrolled out from under an open sheet** in all six measured arms.
  It is locked now by the `position: fixed` form with the offset stored, and —
  ruled harder than the lock — **released on all five exit paths and on the
  card→editor→close handover**, restoring the scroll to the pixel.
- **Five independently-scrolling regions on one phone page**, none declaring
  `overscroll-behavior`. Two now, both contained.
- **Days a GM could not reach.** At a 20-day week, 624px of grid inside a 364px
  `overflow: hidden` box. The grid's own wrapper scrolls the inline axis now;
  a tenday still grows no scroller, and the page fold is re-proven in 24
  measurements.
- **The operator's own gate panel needed 182px of sideways drag** at 390 and
  202px at 360. Zero now, both arms, CSS-only, `bench.templ` byte-unchanged.
- **`/schedule` was tuned to exactly one phone** — 0px of drag at 390, 7px at
  375, 22px at 360. Re-derived fluidly against 360; the budget guard now runs at
  both ends instead of one.
- **The tap floor was 24px against a 44px standard.** A short named list of the
  controls a person hits under time pressure now measures >= 44 in the block
  axis at <= 640, with the desktop measurement proven identical before and after.

**Evidence discipline, and it is the reason the Ledger survived to a live
build:** `benchShotPage` was missing the `.bsurf` wrapper the product emits, so
every one of the three signed ≤640 ordering rules matched nothing and the entire
phone evidence set was of a layout production does not render. That was fixed
FIRST, before any artefact. Every phone number in this lane is measured in a
**NESTED BROWSING CONTEXT (an iframe)**, stated on the face of every probe,
because headless Chromium here clamps its window to ~500px.

**Six browser probes registered in `tools/check-browser-probes.sh`**, none of
them env-gated, so a machine that can drive a browser must run them.

**Still open, and it is the operator's:** `[MOB-S1]` — whether a player's RSVP
control should precede the calendar at ≤640 — is UNSIGNED and shipped as
"nothing changes". And **`C-CALV4-MOBILE-SHELL`**: the app shell at phone width
is unmeasured by anyone, and matters more now that the Block is tall.

### calendar-v4 — C-CALV4-GAMEREADY §4 and §5 SHIPPED: the operator can arm their own gate, and the RSVP flow stopped dead-ending players (2026-08-08)

**The operator's stated go/no-go for starting their game, plus the four measured
dead ends in the flow that gate depends on.** The engine underneath the RSVP
system is genuinely good — the audit could not break its security or its
arithmetic — and every fix here is at an EDGE, where it talks to a human.

**§4. "Collect RSVPs" existed in exactly one place: the legacy V2 event drawer**,
a committed deletion, wired by a script only `calendar_v2.templ` loads.
`daycard.templ` had ZERO occurrences of "rsvp". Every downstream RSVP surface —
the Bench session tile and `/schedule` — is gated on the flag it sets, so a
campaign that could not reach the switch got a player-facing panel saying "You:
no answer", three paragraphs explaining the options, and no buttons. The control
now ships in the v4 day-card editor at the route's OWN `RoleScribe` floor,
writing the already-shipped `PUT …/rsvp-collection`. Zero new routes, zero new
handlers, zero new service methods, zero stylesheet.

**§5, and the reason to read this entry if you touch the RSVP flow:**

1. **The suggest token was consumed by a submission that was REFUSED.** A
   partially-filled form — the shape the page invites — spent the link and then
   rejected the write, so correcting it answered "this RSVP link is invalid or
   has expired" and so did re-opening the email. One incomplete form permanently
   destroyed a player's only way in. **`TestToken_EmptySuggestionRejected` had
   asserted the refusal and never asked what the refusal cost**, which is how a
   green suite shipped it. That is the pattern worth carrying: a guard that
   pins the error and not the side effect is a guard with a hole in it.
2. **A spent link said "RSVP Failed" over an answer that WAS recorded**, on a
   page containing no `<a>` at all. Players told GMs the system was broken and
   GMs believed them. `GetUserRSVP` had shipped on the repository, unused by
   that path, the whole time.
3. **The emailed "Out this week" notified NOBODY** while the in-app twin always
   did — the one decline most likely to cancel a session was the only answer the
   Director never heard, and it was being written as `no` all along, so the
   tally moved under them in silence. **The two surfaces were pinned in
   different places; in fact the in-app half was pinned NOWHERE.** They are one
   test now.
4. **Arming with no SMTP said "the party has been invited" over zero sent mail.**
   The honest sentence already existed as `mailNotConfiguredLine` and was
   already used in three other places; the invite moment was the one that
   skipped it.

**Two operator instructions that are load-bearing until their bookings land:**
run ONE collecting event at a time (`benchRsvpPickSession` resolves exactly
one), and create ONE NON-RECURRING event per session (the RSVP table is
`UNIQUE (event_id, user_id)` with no occurrence column, so a repeating session
shares one set of answers across every occurrence — the control now says so).

Guarded by `rsvp_deadends_int_test.go` (**four real-database tests** — every §5
claim is a claim about a ROW, and a mock can only report that a method was
called), `rsvp_collect_control_test.go`, the authorised amendment to
`TestToken_EmptySuggestionRejected`, and `test/js/daycard_rsvp_collect.test.mjs`.

### calendar-v4 — C-CALV4-GAMEREADY §3 and §6 SHIPPED: the calendar stopped lying about what is happening today, and a festival can finally repeat (2026-08-08)

**Two more table blockers closed, and BOTH of them had a second half that only a
real database could see.** That is the sentence to carry out of this entry: the
project believed for months it had no MariaDB available, and the recurrence
engine has been proven against fakes for its entire life. Both halves below were
perfectly green against fakes and completely dead against a database.

**§3 — a five-day festival marked ONE cell.** `blockMarksForDate`'s only
membership test was `OccursOn`, which matches the stored date and the recurrence
rule and nothing else, so days 2..N of a span carried no mark. The day card is
built straight off those marks, so a GM clicking day three of a siege the party
was standing in read **"No events on this day"** — a positive false statement,
not an omission. `blockEventSpansDate` now matches inside the stored
`[start, end]` window, **inside the visibility-filtered loop** so a `dm_only`
span is absent on all of its days rather than only its first. `model.go`'s
comment promising that *"the ribbon layer renders their span"* was FALSE in the
code that made it — V2 had a ribbon layer, v4 never built one — and is corrected
in the same commit. **The ribbon itself is refused and booked**
(`C-CALV4-SPAN-RIBBON`): five identical chips are visually inferior to one bar
and operationally identical, and the mark is the half that stops the lie.

**§3's second half, found by the database:** `ListEventsForMonth` selects on
`e.year = ? AND e.month = ?`, so a festival running from day 28 of one month to
day 3 of the next was **never loaded** while the second month rendered. The
projection was right and the row never arrived. The month query now carries a
composite-overlap clause (radix 10000, because a user-authored month list can
exceed 99 days and the house `month * 100 + day` idiom cannot).

**§6 — a festival, holy day or birthday could not repeat, and the API said 201
anyway.** `OccursOn` expanded weekly/biweekly/monthly/custom and sent everything
else to `default: return onBase`. It was a REGRESSION — the pre-v4 calendar was
yearly-ONLY, and `.ai/data-model.md` still documented the column as
*"yearly, monthly"*. `RecurrenceYearly` now expands on the same month and day
with the interval applied. **A base day absent from a later year is SKIPPED,
never clamped:** a festival that silently moves to a different day is worse at
the table than one that does not appear, because the GM plans around the date
they authored and an absent occurrence is visibly absent.

**§6's second half, also found by the database:** THREE repository queries
carried a hand-typed `recurrence_type IN ('weekly','biweekly','monthly',
'custom')`. With the engine fixed and every fake green, every yearly festival
was still invisible in every month but its own. The clause is now DERIVED from
the constant block, so adding a recurrence type is one line in one place.

**And the silent 201 is closed at both doors.** `CreateEventAPI` and
`UpdateEventAPI` stored `"daily"`, `"hourly"`, `"WEEKLY"` and `"🐉"` with a 201
and no validation. Both now 400 — **exact and case-sensitive, reject never
coerce** — through ONE shared predicate, because two accepted sets in two places
is how they diverge. It is handler work by house rule (input validation on a
bound field) and deliberately gets no service-level twin. On `UpdateEventAPI`
the guard reads `patch.Field.Get()`, so an ABSENT key still preserves and an
explicit null still clears: neither is a value, so neither is rejected.

**Measured and reported rather than designed:** a recurring event that is ALSO
multi-day. [GR-5] names it a STOP-AND-FLAG. A weekly three-day rite based on
days 5–7 marks 5, 6, 7, 12, 19, 26 — the window is matched once at the base
dates while the recurrence expands only the base day. Pinned by
`TestBlockMarks_RecurringSpanIsMEASUREDNotDesigned` and booked as
`C-CALV4-SPAN-RECURRENCE`.

Bounds held and measured: `routes_snapshot.txt` still **727** lines, zero
migrations, `internal/widgets/calendar_block/**` and every `calendar_v2*` file
byte-unchanged, `check-page-scripts.sh` and `check-calendar-v4-lints.sh` green
unedited, the ONE motion register untouched, `golangci-lint` 0.

### calendar-v4 — C-CALV4-GAMEREADY §1 and §2 SHIPPED: the Bench can leave today, and the GM can move the date (2026-08-08)

**Two blockers a five-lane readiness audit measured as hitting the table**, both
closed on the branch that carries the R4 sweep.

**§1 — the Bench could only ever render the calendar's CURRENT in-world month.**
`benchBlock` built its `BlockRequest` with no `View` field, so `resolveView` — a
function written FOR navigation in wave 1, whose own comment worries in prose
about *"losing a navigated year"* — **had never once been called with one.** A
GM preparing a session could not look at next month. The cursor is now `?y=&m=`
on the existing route: shareable, refreshable, back-buttonable, and needing no
store, no migration and no fifth `benchSectionKeys` entry. **No clamping logic
shipped** — `resolveView` already had it. The control is three `<a>` elements
computed server-side against the calendar's own month list, so no page script,
no stylesheet and no breakpoint number were added. The cursor applies to the
**primary Block only**: `?y=1524&m=3` is a coordinate in the in-world month
list and would have parked the Gregorian Block beside it in March 1524.

**§2 — advancing the in-world date was V2-only**, on the page
`C-CALV4-V2SUNSET` [VS-1] has committed to deleting. `+1 day` / `−1 day` now
seat on the same nameplate row, at the **existing** `CanControlWorldState` gate,
through the **existing** `PUT /campaigns/:id/calendar/world-state`. **Set date
renders only at the stricter, existing Owner-only floor** — so a co-DM steps and
does not set, which is [GR-SIGN-A](b)'s signed, deliberate asymmetry and not an
oversight. Permission is absence throughout. Only two verbs: the other four V2
verbs move the **clock**, and v4 has no clock, so they would change a quantity
the surface does not display.

**THE PROJECT HAD A DATABASE ALL ALONG, AND THIS SLICE USED IT.** Three
real-MariaDB tests ship here (`make test-db-up`, scratch schema per test, core +
plugin migrations replayed): the cursor's month is what `candidateEvents`
actually READS, and the verbs' whole value is that the STORED date afterwards is
the one the GM meant — including the month rollover, which is entirely the
server's arithmetic because the client sends only `{advance:{days:±1}}`.

**One measurement worth carrying:** a form-encoded `PUT` to the world-state
endpoint binds **nothing**. `putWorldStateBody`'s `advance` and `time` are
tagless pointer-to-struct members, which Echo's form binder skips outright, so a
plain `<form>` or a bare `hx-put` would answer 200 having changed no date. That
is why the verb row is driven from the plugin's already-registry-mounted
`calendar_daycard.js` rather than declaratively, and
`TestWorldStatePut_FormEncodedBindsNothing` pins it so nobody re-derives it.

**[GR-3] IS FLAGGED, NOT BUILT.** The editor's cross-month roll cannot be built
as signed: the ruling assumes the day-card payload carries the calendar's month
structure (it does not) and the day-key namespace is `slug + "-" + day` —
**not month-qualified** — so a date outside the rendered month is not
addressable in the key space the picker, `edDateFor`, `edUI.endKey` and guard B4
all share. Both fixes live inside `internal/widgets/calendar_block/`, which this
slice's Bounds close. Full measurements and what it would take are in
`.ai/todo.md` §0a.

Bounds held and measured, not promised: `routes_snapshot.txt` byte-identical at
**727** lines, zero migrations, `internal/widgets/calendar_block/**` and every
`calendar_v2*` file byte-unchanged, `static/css/**` byte-unchanged,
`check-page-scripts.sh` and `check-calendar-v4-lints.sh` green **unedited**, the
`weather` grep over the widget package and `bench.go` still empty, and
`golangci-lint` 0.

### calendar-v4 — R2-2b SHIPPED: the editor earns its chrome (2026-08-02)

**The operator's complaint, closed at last.** 2026-07-29, on the live client:
*"I'm unable to click and do anything… It should be a small card with a nice
unfolding animation, and then a bigger editor that forms via some animation."*
DAYCARD answered the first two thirds and gave one honest reason for the rest —
[DC-7] refused to invent a motion signature and escalated it. The operator
signed the carve-out on 2026-08-01, and `C-CALV4-EDITOR-R2b` is the answer to
the last four words.

**Three things and no fourth.** THE CHROME: the stage-2 mechanism keeps every
`data-de-*` handle and every request body while its controls are replaced — the
locked (hue · pattern · glyph) type rail, a real month grid whose week length is
DERIVED from the payload's own weekday names, the intercalary day as a full-width
row and a real date, the recurrence block, the visibility cards as real radios
with an Owner-only allow/deny roster, the tie pill with an entity picker, and a
live preview. THE MORPH: geometric, ONE named class, four properties
(`inline-size` · `block-size` · `translate` · `opacity`), the register's existing
tokens, no new duration, no scale, declared inside the ONE register section.
DRAG-CREATE: landed rather than severed, on all seven [DC-11] terms.

**THE RECURRENCE UNIT LIST WAS WRONG IN THREE DIRECTIONS AND ALL THREE ARE
CORRECTED.** The week unit is NOT invention and its chip came off — week-based
recurrence strides `WeekLength() × recurrenceWeeks(...)`, so `weekly` MEANS
every tenday on a ten-day calendar, and DAYCARD §5 and the mockup are both
wrong. `year` IS invention, the drawing offers it UNCHIPPED, and it does not
ship at all. And new to this slice: `every N months` degrades exactly the same
way, because `OccursOn`'s monthly branch ignores `RecurrenceInterval` entirely —
so that control is ABSENT for the month unit rather than chipped. There is
nothing there for a backend to add.

**[ER-5] WAS MEASURED AND THE MEASUREMENT DISAGREED WITH THE PREDICTION.** The
ruling expected a wide editor to re-create DAYCARD's occlusion blockers. It does
not: 61 day cells × 11 viewport widths × 6 candidate box widths, Ledger stacked
and docked, and **every** candidate holds 0 px² of overlap. What moves with width
is how often the popover falls to the signed desktop SHEET. The shipped 760px is
the largest that never sheets at or above the editor's own two-column
breakpoint, and the divergence from the drawing's ~1008px is arithmetic: a
1008px box cannot sit beside a 300px docked Ledger inside an 1180px measure.
`placeCard` was not re-opened.

**AND THE MEASUREMENT WENT STALE TWO STAGES LATER — the fix round's first
finding.** It was taken before the morph existed and not re-run. From the morph
stage the same probe FAILED at every width, up to 70,906 px² over a docked
Ledger, `clear=true`. Cause: `edClose` writes the reverse morph geometry as an
INLINE `inline-size` and `edHide` — the only thing that clears it — runs on a
timer `edShow` cancels, so a reopen inside 160ms measured the CARD's width for a
box the sheet sizes at 760. `edShow` now clears it before the box is measured;
`placeCard` is still untouched. The probe's own accounting was separated at the
same time (popover overlap gates; the signed desktop sheet is recorded; the
CROSS-BLOCK case is reported with a card control arm and booked as
`C-CALV4-CARD-CROSSBLOCK-LEDGER`). **A measurement is scoped to the commit it
was taken on.**

**THE FIX ROUND'S OTHER FIVE, IN ONE PARAGRAPH EACH, because every one of them
was GREEN and wrong rather than red and obvious.** (1) The greyscale proofs
declared `html{filter:grayscale(1)}` and the card and editor are `[popover]` —
top-layer, not painted as descendants of `<html>` — so the filter never reached
the surface under test; measured at 0.39% of editor pixels above chroma 20 with
a maximum of 255 while the caption said hue was removed. (2) The editor's
primary action shipped at **1.0:1**: `.cal-dayeditor .btn.fill` set `color:
var(--accent)` at the same specificity as the Bench's `background:
var(--accent)`, with the day card's sheet linked second, so `Create event` was
painted in its own fill. (3) The capture fixture seeded NO event categories, so
every shot photographed the module's `No type` fallback while its caption
claimed "the locked type rail" — and the producer was emitting no pattern
channel at all, making the type rail's three locked channels two. (4)
`TestDayCardCSS_EverySelectorIsScoped`'s comment claimed a literal check over a
`strings.Contains` prefix test; `.cal-daycard-drag` passed by accident and so
would a fourth root. (5) The carve-out monopoly guard iterated a hardcoded
eight-file list while claiming "product-wide"; adding `.edmorph` to
`gm_panel.js` left the whole package green. **Every one of them is now a
mechanical check with a named mutation that turns it red.**

**A GUARD THAT COULD NOT SEE ITS OWN SUBJECT.** The morph's close must remove
`.dcopen` before writing the reverse geometry or leaving takes as long as
arriving. Mutating that order left every end-state assertion green — it is an
ordering claim inside one task. The fixture's DOM stub grew an operation log and
the ordering is now a real assertion. A guard nudged until green stops proving
anything; a guard that was never able to see its subject never proved anything
at all.

**THE SECOND FIX ROUND WAS ENTIRELY ABOUT THE EVIDENCE, AND IT TOUCHED NO
PRODUCT CODE.** Three blockers, all of the same family: a picture and the
sentence beside it were not the same claim. (1) `.ed-body` is
`max-block-size: min(70vh, 620px); overflow-y:auto`, so at 1440 the two-column
body FOLDS and the ◈ Restricted card, the allow/deny roster and `Tied to` fell
below it — four captions and two index rows named marks their frames did not
contain. Desktop shots are now upper/lower PAIRS, the lower half scrolled as a
wheel would scroll it with no geometry overridden, and every shot burns a fold
report saying what is VISIBLE IN THIS FRAME. (2) Every shot ran `+ New event`
while **21 of the 22 gating stills are EDIT mode** — and the cause was one layer
back: the shared Bench fixture projects with NO events, so no card ever had a
row to Edit and create mode was the only editor the rig could reach. A
capture-only evented fixture and a stubbed single GET fixed it; nine edit-mode
shots followed, including the Delete axis, which create mode cannot make because
a draft has no id to delete. (3) `daycard_test.go` handed the 24px floor and the
horizontal fold to "the screenshot gate" and the gate carried two CAPTION
STRINGS; `daycard_floors_probe_test.go` now MEASURES both over 12 runs, and the
drag row is captured rather than described.

**THE 390px VIEWPORT WAS NEVER REAL, IN ANY ARTEFACT, UNTIL NOW.** Headless
Chromium here **clamps the window to a 500px minimum width** — measured, in old
headless and `--headless=new` alike. Three files were named `-390x844` and were
500px wide, and two of the three did not say so, so the fold had never been
checked at 390 in any form. A nested browsing context has its own viewport;
both the probe and the shot rig use one, the probe asserts the width it actually
measured in, and the shot burns the substitution onto the image. **A later hand
should not have to rediscover this.**

**THE MORPH DID NOT RUN, AND THE EVIDENCE'S DIAGNOSIS OF WHY IT COULD NOT BE
PHOTOGRAPHED WAS ITSELF THE DEFECT (R2b stages 18-19).** The signed carve-out
shipped INERT IN THE OPEN DIRECTION: measured frame by frame in real Chromium,
the editor's box was at its resting geometry, translate 0, opacity 1 from the
first sample after the door was clicked through 434ms. It popped in at full
size and animated only on the way out. Cause: `edOpen` wrote the seeded start
geometry and added `.edmorph` in the SAME style recalc, and CSS Transitions
start from the AFTER-change style — so that one recalc started transitions
running AWAY from the resting box toward the seed, the seed never became the
settled before-change style, and the final write saw nothing to change.
`edClose` always worked because it adds the class and flushes BEFORE any value
change. **The fix is one flush** (seed → flush → class → flush → final write)
and it is browser-general.

**Three lessons the books should keep, because each one cost a round.**

 1. **EVERY MORPH GUARD ASSERTED THE STATE MACHINE AND NONE THE RENDERED
    RESULT** — a DOM-stub test reading end-state inline styles, a
    MutationObserver over the style attribute whose own header conceded it
    "DOES NOT PROVE that the compositor interpolated", and two CSS guards that
    read the sheet. All four stayed green with the morph completely dead.
    `TestDayCardMorphInterpolates` is the missing one: real Chromium, real
    timeline, `getBoundingClientRect` + `getComputedStyle` sampled every
    animation frame in BOTH directions, and it is NOT env-gated.
 2. **A RIG'S LIMIT IS A CLAIM, AND CLAIMS GET CHECKED.** "This environment
    cannot photograph the morph" was a true measurement of
    `--virtual-time-budget` (which never runs the rendering lifecycle) promoted
    into a false statement about the environment. Serve the page, hold the
    `load` event with a slow subresource, freeze `setTimeout` at the click, and
    the same Chromium photographs the flight in both directions.
 3. **A DEAD ANIMATION HIDES THE BUGS DOWNSTREAM OF IT.** The moment the morph
    ran, three synchronous rigs turned out to have been measuring the box
    mid-flight, and the [ER-5] probe's own stale-geometry guard then caught a
    PRE-EXISTING placement defect: `applyPlacement` writes `.dcsheet` and its
    `style.width` AFTER `edPosition` measures, and nothing cleared them, so a
    reopen after a sheeted placement handed the placement law the viewport
    width for a box about to render at `--de-w`. `edShow` now clears both.

**`C-CALV4-CARD-REDUCED-ANCHOR` is closed (sweep R3, 2026-08-07).** R2b measured
it at stage 19 and booked it, because [ER-5] made `placeCard` a STOP-AND-FLAG and
[ER-6]/[ER-7] bound the slice to the morph's ordering. The fix needed neither:
`edOpen` freezes the anchor's rect one line after the morph's own `fromRect` —
BEFORE `closeCard()` runs `hide()` synchronously on the reduced-motion branch —
and hands `edPosition` that frozen rect through the same one-shot
`{getBoundingClientRect}` shim the drag-create path already used. Both motion
modes now land on the same placement, measured in real Chromium by
`TestDayCardReducedMotionAnchorsToItsDay`; shot 13's caption, which disclosed the
defect, is corrected in the same commit.

**Carried, not closed:** `C-CALV4-DAYMENU` (the 10 `menus-*` stills travel with
it — a 22-of-32 fidelity split, never a shortfall); `C-CALV4-DAYPICK-A11Y`, now
also holding drag-create's missing keyboard equivalent; `C-CALV4-TOKENS-RESIGN`,
booked a **sixth** time, with two of the seven defects now visible in stills the
operator has signed; the live-authed CSRF case DAYCARD could not measure; **the
day-of-week wrap at 390**, which is performed by no test and no image and is now
named as unperformed in `daycard_test.go`'s own handoff comment rather than
pointed at a gate that does not do it.

Books: ADR-048 §27 · `reports/chronicle/2026-08-02-C-CALV4-EDITOR-R2b.md` ·
`reports/chronicle/screenshots/2026-08-02-c-calv4-editor-r2b/`.

### calendar-v4 — W-H SHIPPED: the builder wizard, the last wave (2026-08-02)

**The remodel's last wave, and the only one whose gate was *finish*.**
`C-CALV4-WIZARD-P13` shipped `GET /campaigns/:id/calendars/builder` — nine
stations presented as Start plus eight steps, a five-card preset gallery, the
front door for the 4-format importer, and a live month preview that IS the
shipped Block rendered from an in-memory `*Calendar` that is never persisted.
Operator signature: `decisions/2026-08-01-operator-signatures-wz1-sky-editor.md`
("Signed — build P13"); the dispatch was 16/16 signed.

**Almost none of it is new capability, by design (L6).** The preset gallery is
the importer with four embedded payloads read through the SAME `DetectAndParse`
an upload meets — no preset table, no migration, no new parser, no second apply
path. The importer front door is the existing parser behind a route with no
`:calId`. What is new is the shell, the honesty states, and the motion.
**The two parser gaps W-H booked rather than owned are closed (sweep R3,
2026-08-07):** `parseCalendaria` is now deterministic (moons were never sorted
at all and seasons tied three ways at `dayStart` 0 in `presets/elven.json`, so
100 parses of the Elven card's own bytes gave 2 moon orders and 3 season
orders), and `parseSimpleCalendar` carries the file's own `calendar.name`
instead of naming every Simple Calendar import "Imported Calendar". Details in
`.ai/todo.md` under Critical.

**The three-card V1 setup chooser is RETIRED.** `GET /calendars/new` resolves to
the wizard, so every link across the product and every external bookmark lands
on the designed surface with no href edited in a file the wave did not own —
including the frozen `calendar_v2` helper. Three pins refreshed, none deleted.

**W-H's durable deliverable is a guard the motion policy asked for on
2026-07-27 and nobody had built**: a property/keyframe/reduced-motion/duration
budget over `static/css/calendar-builder.css`, landed BEFORE a line of that
sheet existed. Read ADR-048 §26 for what it can and cannot see — a comment
terminator inside prose silently disabled the entire motion register while all
six assertion families stayed green, and only a browser probe caught it.

**A FIX ROUND FOLLOWED, AND IT IS PART OF THE WAVE.** Adversarial verification
rejected the first pass on five blocking findings, every one of them found in a
browser against an entirely green suite: a moon-name input rendering 0px wide, a
"no turn this month" that was unconditional and false, an era name clipped
mid-word, a blank inside the leap station's flagship sentence, and three preview
features absent from the build while the copy asserted them. All five are fixed
and the evidence that missed them is widened — the narrow-lane probe now walks
all eleven station sheets at all seven widths and measures whether a control can
show its own value, and the `?step=` reject path is pinned on the GET as well as
the form. ADR-048 §26 records the three lessons.

**A SECOND FIX ROUND FOLLOWED IT, AND ITS FIRST FINDING IS THE WAVE'S WORST.**
An undisclosed edit had re-pointed `Index`'s zero-calendar branch at the wizard —
and `Index` is on the PUBLIC group behind `RequireViewAccess`, so a player AND an
anonymous visitor on a public campaign rendered the whole builder at 200, both
`needs backend` chips and the Create button included. §6.3's "every viewer of the
wizard is an owner, satisfied BY CONSTRUCTION" had only ever been true of the
three ROUTE REGISTRATIONS; a Go call does not read the route table. The floor now
travels with the handlers, a non-owner goes to V2's own empty state, and roles
are exercised by tests rather than asserted by a test's name. Also closed: the
fault sheet now draws the composition [WZ-15] item 5 ratified (the anchor stays,
the fault goes where the grid would be) instead of a headline claiming the date
cannot resolve above a Nameplate resolving it; the fidelity index lists
twenty-six differences instead of nine "exhaustive" ones (twenty-four when this
paragraph was first written; the third round added the one the second missed and
the one it created deliberately); §12.1(ii)'s five motion clips exist and the
stills are captured at rest and reproduce **content-identically, caption-glyph
rasterisation aside** — re-measured across two full regenerations, 39 of 42
byte-identical with `importer--dark`, `presets--mobile-light` and
`step-eras--dark` differing only inside rendered text runs, and WHICH three land
in that set varies between run pairs; and the importer, which had been parsing
the dropped file and then discarding it, now adopts it as the draft. ADR-048 §26
carries all of it.

*This sentence said "byte-for-byte" and "twenty-four" for one round longer than
it was true*, in the very entry whose round claimed every false evidence claim
had been corrected. The evidence index and the report were fixed and Chronicle's
own books were not, which is the same failure at a smaller scale: a correction
that does not travel to every place the claim was made has not been made.

**A THIRD ROUND CLOSED IT, ON FIVE COORDINATOR RULINGS.** Two of the five were
decisions rather than repairs, which is why they came from the coordinator and
not the executor. **The delay ladder [WZ-8] tabulates had never run**: the rule
sat before pass 2's `animation:` shorthand at equal specificity, the shorthand
reset `animation-delay` to `0s`, and every static guard passed because every
static guard asks whether the rule exists. It runs now (0 / 33.3 / 66.7 / 100 /
133.3ms, measured in Chromium — the dispatch's tabulated "≤132ms" is the same
rule with `--m-step` rounded to 33ms in prose, and the sheet now carries the
division rather than the rounding), pinned by a byte-ORDER assertion and a browser
probe, and it **deliberately diverges from the sealed mockup**, which carries the
identical defect — the written signed mechanism outranks the drawing's accident,
and the mockup's own one-line fix is booked, not taken. **The wizard's host layer
set gained `eras`** (ruling R3, a deliberate re-pin of [WZ-2c]): the preview
draws the era bands the signed stills draw, DEF is unchanged, and the preview's
disclosure note shrank to the two absences still true — pinned together, with the
stale phrases asserted gone by name. **The indigo-vs-amber CTA** is disclosed and
booked to `C-CALV4-TOKENS-RESIGN` rather than patched, the sibling wave's way.
**The preset ↔ `BuildExport` round trip** the acceptance list required exists and
found one asymmetry (a moon's colour), asserted exactly and booked. And the
evidence's own arithmetic now describes the photograph — plus one more defect the
regeneration surfaced: the two "reduced motion" stills were ordinary renders,
because the capture never asked Chromium for the preference the harness's media
query needed.

Books: ADR-048 §26 · `reports/chronicle/2026-08-02-C-CALV4-WIZARD-P13.md` ·
screenshots, clips + measurements under
`reports/chronicle/screenshots/2026-08-02-c-calv4-wizard-p13/`.
### HOTFIX — two independent defects the operator hit at once (2026-08-02)

Branch `claude/hotfix-boost-scripts-prefs-fk`, off the #584 merge. Both were
reported as one symptom ("the calendar page does nothing"), and both were
reproduced before anything was changed.

1. **Boosted navigation deleted the Bench's page scripts.** The App layout puts
   `{children...}` inside `<main id="main-content">`; every sidebar link is
   `hx-boost="true" hx-select="#main-content" hx-swap="innerHTML"`; `boot.js`
   sets `htmx.config.allowScriptTags = false`, at which point htmx's
   `makeFragment` REMOVES script tags from the swapped fragment rather than
   skipping them. `bench.templ` mounted `cal_visibility.js`,
   `calendar_permissions.js` and `calendar_daycard.js` at exactly that depth, so
   the Bench wired on a direct load and silently did not through the sidebar —
   dead day card, dead Permissions button, and nothing looking wrong, because
   `<link rel=stylesheet>` in the same region survives the same code path. Fixed
   by moving all three into the plugin body-script registry
   (`internal/app/routes.go` → `layouts.SetPluginBodyScripts` → `base.templ`),
   which emits outside the swapped region. **Class defect:** 29 more page-side
   `<script src>` tags across 16 templs remained; `tools/check-page-scripts.sh` +
   `tools/page-script-allowlist.txt` is a whole-tree ratchet (wired into CI,
   self-testing) that lets the count only shrink, and the sweep is booked as
   **C-HTMX-SCRIPT-SWEEP**. **First survivor closed (sweep R3, 2026-08-07):**
   the Characters page's `characters.js` — measured live in headless Chromium
   against the vendored htmx 2.0.4 and boot.js's real config (direct load wires
   the cast cards' quick-look; boosted sidebar nav never fetches the file and
   the button does nothing), moved to the registry, ratchet lowered to
   **28 across 15**. The other 15 are undiagnosed. **Ratchet strengthened
   (sweep R3, 2026-08-07):** it counted one literal byte sequence,
   `<script src=`, so `<script defer src={…}>`, `<script type="module"
   src={…}>` and a newline-split open tag put the file into the inventory not
   at all — the guard exited 0 with all three present, and `templ fmt` does not
   normalise attribute order, so the evasion survived `make templ` and CI. The
   harm is order-blind (htmx removes scripts BY TAG NAME), so the guard now is
   too: it walks the open tag a character at a time, the same walk
   `check-calendar-v4-lints.sh` does for B3/B4. Tree counts unchanged — the gap
   was latent — so the allowlist needed no edit; the three forms plus an
   inline-body decoy are fixtures in the guard's own self-test.

2. **Every calendar preference write was a guaranteed FK violation.**
   `SetSidebarPinned` / `SetBlockLayers` / `SetBenchSections` inserted an empty
   `calendar_active.calendar_id`, which migration 006 declares NOT NULL and
   foreign-keyed to `calendars(id)` → errno 1452 → 500 → no `HX-Refresh` → a
   switch that visibly did nothing. InnoDB checks the FK on the attempted
   insert, so `ON DUPLICATE KEY UPDATE` never rescued it and this failed for
   every viewer. Invisible to CI because all existing tests mock the repository.
   The service now resolves a real calendar (reusing `resolveActiveCalendar`'s
   ladder) and passes it down; the conflict clause still touches only the
   preference column, so a preference write cannot move a chosen active
   calendar. Failures now map to a named `apperror` instead of an anonymous 500.
   **Real-DB proof is booked, not claimed** — no database runs in the build
   environment; see **C-PREFS-FK-INTEGRATION** in `.ai/todo.md`. The operator's
   instance is the live confirmation.

Both entries, with the full reasoning, are in `.ai/todo.md` §1 Critical; the FK
correction also amends `internal/plugins/calendar/.ai.md`'s C-CALV4-LAYERS-P9
section, which described the empty-string seed approvingly.

### calendar-v4 — W-G PART B SHIPPED: `/campaigns/:id/schedule` (2026-08-01)

**The operator's #1 feature has a page.** `C-CALV4-RSVP-P8` Part B was gated on
a drawing pass — the spec forbade building the Verdict, the Matrix, the Roster
or the Painter from its own prose — and the gate closed when the operator signed
`mockups/calendar-v4-schedule.html` on 2026-07-29 against the `wg-schedule-*`
stills. **Where that mockup and the W-G spec disagree, the mockup wins**; the
spec now carries a supersession note naming the sections it overrules.

- **Five surfaces, two role orders, one route.** Director: Verdict → Matrix →
  (Roster · Painter) → Answer. Player: (Answer · Painter) → Verdict → Matrix.
  SOURCE order, never CSS `order:`. `GET /campaigns/:id/schedule` at Player+ is
  the whole route budget — `routes_snapshot.txt` **723 → 724**, no migration.
- **No new write path.** The Painter writes through the scheduler's shipped
  availability PUTs (two of them, because `[This week only | Every week]` means
  two things and already has two tables), the Answer through P8A's event RSVP
  POST, the Nudge through P8B's `/calendar/ask`. Forking a second availability
  write would fork the composition invariant with it.
- **Rank 1 IS the Bench's derived window**, by construction: `benchRsvpPeakRun`
  is extracted and shared, and the oracle asserts both surfaces name the same
  day and the same hour.
- **Permission is absence, including in the prose.** A player's payload carries
  no lane, no other member's name, no chip — so there is no `if IsGM` in the
  markup at all. The one real bug the shots found was a SENTENCE: reason clauses
  computed over an absent lane map told a player that five of five people had
  ignored the question.
- **The door is the Bench's RSVP panel title**, which has read `RSVP · Schedule`
  since the signed contract drew it. Not the nav — WG-2's ruling keeps that
  pointing at `/availability` and books the retirement as its own slice. The
  Bench's 23 shot keys are byte-identical with the link in place.
- **The fidelity gate is pixels and it paid for itself five times**, catching
  twenty-two defects every string assertion had passed before the verdict round
  found three more (below). The third pass was mostly
  about SHAPE rather than words: every caption on the page shipped with the right
  sentences and none of the drawing's emphasis — five bold lead-ins gone, two of
  which ARE this surface's named honesty claims. Captions are a `[]ScheduleRun`
  model now, never a markup string. Also: the matrix caption was three paragraphs
  where the drawing returns one, the count lane ran its numeral beside its hour
  instead of over it, the matrix head named the week but not the visible band,
  and a member with no zone was drawn amber where the drawing draws it grey.
- **The fourth pass was not pixels at all — it was a PARSER**, and it found six
  the eye had signed off four times. Diffing the sealed `<style>` against the
  shipped sheets declaration by declaration (normalising the `.cal-schedule`
  scoping and whitespace) caught: every segmented control rendering its selected
  rung **PINK**, because `color-mix(in oklch, var(--surface-card) …, var(--accent))`
  mixes from an ACHROMATIC surface and oklch resolves the missing hue by the
  short arc — 349.2deg the wrong way round the wheel — where the drawing mixes
  into `transparent` precisely so it cannot; a `.say .badge` rule dropped from
  the MIDDLE of a run of narrow rules the sheet otherwise carries in order,
  leaving every chip ~27% oversized at 390; a drawn `width:auto` "tidied" into
  `min-width:0`, which is a no-op replaced by a real effect and collapsed the
  answer well until `19:00` and `in` collided; and a missing `gap`, a missing
  `:hover` and a `cursor: default` where the drawing writes `not-allowed`.
  **Screenshots show a page that looks plausible — a 27% oversized chip still
  looks like a chip, and a pink pill still looks like a selected pill.** Only a
  mechanical diff sees a rule that is simply absent.
- **A measuring instrument that differs from its subject reports its own
  defects as the subject's.** The harness padded 20px at every width where the
  product's `<main>` pads 12px below 768 — 8px, and the drawn narrow matrix has
  exactly 8px to spare, so the phone shot clipped a column the shipping page does
  not. Three harness lessons are now written down in
  `internal/plugins/calendar/.ai.md`: shoot with a driver (the Chrome CLI clamps
  `--window-size` to a 500px minimum), scroll a control into view before
  measuring its target, and **measure horizontal overflow per ELEMENT** — a
  nested `overflow-x:auto` container never contributes to
  `documentElement.scrollWidth`, so "the page does not drag sideways" was true
  while the matrix dragged inside its own panel.
- **The fifth round found a whole VIEW, and the parser found it again.**
  `.sc-ruler` and `.sc-head-row b` were absent from the shipped sheet; pulling
  that thread found that **`?zoom=day` was a live control that changed nothing** —
  it round-tripped the query and inked `aria-current` while `scheduleColumns` and
  `scheduleDayCount` never read it, so the Day rung returned the identical week
  matrix. At 390 the same rung is DISABLED in the drawing (`--text-muted`,
  `cursor: not-allowed`, `title="week zoom is forced at this width"`) and the
  build shipped a live link at every width, while `ScheduleToggle.Disabled` was
  set NOWHERE — leaving a doc comment, a templ branch and a CSS `:disabled`
  repair all describing a refusal that did not exist. All three are closed: the
  day view is built out of the week payload the Matrix already ships (no route,
  no query, no seam), and the refusal is a real disabled rung that a media query
  chooses between — the server cannot see a viewport, which is ADR-048 §13's own
  bound, so both rungs ship and exactly one is ever displayed. Two footnotes
  worth keeping: the sealed sheet DECLARES `.sc-ruler` and never renders it
  (`const ruler = DAYZOOM ? '' : '';`), so those 21 declarations are deliberately
  still not carried — a rule matching nothing is the defect that was just fixed,
  not its repair; and measuring the refused rung turned up four more drawn
  declarations the sheets never carried, growing every page-head control to a
  real touch target at 390.
- Report: cordinator `reports/chronicle/2026-08-01-C-CALV4-SCHEDULE-PARTB.md` ·
  ADR-048 gained a Part B section · shots in
  `reports/chronicle/screenshots/2026-08-01-c-calv4-schedule-partb/`.

### calendar-v4 — ROUND 2 OPEN · R2-2a (THE DAY ANSWERS ITS CLICK) SHIPPED (2026-07-31)

**`C-CALV4-DAYCARD` stages 1-2 closed the operator's loudest live-client
complaint:** *"I'm unable to click and do anything, it just selects the date and
nothing happens."* Both halves were mechanically true — the click sets a
visually-hidden radio and the CSS-only ANSWER ladder filters the docked Ledger
(quiet by design; where the `ledger` layer is off it emits no control at all),
and calendar-v4 had no way to create or edit an event anywhere.

- **The card** is the pointer-first answer ON TOP of the CSS-only one. One
  page-level `[popover]`, positioned from the clicked cell's rect, listing the
  day with the LEDGER ROW's own field set. **The shipped ladder is untouched.**
- **The agreement law is pinned in Go, at the producer, joined to the count
  oracle** (GM / Nissa / Bryn): the card's event-id set per day EQUALS the set
  the ladder leaves visible. One source, one viewer-filtered pass. A card
  showing one more event than the Ledger is a permission leak wearing a UI
  change's clothes.
- **The editor's MECHANISM** ships against the shipped, IDOR-closed event API:
  **zero new write routes.** Create/edit are `POST`/`PUT` (Scribe), delete is
  `DELETE` (Owner), all through `Chronicle.apiFetch`. Its full §5 chrome pass
  and drag-create split out as **C-CALV4-EDITOR-R2b** at a stage boundary,
  under the dispatch's pre-authorised split.
- **ONE new read route**, the whole budget: `GET
  /campaigns/:id/calendars/:calId/events/:eid`, authed `cg`, `RolePlayer` floor,
  literal path, the grid's own viewer filter inside. Hidden / filtered /
  `:calId`-mismatched / missing are **one branch, one body**.
  `routes_snapshot.txt` **722 → 723**, one addition, zero removals.
- **Motion CONSUMES R2-1's register rather than opening a second one**
  ([DC-6], first-lander clause). Two rules added by name inside
  `calendar-bench.css`'s existing register section and inside its single
  `prefers-reduced-motion: no-preference` wrapper, reusing all three `--disc-*`
  tokens. The allowlist was widened by zero bytes; what was added is the CLAIM
  that the card is inside it, failing in both directions.
- **The visibility mapper was EXTRACTED, not copied a third time** — `mode ↔
  {visibility, visibility_rules}` now lives once in
  `internal/plugins/calendar/static/js/cal_visibility.js`, and both the
  permissions modal and the day-card editor read it.
- **Permission is absence, mechanically.** Every role gate is markup-level from
  the producer; a player's Bench contains no editor scaffold, no field name and
  no route string. The Block's DOM is asserted byte-identical before and after
  open + close.

**Known gap, stated rather than papered over:** where the Ledger is NOT docked a
day has no focusable control at all, so the card is pointer-only for that
viewer. No `tabindex` was injected (that is a Block mutation and a control the
server never rendered). Booked as **C-CALV4-DAYPICK-A11Y**.

**Fix-forward round 1 (stages 3-5), against the slice's adversarial review:**

- **The occlusion report reached no consumer** (DC-CLEAR-1). `placeCard`
  computed a `clear` flag; two comments and a commit body promised an
  unclearable geometry would be "visible rather than silent"; nothing read it.
  The one condition [DC-3] signs as a STOP-AND-FLAG shipped as the quietest
  thing on the page — the "sentence promising a guard that never ran" class
  R2-1's stage 9 existed to kill. The mobile branch was additionally returning
  `clear: true` without ever consulting the Ledger's rect, while measuring
  8.5k-54k px² of overlap. `clear` is now MEASURED the same way in both
  branches, from the box that actually landed, and it has two readers:
  `data-dc-clear` on the card's root at every width (a report, guarded against
  ever being styled) and ONE console warning per session at desktop widths only.
- **The intercalary payload path was 0% covered** (DC-ICAL-2) while daycard.go
  claimed the mirrored key helpers were pinned "in either direction" — true of
  `slug-N`, false of `slug-iN`, because the signed fixture declares no
  intercalary month. Five tests against real intercalary months spliced into the
  oracle fixture, including two of UNEQUAL length, which is the shape the coords
  zip fails on. The shipped code was correct; the guard and the comment were not.
- **Two listeners reached `edSave` with no in-flight guard** (DC-SAVE-6). Single
  -fire rested entirely on `preventDefault` beating the submit button's
  activation; a capture-phase move or a branch reorder would have made every
  Save write twice, and a double-click did it already. Guarded at the WRITE, not
  the listener.

**Fix-forward round 2 (stages 7-8), against the second adversarial review:**

- **A rename un-repeated the event** (DC2-RECUR-DATALOSS, the blocker). The
  editor authors no recurrence in this stage and "preserved" it by saying
  nothing about it — but `is_recurring` is a VALUE-typed bool on the shipped
  `PUT` and the service assigns it unguarded ON PURPOSE ("false IS the value,
  not 'absent'"), so omission was a write of false. The nil-guarded
  `recurrence_type` / `interval` / `end_*` around it then survived, leaving the
  exact half-state C-CAL-RECURRING-PARTIAL-STATE-CLEANUP cleaned up once. The
  read route now HANDS BACK the three fields the write path clobbers and the
  editor sends them home; create mode still sends none, because for a new event
  false is the true value. The lesson is in ADR-048: *"the client round-trips
  what it does not offer" is true field-by-field, and its truth depends on the
  field's TYPE on the wire.* The remaining partial clients of that PUT are
  booked for the same sweep.
- **Two guards were green-but-blind, both by enumerating from a SAMPLE**
  (DC2-PAYLOAD-OMITEMPTY, DC2-SCOPEGUARD-LINEFORM). The payload law's key
  inventory came from marshalling a hand-written literal, so a ninth field
  tagged `omitempty` would have been invisible to "want exactly these eight
  keys"; the stylesheet's scope guard read only lines ENDING in `{`, so the
  sheet's 21 single-line rules were never examined. Both now derive from the
  definition — reflection over the type's json tags, a brace-scanner over the
  sheet — and both go red on the verifier's own mutations.
- **The W5a same-answer table was missing the ownership row**
  (DC2-W5A-OWNERSHIP). `GetEventAPI`'s header claims FOUR refusals are
  indistinguishable; only three were pinned, and the unpinned one is the branch
  that separates "this event is on a calendar you can see" from "one you
  cannot". Sixth row added, with both calendars resolvable inside the campaign
  so the ownership check is what actually fires.

**Fix-forward round 3 (stages 9-13), against the third adversarial review:**

- **The card covered the STACKED Ledger, deterministically, in the DESKTOP
  treatment** (DC3-STACKED-LEDGER-OCCLUSION-1, the blocker — [DC-3]'s own
  STOP-AND-FLAG). `placeCard`'s dodge was HORIZONTAL only, which is complete
  while the Ledger is a right-hand column and structurally impossible once the
  Bench stacks it full-width BELOW the grid: the clamp computed a negative limit
  and no-opped, and the vertical branch above it only ever flipped for viewport
  room. Between roughly **625px and 884px of `.cal-bench` content width** the
  card therefore landed on the band for every day and every viewer, measured at
  21,080 px² against the real render. The editor escaped only by accident — too
  tall to fit below, so its viewport flip happened to clear the band, which is
  the proof the missing dodge was available. `placeCard` now treats the Ledger's
  SETTLED RECT as an exclusion zone whatever its role in the layout, and tries
  below → above → the bottom sheet ([DC-3] bullet 4's own signed answer; no
  third geometry, no resizing). Same three widths now measure 0 px²,
  `data-dc-clear="1"`, zero warnings. The desktop sheet FALLBACK still warns,
  because the geometry running out is the signed condition even when the card no
  longer covers anything; the mobile sheet stays silent, as DC2-MOBILE-4 set it.
- **The suite had mislabelled the product's own layout as pathological.** The one
  negative placement case called the stacked-Ledger geometry "a pathological
  geometry: the column starts 40px in", which is why the hole survived two
  reviews. It is now positive regression coverage at the ~884px and ~944px
  boundaries, plus a case for the sheet fallback.
- **A driver was mounted without the global it reads** (PERM-JS-HARD-DEP-2).
  [DC-10]'s extraction left `calendar_permissions.js` depending hard on
  `window.ChronicleCalVisibility` with no fallback — correct, but it means the
  driver mounted alone wires nothing, and `app_dashboard.templ` mounted it alone
  on a page that is retained-but-unrouted. Mount dropped; absence pinned against
  the page that renders it; and a source-level guard now requires any template
  mounting the driver to mount `cal_visibility.js` FIRST.
- **The editor wrote to the id the SERVER echoed, not the door that was clicked**
  (EDIT-MODE-ID-FALLBACK-3). The same line decided PUT-vs-POST, so a record
  without an `id` would have turned an edit into a duplicate-creating POST.
  Edit mode now carries the door's id, Delete follows it, and one pure
  `writeTarget()` REFUSES an edit with no id rather than creating.
- **Guard B4 did not read the day card** (DC2-B4-GLOB-4, ruled). `*daycard*`
  added to the scope glob with two self-test rows — coverage widening only,
  mutation-verified in both directions.

**Fix-forward round 4 (stages 14-15), against the fourth adversarial review:**

- **The round-3 fix closed the short-box case and opened a larger one on the same
  code path** (DC3-DESKTOP-SHEET-OCCLUSION-R4, the blocker). The two-candidate
  dodge admitted the ABOVE position only when it fitted the viewport outright and
  sent everything else to a clamp that pins the box to the BOTTOM of the
  viewport — where the stacked band is. A box taller than the room above its day
  therefore never flipped, failed both candidates and took the DESKTOP SHEET,
  covering **100% of the Ledger** across the same 625-884px band, while warning
  that the geometry was impossible. It was not impossible, it was not attempted:
  a 481px box at top=8 ends at 489 and the band starts at 595. Reachable two
  ways with ordinary data — `+ New event` (a 420x400 editor: 107,604 px² at
  884px, a REGRESSION against the pre-round-3 module, which placed the same
  editor clear) and a day with 12+ events (≈379px). `above` is now CLAMPED into
  the viewport rather than dropped: the same clamping the below-candidate always
  had, in the direction this one prefers. Still two anchored candidates and one
  sheet; the card is still never resized; the sheet fallback is unweakened and
  is now the only thing its STOP-AND-FLAG speaks about. **The lesson is not
  round 3's:** the round-3 fix was verified on the case it was written for and
  not on the surface that case hands off to, and the module's own comment
  ("THERE IS NO THIRD GEOMETRY") foreclosed the placement that fixes both.
- **A harness fidelity gap hid the editor half for three rounds.** `daycard_dom`'s
  rect is a static all-zero and the card is the EDITOR's anchor, so every editor
  in the JS suite was placed at the viewport origin, where nothing can be
  occluded. The card's rect is now derived from the placement the module wrote,
  and the editor regression goes red without it.
- **The route record's field law was a deny-list**
  (GETEVENT-FIELD-BOUND-IS-A-DENYLIST). §8's "only fields the editor writes
  back" is pinned twice; the payload's half reads the type by REFLECTION, the
  route's half asserted six required keys and fifteen hand-written forbidden
  names, so an `owner_id` or an `attendees` would have passed in silence — the
  exact shape the payload's own guard was fix-forwarded away from one round
  earlier. Both halves now read the definition.

**The §12 screenshot gate is PART-EXECUTED** — the geometry rows are measured
headlessly against the real render (1232px clears the Ledger at 0 px²; the
625-884px stacked band clears it after rounds 3 and 4, for short boxes and tall
ones; 390px is the signed bottom sheet, recorded rather than mis-reported); the
rows that need a live authed session are still open. Both halves are in
`.ai/todo.md`. Round 3's disclosure lesson rides with it — **a geometry claim is
a claim about the widths it was taken at** — and round 4 adds the second axis:
it is also a claim about the BOX HEIGHTS it was taken at, and the surface the
card hands off to has a different one.

### calendar-v4 — ROUND 2 · R2-1 (THE REVEAL PASS) SHIPPED (2026-07-30)

**`C-CALV4-BENCH-R2` slice R2-1 closed the operator's three 2026-07-29 live-client
complaints.** Round 2 adds no data, no zone and no engine: it decides what a
viewer meets first, how wide it is allowed to be, and what a surface says when it
is closed.

- *"where are the menus" / "5-6 blocks of data before you get to the calendar"* →
  four Bench sections (`ribbon` · `rsvp` · `nextup` · `rows`) are now native
  `<details>`/`<summary>` disclosures, **closed by default at every width**, each
  stating one true line computed from the VIEWER's own payload. The two Blocks,
  `.phead`, `.sechead` and `.caption` never collapse — the subject of the page may
  not be inside a disclosure.
- *"so stretched out, especially the RSVP menu"* → `--bench-measure: 1180px` on a
  page that had no `max-width` at all, and the RSVP panel becomes a two-column
  grid at ≥1024px via `display: contents` with a **byte-identical DOM**. The
  `.benchblock` measures **1144px @1440 / 1180px @1920**, both clear of the
  Block's 900px full-tier floor: no silent demotion.
- *"on the entity it scrolls"* → the entity embed's seed drops from five keys to
  `["moons","eras","weeknums"]` — exactly the two keys that add a ZONE leave, the
  three inside the month stay. No widget file was edited; that is what [LYR-3]'s
  seed rule bought.

**Store:** migration 016 `bench_sections` on `calendar_active` — the THIRD
extension of that row, under the same PR #368 decision 007 and 014 both cite. It
holds the **CLOSED** set so `NULL` (never chosen) stays distinguishable from `''`
(closed nothing). **ZERO new routes:** the existing `POST
/campaigns/:id/calendar/prefs` grew one optional `section=` field and
`routes_snapshot.txt` is byte-identical. That branch answers 204 with **no**
`HX-Refresh` while `layers=` keeps it, and the asymmetry is documented where the
next hand will misread it.

**Motion:** the product's first signed animation family ships —
`cordinator decisions/2026-07-29-motion-disclosure-register.md`. Clip-reveal +
opacity on `::details-content`, 200ms open / 160ms close, easing READ from the
Block's own `--ease` ladder, and the whole rule block inside ONE
`@media (prefers-reduced-motion: no-preference)` so reduced motion is instant and
complete rather than shortened. `TestBenchCSS_NoMotionAtAll` was **inverted to an
allowlist, never deleted**, and now asserts `close < open` arithmetically.

**A three-wave-old warning was measured out.** The entity producer claimed
dropping `ledger` would break the full-tier column arithmetic. `ColWidth` /
`IsNamed` / `IsNamedCSS` have zero non-test callers, the density flip is a
`@container cal-cell` query, and the full-tier track is `minmax(0, 1fr) auto` —
an absent Ledger collapses its own track. The one real consequence is the
opposite of the fear: named columns now flip on at a **narrower** host
(1198px → 898px for a ten-day week). Pinned by
`entity_ledger_geometry_test.go`.

ADR-048 gained a **section** (there is no ADR-049) covering the disclosure
mechanism, §13 met for the SECOND time (a pattern, not an incident), the third
extension of `calendar_active`, and the three-way distinction between
permission-is-absence, needs-backend-omission and **compactness-is-a-choice** —
all three render as "less" and are indistinguishable in a screenshot.

**Verifier round, fixed forward (2026-07-30, stages 6–8; the history was
pushed, so nothing was amended).** Three findings, all in what the slice had
already shipped:

1. **The disclosure flip was speaking for the eight controls inside it.** htmx
   resolves `hx-vals` by WALKING ANCESTORS (`bn()` recurses to `parentElement`
   in `static/vendor/htmx.min.js`), and the flip has to ride on the `<details>`
   because `toggle` does not bubble — so `section=<key>` was silently appended
   to the player's RSVP trio, the owner's `Ask →` form and all five sort links.
   Benign (their handlers ignore unknown fields) and a live trap: a control
   inside a disclosure posting `layers=` would inherit `section=` and be
   rejected 400 by this slice's own "exactly one of" guard. **The key now rides
   the POST URL**, which nothing inherits; Echo reads it either way because
   `FormParams` → `ParseForm` merges query and body on a POST.
2. **The closed ribbon did not name the session it was holding.** A player
   receives three tiles and their ONLY RSVP answer control is inside that
   disclosure; the summary was built from the Today tile alone. A `session`
   clause now prints the tile's headline and the viewer's own standing
   ("Session 41 · You: in"), gated on the existing `NeedsBackend` marker so
   build status never becomes a sentence.
3. **A twisty comment described a rotation the sheet never shipped** — the same
   defect stage 4 existed to delete, arriving fresh. The mechanism is a content
   swap (▸ / ▾ on `[open]`); the comment now says so and `bench_test.go` asserts
   no rule naming `.disc`/`summary` declares `transform`.

**Second verifier round (2026-07-31, stage 9; also fixed forward).** The same
class a third and fourth time, which is why the fix is now a PIN over prose and
not another rewrite. `bench.templ`'s header still read "calendar-bench.css
defines no transition, no animation and no @keyframes, and its contract test
says so" — both clauses died in stage 3 — and `calendar-bench.css`'s RSVP header
read "the Bench sheet is under tools/check-v2-motion-discipline.sh". **That
second one is the dangerous half**: the guard scopes to
`internal/plugins/{calendar,timeline,ai_workspace,campaigns}` and filters to
`${scope}/*.templ` / `${scope}/*.css`, so it has never seen `static/css/` — and
line :50 of the SAME FILE says so correctly. The sheet contradicted itself, and
the false half named a CI net under the one allowlist [BR2-2] warned has no net.
`TestBenchProse_MotionClaimsMatchTheSheet` now derives both facts: it fails if
any sheet-wide motion denial survives while the sheet declares a `transition:`,
if the guard is named anywhere near this sheet without "does not police", or if
`bench.templ`'s header stops citing the register and the test that enforces it.

**⚠️ Operator, and it is flagged at the PR gate:** [BR2-4] diverges from the
literal instruction "desktop defaults open, mobile defaults closed". One
server-rendered `open` cannot vary by viewport (ADR-048 §13). What shipped is
closed-everywhere + CSS `order` putting the calendar above the ribbon at ≤640px.
It is cut-able at the gate.

**⚠️ No screenshots.** The §13 measurement gate could not be executed: this
environment has no browser and the Playwright chromium download failed. Every
geometric claim above is derived arithmetically from the shipped stylesheet and
shell, and pinned by tests. See the slice report for the full list of what a
live client should still confirm by eye.


### calendar-v4 — WAVE 3 TAIL · THE ASKING EMAIL SHIPPED (2026-07-29)

**`C-CALV4-RSVP-P8B` closed the operator's own 28 July directive and the last
`needs backend` chip inside the RSVP panel.** Chronicle can now ask a table for
its schedule.

**The route:** `POST /campaigns/:id/calendar/ask`, Scribe+, inside
`RegisterRSVPRoutes`' authed `cg` — **721 → 722, one addition, zero removals**.
Same role floor as the `rsvp-collection` toggle it is the re-send of, for that
toggle's own stated reason: enabling is the invite moment, so it must not be
reachable by the people being invited.

**One email, two sections.** The schedule ask leads — subject and primary CTA —
and needs no session, so it is valid in a campaign that has scheduled nothing.
The five EXISTING RSVP action links ride the same email when a collecting
session is resolvable AND the recipient may see it, re-minted through the
unchanged `MintActionTokens`. `renderScheduleAskEmail` is a sibling of
`renderInviteEmail` sharing lifted package-level helpers; the invite email's
output is byte-identical after the lift.

**The CTA is a plain deep link, and that is a correction to the booking, not a
reduction.** *"A tokened link to the availability grid"* is not buildable: the
grid is a 1,318-line client SPA over six authenticated JSON routes, so a token
would have to authenticate all six or mint a session from an emailed link. The
directive's one-time-token pattern is honoured by the RSVP links in the same
email. See ADR-048 §25(b) — it is the section a future slice will need most.

**Three auth defects repaired first, in their own commit** (stage 1, fenced to
destination preservation + sanitisation): `handleUnauthenticated` now carries
where you were going into `/login?redirect=…`; the login form carries it through
the POST as a hidden field exactly as register already did; and `Login` uses
`sanitizeRedirect` instead of a bare `HasPrefix(redir, "/")`, which accepted
`//evil.example`. The third was unreachable ONLY because of the second, and this
slice is what made it reachable.

**The rate limit is persisted.** Calendar migration `015_schedule_asks`: a 6h
per-campaign cooldown and a 24h per-recipient floor, plus a per-user 10/h
in-memory limiter on the route. The cooldown REFUSES the send; the floor SKIPS
that member, so a second ask after somebody joins mails the new member and
nobody else. Nothing is recorded when nothing was sent.

**The panel's `Nudge` is LIVE, in exactly one place.** Both session-tile Nudges
were RETIRED rather than lit — two live buttons mailing one roster from one page
is a double-send affordance. `Propose` keeps its chip and `ActionsWhy` shrank to
name it alone. The Bench's own chip enumeration went from four to three. The
`email not configured` state is a `.badge.warn`, never `.badge.need`.

**Still open and named:** the propose-from-window write path; ledger #3
(`HasPattern`) which stays Part B's; a plain Scribe has the ask capability and
no button, because the panel `.side` is GM-tier by WG-8; and **`C-NOTIFY-PREFS`
is newly booked** — this is the first Chronicle email a member can receive
repeatedly because somebody else pressed a button, and there are no notification
preferences anywhere in the product.

---

### calendar-v4 — WAVE 3 · W-F LANDED, THE LAYER SYSTEM IS REAL (2026-07-29)

**A default nobody can leave is not a default — and for three waves, nobody
could leave DEF.** `C-CALV4-LAYERS-P9` (W-F) built the per-viewer preference
store that `block_projection.go`, `entity_calendar_block.go`, `bench.go` and
`shelf.templ` had all been writing "does not exist" about, in the same words,
since wave 1.

**The store:** calendar migration `014` adds `block_layers VARCHAR(255) NULL` to
`calendar_active` — extending the row migration 007 already chose to extend
under PR #368 stop-and-flag #3, rather than minting a table. `NULL` means "never
chosen" and renders the HOST'S SEED, so every wave-1/2 screenshot stayed valid
on day one; `''` means "chose nothing" and renders a bare month. **The two are
different answers at every layer**, because collapsing them makes the bare month
unreachable and turns the default into a floor.

**The route:** `POST /campaigns/:id/calendar/prefs`, the calendar-v4 wave's ONLY
new route — **720 → 721, one addition, zero removals**. It answers **204 with
`HX-Refresh: true` and no body at all**, so the host page rebuilds every Block
through the handler that already owns `requireVisibleCalendar` and the W5a
split. Nothing about which calendar or whose entity is ever taken from a request
body, because there is no response to build. That single choice is why §12.1's
security review is short by construction rather than by argument.

**The switchboard is a top-layer `[popover]` with NO JS ANYWHERE** — opened
declaratively by `popovertarget`, eight rows, "of 8" for every role. Pin
amendment **r54** adds `LayerState.PersistURL` with
`HasSwitchboard == (PersistURL != "")` pinned rather than assumed, and both
failure modes are silent, which is why it is pinned.

**Two of the three chipped zones are FILLED, neither needing a new pin field:**
`legend` from r52's `Mark.AxisLabel` (type axis only; it JOINS the count oracle,
and a type whose every event is hidden from a viewer is absent from their legend
— not even a zero), and `moongraph` from r53's `MonthGeometry.Almanac` (one lane
per drawn body, no composite ever, the ceiling still declared once in the
Nameplate). `horizon` keeps its chip and the chip is now **role-gated** — a
player who enables the key gets nothing at all, because `needs backend` is
GM-facing copy and `display:none` is not absence.

**The screenshot gate fired twice and both were real.** The sheet anchored to
the ⋯ covered the docked Ledger's own "Event list" row (a control hiding its own
target — re-anchored to the instrument); and HOST-P3's predicted std collision
happened with a real legend and a real graph, answered per CTS-8 inside the
Block's own std geometry with the body scrolling and **no layer key dropped**.

**DEF is still `["moons"]` and neither host seed gained a key** ([LYR-7] — the
HOST-P3/BENCH-P4 bookings close as SUPERSEDED, because the switchboard IS the
reachability they wanted). **ADR-048 gains §20-§24**, and there is no ADR-049.

**Split out under named follow-ons:** the Filters engine
(`C-CALV4-FILTERS-P10`), the colour-by picker and the legend's other two axes
(`C-CALV4-AXIS-P11`, blocked on DATA — no `Mark.OwnerLabel`, no per-calendar
label), the horizon data and the `Reveal through` write
(`C-CALV4-HORIZON-P12`, with its own security review), and the Tonight retarget
plus the `.sp2`/`.almgrid` ladder extension (`C-CALV4-ANSWER-EXT`, W-B's files).
`moonstyle=words` is deferred un-numbered: three of L20's four sky values reduce
to layer keys, and `words` names a register that exists nowhere in Chronicle.

### calendar-v4 — WAVE 3 · W-G PART A LANDED (2026-07-28)

**The operator's #1 feature is backed, and the slice began by retracting a
claim we had signed.** `C-CALV4-RSVP-P8` **Part A** filled the Bench's signed
RSVP panel (`cv4:2205-2263`) from storage that had shipped long before wave 1
said it had not. Wave 1's honesty copy asserted, in code and in the page
caption, that *"there is no session entity, no RSVP table and no per-member time
zone"* — **all three false**, contradicted by calendar migration 013, sessions
migrations 002 and 004, and core migration 000001. A `needs backend` chip that
is wrong is a fabricated ABSENCE and is worse than no chip, because it reads as
a verified finding. **ADR-048 §15** records the miss and the rule it produces:
a chip is a preflight FINDING, and a finding names the migration or the route it
checked.

What Part A ships: per-member availability lanes (owner / co-DM only), the
anonymous density row (everyone's), a **derived** recommended window under a
permanent `derived · not stored` chip with a quorum refusal below three
members, the party-visible member table with roles, zone chips, per-member
local clocks and answers, and a **live** RSVP trio on the session tile. Its
gate is `bench_rsvp_oracle_test.go` — no stored aggregate reaches the surface,
and a departed member's stored row changes no number.

**Zero new routes, zero migrations, zero JS.** `internal/wire/routes_snapshot.txt`
is byte-identical at 720 lines: the trio posts form-encoded to the existing
Player+ RSVP endpoint and the handler branches on `middleware.IsHTMX`.
`014_` in the calendar plugin was still free for W-F, which took it.

**ADR-048 gains §15-§19**: the preflight miss, the permission law (answers are
party-visible, lanes are not), the retired two-value role vocabulary and
`.badge.gm`'s third signed string `co-DM`, the zone-abbreviation ruling and the
zone-less clock, and the no-stored-aggregate rule.

**Still open in wave 3:** W-F (`C-CALV4-LAYERS-P9`) took the route lane after
P8 sealed and is **DONE** — see the section above. **W-G Part B** (`/campaigns/:id/schedule` — the Verdict, the
Director's matrix, the Roster, the Painter) is GATED on a coordinator drawing
pass and is NOT built; `C-CALV4-RSVP-P8B` ("the asking email") is booked for the
reminder endpoint the Nudge control still lacks.

### calendar-v4 — WAVE 2 COMPLETE (2026-07-28)

**The Block's four zones are all REAL.** Wave 1 (P0, PB, P1, P2, P5, P3, P4) is
merged; wave 2 is done — W-B filled the docked Ledger (`C-CALV4-LEDGER-P6`) and
W-E filled the Shelf (`C-CALV4-SHELF-P7`) with Upcoming, Filters and the
Almanac. W-F (the layer system, the per-viewer preference store, the legend, the
horizon, the moongraph — and now the Filters engine) is wave 3.

`.ai/decisions.md` **ADR-048** records the whole architecture decision in ONE
place, by design: the widget/producer split, the CSS-only controls, the four
honesty idioms, the one-pass count rule, the motion budget, and — as §10-§14
rather than a competing ADR — the Almanac, why the grid's three-moon ceiling is
only legitimate because it exists, and the two bounds wave 2 discovered (a
server-rendered `checked` cannot vary by container query; a primitive shared
across zones needs every rule that names it to name its zone too).

The pinned render contract has been amended three times under numbered
decisions: r51 (tie/mark emission), r52 (the Ledger row's three fields) and r53
(the celestial register, **uncapped by MoonCap** — the one sanctioned place a
host-passed parameter is deliberately non-authoritative for a zone).

Two things want a coordinator eye rather than a fix, both booked in
`.ai/todo.md`: the std tier is tighter than either earlier reading (a filled
Shelf wants 166px in a 520px Block), and `.ltabs` still carries `Month` alone
because [S9]'s four-tab construction is not buildable while selection is
CSS-only.

### Current release + branch state

- **Release line:** 0.0.1 (Release Readiness completed 2026-04-25 — backup scripts + mariadb-client in image + deployment runbook)
- **Active phase:** Pre-launch cleanup — locked macro-sequence: Calendar → Docs-audit → Tech-debt → Bugs → Features. The ~2026-06-26 target launch date has elapsed; pre-launch cleanup continues (no new date set). PC-Claim all 4 stages complete (Foundry PR #64 merged). Sidebar/nav consolidation (C-NAV-V3, both PRs) shipped as the last major structural arc; see `cordinator/plans/2026-05-21-master-plan.md` for the prior phase definition, current dispatch queue drives day-to-day priorities.
- **Coordinator artifacts (books, reports, dispatches):** live on the Cordinator repo's `main` branch — no dedicated working branch anymore.
- **LANDED (2026-07-28), branch `claude/coordinator-handoff-stage-3-3d3s4w`:**
  `C-CALV4-SEAM-P5` is **complete** — stages 1–15, one commit per stage, each
  green on the full `-short` suite. The durable deliverable is the **seam
  suite** (`internal/plugins/calendar/block_seam_test.go`): real fixtures
  projected through `projectBlock` and rendered through `calblock.Block`,
  asserted on the HTML. The discipline it encodes: assertions about what the
  PRODUCER chose to emit live producer-side; widget-side tests only pin
  renderer behaviour on hand-written `BlockData`.
  - Stages 1–3 (`18a84b8`/`ae9405d`/`71bfc99`): the amended **r51** pin (four
    identity fields `int64`→`string`, `Mark.Tied`,
    `MonthGeometry.MoonsDeclared`), the tie toggle as **ink, not membership**
    (measured cost: 1 byte), the calendar **identity triple** reaching the DOM.
  - Stages 4–13 closed everything the RESUME listed open: §3.3 the dm_only
    **dogear** (`Restricted` is the discriminator: false→notch,
    true→diamond), §3.4 the overlapping `MoreCount` (both foot-line sites),
    §3.5 `.band.half`'s stray border-right, §3.6 `needs backend` chips gated
    on their flags, **layer gating** (3 of 8 `LayerState` keys gated nothing;
    the widget fixtures were pinning the defect), §4 calendar-level
    visibility on `Block()`/`EventsForDay()` (a hidden calendar answers
    byte-identical to a missing one), §5 recurring-event dedupe at
    `candidateEvents`, §6 the lint scanner's templ-conditional blind spot +
    guard B4's `data-cell`/`data-row` subjects, §7 the three review fold-ins
    (timeline `visibility_override` folded into `EventCount`,
    `nextOccurrence`'s loop-invariant base hoisted,
    `EventsForEntityFiltered` promoted onto the `CalendarService`
    interface). Dated detail: `internal/plugins/calendar/.ai.md` +
    `internal/widgets/calendar_block/.ai.md`.
  - **Stage 15 (2026-07-28) closed the booked residual:** `buildMonthGeometry`
    populates `MonthGeometry.MoonsDeclared` from `Calendar.Moons` (hydrated by
    the block path's `MoonsForCalendars` batch read), and the Nameplate renders
    the r51 "3 of 4 moons" badge IFF the declared total exceeds the grid's
    `moonCap` ceiling (3) — a calendar declaring three or fewer states nothing
    extra, per the signed acceptance line. Seam-pinned through the real
    `BlockService.Block` path (`TestSeam_DeclaredMoonTotalReachesTheNameplate`)
    plus a widget-side render pin on hand-written `BlockData`.
  - `0c5f5f4` (same branch, not P5) `C-PERM-DMGRANT-REVOKE`: a removed co-DM
    kept owner-level visibility on **public** campaigns. Both the stored
    grant and the resolve-time gate are fixed. No migration.
- **LANDED (2026-07-28), same branch — `C-CALV4-HOST-P3` (wave 1, phase B):**
  the calendar-v4 Block now has its **first production caller**. Entity pages
  are the primary consumption surface (round-1 delta L3), so the entity calendar
  embed's month surface is `calendar_block.Block` projected through the P2
  spine, replacing the compact `adaptiveCalendarWidget`. Three commits.
  - `dcaaa4b` **the block host seam.** `BindingAffordanceFor` is the
    host-type-agnostic affordance (the old signature is unchanged and
    delegates; four widget blocks embed it), closing the deferred P3b gap where
    the picker query hardcoded `host_type=entity`. `BlockHost` gains
    `container-type: inline-size` **per widget type**
    (`DeclareInlineSizeHost`), not unconditionally — see the measured note
    below.
  - `fc2fe9c` **the tie toggle is live and pure CSS.** A hidden radio pair plus
    `:has()` changes the untied ink level; zero JavaScript (a `<script>` in an
    HTMX-swapped fragment never runs, `boot.js:163`) and zero routes. TWO ink
    levels now, per the signed `setTie()`: 0.28 in tied mode, 0.70 in whole —
    the sheet previously had one branch at the wrong level.
  - `0437fad` **the entity page hosts the Block.** Degrade ladder kept and
    extended: a calendar the viewer may not see renders **byte-identically** to
    no calendar at all (P5 stage 9's not-found shape, held at the host
    boundary). The linked-events list moved onto `EventsForEntityFiltered` —
    the loop it replaced applied only the event-level half of the filter. The
    entity page passes its own **layer set** (`moons, eras, weeknums, ledger,
    moongraph, shelf`), which is the mechanism the DEF ruling named; producer
    DEF stays `["moons"]`.
- **MEASURED, and it retracts a wave-1 assumption:**
  `container-type: inline-size` does **NOT** make an element a containing block
  for `position: fixed` / `position: absolute` descendants — `contain: layout`
  does. Chromium 141, three identical hosts: plain `(0,0)`, `container-type`
  `(0,0)`, `contain: layout` `(82,450)`. The C-CALV4-HOST-P3 dispatch warned
  the opposite (it would have trapped the maps block's `fixed inset-0` modals),
  and the warning reads correct from the spec text. Do not re-derive it from
  the spec; the reproduction is in
  `cordinator/reports/chronicle/screenshots/2026-07-28-c-calv4-host-p3/`.
- **`data.go` is byte-identical to the coordinator's pin and must never be
  `gofmt`-ed** — the pin is not gofmt-clean and formatting it breaks the
  match. Verify with `cmp` against `C-CALV4-BLOCKDATA.go.txt`.
- **Recent cross-cutting decisions** (most recent first):
  - 2026-07-24 — **Entity-tie visibility leak fix (C-CAL-ENTITY-TIES-LEAK-FIX, cordinator#32 gap #1 follow-up)**: `GET /campaigns/:id/calendars/:calId/events/:eid/entities` (RolePlayer-gated) returned every tied entity's name/type/icon/color with NO entity-visibility filtering — `EntitiesForEvent`/`EntitiesForEra` (`entity_ties_repository.go`) took only an ID, so the WHERE clause had nothing to gate on; a Player could read a dm_only/custom-restricted entity's NAME via any event or era it was tied to. The sibling `EntitiesForCalendar` (Calendars-dashboard associations panel, C-APPS-CAL-DASH-W1) was already hardened with `entityVisibilityFilter(role, userID)` — the original cordinator#32 audit missed these two. Fix: both methods now take `role, userID` and apply the same `entityVisibilityFilter`, entities re-aliased `e` (was `ent`) to match the filter's hardcoded column prefix. Threaded through `CalendarRepository`/`CalendarService` interfaces + `entity_ties_service.go`; `ListEventEntitiesAPI` sources the viewer context via `cc.VisibilityRole()` (co-DM/`IsDmGranted` counts as Owner) + `auth.GetUserID(c)`. `EntitiesForEra` has no production HTTP route yet — the signature change is forward-looking, no behavior change today. Updated the syncapi cross-plugin `CalendarService` test stub (`calendar_api_handler_test.go`) — the one other implementor, confirmed via `grep -r "EntitiesForEvent"`. **No live MariaDB in this sandbox** (confirmed: no docker daemon) for a literal query-level red→green repro; matched the test depth the precedent fix shipped with (fragment-level `TestEntityVisibilityFilter`, unmodified) plus new service-delegation tests (`TestEntitiesForEvent_ServiceDelegates`/`Era`) and a Player/Owner/co-DM matrix at the handler layer (`TestListEventEntitiesAPI_RespectsEntityVisibility`) + an equivalent service-layer matrix for era (`TestEntitiesForEra_VisibilityMatrix`), each driving a mock repo that models the real `role >= RoleOwner` threshold to prove the fix's plumbing end-to-end. `go build ./...` / `go test ./...` (45 packages, 0 fail) / `golangci-lint run ./...` all green. See `internal/plugins/calendar/.ai.md` §"Entity-tie visibility leak fix". Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first), §T-O1 (verify before claim — the no-live-DB gap is stated plainly, not papered over).
  - 2026-07-21 — **Owner-only field visibility (C-FIELDS-OWNER-FILTER)**: a player-visibility audit found that a claimed player character's fields (e.g. Draw Steel's `backstory`) were visible to the WHOLE party — Chronicle's only field-level tier was `GMOnly` (GM-tier vs. everyone, #517/audit M-1), with no way to keep a field private to (the claiming player + GM) while still showing the rest of the party that the character exists. Added the missing tier: `FieldDefinition.OwnerOnly` (`model.go`), enforced by the new `Entity.IsOwnedBy(userID)` + `FilterOwnerOnlyFields`/`FilterRestrictedFields` (`gm_fields.go`, composes with the existing `FilterGMOnlyFields` — a GM-only field stays hidden even from the entity's own owner; an owner-only field is shown to the owner but not to other players). Wired into every field-data egress point identified by the original M-1 audit: entities-plugin `GetFieldsAPI`/`PreviewAPI` (`handler.go`), `CharacterSurfaceSchemaJSON` (`character_surface.go` + `character_surface_block.templ`, threaded a `userID` through `BlockRenderContext`), and syncapi's `GetEntity`/`ListEntities` (`egress_sanitize.go`/`api_handler.go`, the same path Foundry sync AND the Draw Steel character-sheet widget's client-side fetch both use). Convergence for existing campaigns mirrors the GM-flag reconciler exactly: `EntityService.SyncFieldOwnerOnlyFlags` (refactored the shared per-type walk out of `SyncFieldGMFlags` into `syncFieldFlag`, both call it now), `systems.FieldDef.OwnerOnly` manifest annotation, `preset_applier.go`'s `buildOwnerOnlyFlagsByCategory`/`reconcileFieldOwnerOnlyFlags` run at boot and on package install/update alongside the existing GM ones. Also added `data-is-owner` to the manifest-renderer widget mount (`show_renderer_registry.go`, mirrors the existing `data-is-gm`) and threaded `UserID` onto `EntityShowRenderContext`/`EntityShowPage`, so a system-package widget can gate UI the same way the GM Lore box already does. **Draw Steel side** (companion repo): `manifest.json`'s `backstory` field now carries `"owner_only": true` (its `gm_notes`/director-notes sibling was already `gm_only`); `character-sheet.js`'s Background row is now scheduled only `if (data.isGm || data.isOwner)` — a teammate no longer sees a misleading "No backstory yet." placeholder for a hero that actually has one, they see no box at all (same rationale as the pre-existing GM Lore gate — a permission gate, not a data gate). Tests: new table-driven cases mirroring every existing GM-only test 1:1 (`gm_fields_test.go`, `gm_fields_handler_test.go`, `character_surface_test.go`, `gm_fields_egress_test.go`, `show_renderer_registry_test.go`) plus the Draw Steel `node --test tools/` suite (unaffected, still 162/162). `go build ./...` / `go test ./... -short` (whole repo) / `golangci-lint run` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first — server is the authority, not the widget's client-side box-hiding), §T-O4 (single shared filter/reconciler, extended not forked, so the two tiers can't drift).
  - 2026-07-19 — **3 residual JS-attribute XSS sinks closed (C-SEC-XSS-JSATTR-SWEEP-R1)**: the follow-up to #560's mandated 56-site sweep, which found these 3 and deliberately left them unfixed to hold scope (the other 53 are documented safe). All three interpolated free text into an Alpine `x-data` JS string literal unescaped; each now routes through a plugin-local `jsEsc` (re-declared per plugin, §T-B2, no cross-plugin import). **(1) entities/form.templ `parentSelector` (HIGH, cross-user, stored):** a parent entity's `Name` flowed into `selectedName: '%s'` — any Scribe+ renames an entity to `');<payload>//`, sets it as another entity's parent, and the child's edit form executes it for any member who opens it → wrapped in the entities plugin's existing `jsEsc`. **(2) sessions/sessions.templ `editSessionModal` (HIGH, cross-user, stored):** `RecurrenceType` flowed into `recType: '%s'`, and `UpdateSession` (`service.go`) never validated it (the JSON PUT binds the body unchecked) so the "enum" was attacker-writable → executes for every isScribe viewer. Fixed BOTH layers: `UpdateSession` now rejects any non-nil `RecurrenceType` outside `{weekly,biweekly,monthly,custom}` (mirrors CreateSession's switch but unconditional — the sink reads it regardless of `IsRecurring`), plus a new sessions-local `jsEsc` at the sink. **(3) campaigns/settings.templ `settingsGeneralTab` (LOW, self-XSS):** `SystemID` (`custom:<url>` remainder unvalidated) flowed into `_savedSystemId: '%s'`; owner-only to set AND to view, so injector == only viewer — sink hardened with a campaigns-local `jsEsc`, no new `custom:`-validation machinery (per the dispatch, not worth it for a self-XSS). Per-sink regression tests (`parent_selector_xss_test.go`, `recurrence_type_xss_test.go`, `system_id_xss_test.go`) assert the hostile `');alert(1)//` family renders backslash-escaped (`\&#39;`, survives templ's HTML round-trip) while legit values are unchanged, plus the service-level `UpdateSession`-refuses-unknown-type test. `make templ` / `go build ./...` / `go test ./internal/plugins/{entities,sessions,campaigns}/...` / `golangci-lint run` all green. No new routes, no migrations, no cross-plugin imports. **Overlap note:** #560 was still open at write time and also adds a byte-identical campaigns `jsEsc` at the same location (and prepends to this same list) — trivial keep-one-copy resolution when both land; this branch is self-contained and green on fresh `main`. Report: `cordinator reports/chronicle/2026-07-19-c-sec-xss-jsattr-sweep-r1.md`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security first — this *is* the tenet), §T-B2 (plugin isolation — `jsEsc` re-declared locally per plugin).
  - 2026-07-19 — **Reflected XSS on campaign Settings `?tab` closed (C-SEC-XSS-SETTINGS-TAB, audit SEC-1)**: the audit's lead T-B1 finding. `campaigns.Settings` read `?tab=` and passed it unvalidated into `CampaignSettingsPage`, which interpolated it into an Alpine `x-data` expression (`settings.templ` `fmt.Sprintf("{ tab: '%s' }", …)`). templ HTML-escapes the attribute, but the browser HTML-decodes it before Alpine `evaluate()`s it as JS, so `?tab=%27%29%3Balert(1)%2F%2F` (`');alert(1)//`) broke out of the string literal and ran attacker JS; CSP (`middleware/security.go`, `script-src 'unsafe-inline' 'unsafe-eval'` for Alpine) can't stop it, Settings is owner-gated (`routes.go` `RequireRole(RoleOwner)`), and the CSRF token is JS-readable (`csrf.go` `HttpOnly:false`) → owner-targeted account takeover from one crafted link. **Two-layer fix:** (1) `sanitizeSettingsTab(requested, tabs)` (`settings_tabs.go`) resolves `?tab=` against the already-role-filtered visible-tab set, falling back to the constant `"general"` on any empty/unknown/hostile value — an allowlist derived from the live `tabs` slice, so a viewer also can't pre-select a tab their role hides, and it removes the blank-tab-body side effect (the #5 seed: an unknown tab matched no `x-show`); (2) a local `jsEsc()` at the templ sink (mirrors entities' helper; re-declared locally to avoid a cross-plugin import, §T-B2) escapes `' \ \n \r` as defense in depth. Tests: `settings_tab_sanitize_test.go` (known tabs pass through; the exploit payload + 8 hostile/unknown inputs → `"general"`, asserting the sanitized value not the source text; a role-hidden tab is rejected for a player but reachable by an owner; `jsEsc` neutralizes the quote-breakout). `templ generate` + `go build ./...` + `go test ./internal/plugins/campaigns/...` + `go vet` green. Branch `claude/campaigns-settings-xss-fix-ynnr8v`. Report: `cordinator/reports/chronicle/2026-07-19-c-sec-xss-settings-tab.md`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (security-first — this fix *is* the tenet), §T-B2 (local helper, no cross-plugin coupling).
  - 2026-07-18 — **Restore drill kit (C-BACKUP-RESTORE-KIT)**: closes the beta plan's single largest data-loss risk (`cordinator/plans/2026-07-10-beta-transition-plan.md` §2 item 0.6) — a restore had never been verified, even once. **Step-0 finding:** backup/restore tooling already exists and is thorough (`scripts/backup.sh` + `scripts/restore.sh` + `make backup`/`make restore`/`make backup-list`, ADR-035/036/037) — no `backup-now.sh` companion needed, out of scope per the dispatch's own rule. **New:** `tools/restore-drill.sh` — one command, no flags for the happy path. Finds the newest backup via `docker compose exec` against the live `chronicle` app container (read-only `ls`/`cp`, same place `make backup-list` looks), spins up a throwaway MariaDB container (no host port, no shared network/volume, machine-generated name asserted against a reserved-name list so it can never collide with `chronicle`/`chronicle-db`/`chronicle-redis`), loads the dump, and checks: `schema_migrations` present/plausible (version vs. `ExpectedCoreMigrationVersion`, `dirty` must be 0), core tables (`users`/`campaigns`/`entities`) have rows (`calendar_events` checked too but a missing plugin table only warns, doesn't fail), and a spot FK check (`entities.campaign_id -> campaigns.id`, 0 orphans). Prints green `RESTORE DRILL: PASS` / red `FAIL: <why>`; a `trap`-based cleanup always removes the throwaway container. `--file <manifest-or-dump>` tests a specific backup instead (sha256-verified when a manifest is given). **Docs:** `docs/RESTORE-DRILL.md` (when to run, the one command, PASS/FAIL meaning, a FAIL-reason table, a clearly ⚠️-marked "restore FOR REAL" section pointing at the existing `scripts/restore.sh`/`make restore`) + cross-links added in `docs/deployment.md` §1/§8. **CI self-test:** `tools/test-restore-drill.sh` runs the real script against fixtures in `testdata/restore-drill/` (`good.sql` plus three FAIL fixtures — dirty migration flag, orphaned FK, empty core table — each proving its specific check actually fires, not just the happy path) via real disposable MariaDB containers; wired into `.github/workflows/ci.yml`'s `test` job (ubuntu-latest ships Docker). **Verification note (T-O1):** this sandbox's Docker registry blob fetches are blocked by org egress policy, so the full container lifecycle couldn't be pulled/run directly here; validated instead by (a) loading all four fixtures + running the exact verify queries against a locally-installed MariaDB 10.11, which caught a real bug — MariaDB's non-interactive client does not expand `\t` inside SQL string literals passed via `-e`, so `CONCAT(version,'\t',dirty)` round-tripped as the literal two characters `\`+`t`, not a tab byte, silently breaking the migrations-check parser; fixed by switching the delimiter to a literal `|`; and (b) a throwaway shim standing in for `docker run`/`exec`/`stop` (routed to the same local MariaDB) to exercise the real, unmodified script end-to-end — arg parsing, `--help`, manifest+sha256 verification (happy path and induced mismatch), missing-file/unknown-arg/no-compose-file preconditions, and all three FAIL fixtures — confirming exit codes and messages match design. CI (which has real registry access) is the first environment to run the real `docker run mariadb:latest` path — and it immediately caught what the local substitute couldn't: the real image ships only a `mariadb`-named client binary, not the `mysql` compat symlink an older locally-installed MariaDB 10.11 still carries, so every hardcoded `mysql -uroot ...` call failed with `env: 'mysql': No such file or directory`. Fixed by resolving the client binary once right after the healthcheck-ready loop (`mariadb` preferred, `mysql` fallback) and threading that through every subsequent call — exactly the kind of environment-specific gap this verification note exists to surface honestly rather than paper over. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-O1 (verify before claim — this entry states exactly what was and wasn't executed), §T-B1 (the drill never widens the live DB's attack surface: no host port, no shared network/volume, no live-service name reuse).
  - 2026-07-18 — **Embed event chips now navigate to V2 (C-CAL-EMBED-CHIPS-TAP, wave-8 #549 gate follow-up)**: the dashboard/entity-page calendar embed (`calendar.templ` `dayCell`) rendered event chips as `<button>` with `cursor-pointer` + a hover ring but no click handler anywhere in the embed context — the V1 scripts that once wired them were correctly deleted by #549 (C-CAL-V1-SUNSET), leaving a silently inert affordance. **Chose Option A (tap → navigate)**: chips are now `<a href>` links to `/campaigns/:id/calendar/v2/:calId?year=Y&month=M&day=D` (the SAME calendar the embed is bound to), cursored to the tapped day via the `year`/`month`/`day` query params `ShowV2` already parses (`handler_v2.go`) — no new route, `routes_snapshot.txt` byte-unchanged. Uses the day CELL's year/month/day, not the event's own (a recurring event's stored fields are its original creation date, not the occurrence being displayed). V2 surfaces and `event_grid.js` untouched — `calendarGrid`/`dayCell` have exactly one caller (the embed). Test: `embed_chip_nav_test.go` pins no-`<button>` + the exact href. `go test ./... -short` + `make test-js` (266 JS tests) + `golangci-lint run ./...` all green. See `internal/plugins/calendar/.ai.md` §"Embed event chips now navigate to V2". Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B3 (an inert button is the production-grade-UI smell this closes), §T-O2 (consumer-verified: read `handler_v2.go` ShowV2's query-param parsing before picking the href shape).
  - 2026-07-18 — **Timezone-list consolidation + two booked riders (C-TZ-CONSOLIDATION, RC-16.3)**: three hand-curated IANA timezone dropdown lists had drifted independently — `auth.commonTimezones()` (account settings) and `calendar.commonTimeZones()` (real-time calendar anchor) were byte-identical (the latter's own header comment already flagged the duplication as an intentional-for-now mirror pending a DRY pass), but `static/js/availability.js`'s `COMMON_TZ` (scheduler zone picker) carried a literal `"UTC"` entry neither Go list had. Consolidated to ONE canonical home: **`internal/timeutil/zones.go`** (`Zone{Value,Label}` + `CommonZones()`), the **UNION** of all three old lists (63 + `UTC` = 64 entries) — pinned by `zones_test.go` encoding the three old lists verbatim and asserting exact-union membership both directions, so no zone any surface previously offered can silently vanish. All three consumers now render from it: `account.templ` + `calendar_settings.templ` loop `timeutil.CommonZones()` directly (old `commonTimezones()`/`commonTimeZones()` deleted, `internal/plugins/calendar/timezones.go` removed entirely); `availability.js` no longer hand-rolls a list — it reads a `data-common-tz` JSON attribute (`timeutil.CommonZonesJSON()`) added to the availability page's EXISTING root div (`data-availability-root`, same surface as `data-tz`/`data-campaign-id` etc.) — **no new route**, `routes_snapshot.txt` unchanged (`internal/wire` test green). 4 new `test/js/availability_tz.test.mjs` cases pin the JS wiring (renders from the attribute, a zone outside it is never offered, missing/malformed attribute degrades to `[]` without throwing). **Rider 1 (view-pill accent, #541 deviation-3 booked follow-up):** the calendar V2 active view pill's App-accent fallback chain (`calendarV2ViewPill` → `var(--color-accent-app, var(--color-accent-surface-1, var(--color-accent, #6366f1)))`) turned out to have ALREADY shipped as a drive-by of an unrelated dispatch (commit `9d3b7954`, C-CAL-SKY-STRIP) — this PR adds the missing byte-identity-style pin test it should have shipped with (`calendar_v2_accent_pill_test.go`: active pill uses the App slot, inactive carries no accent style, markup is unchanged across unrelated context values). **Rider 2 (docs-only):** `internal/plugins/campaigns/.ai.md` "Campaign Customization" section — stale since C-ACCENT-TRIO/C-ACCENT-SLOTS, still described the pre-trio single-`accent_color`-column model — refreshed with the full 4-slot table (Site/Action/App/legacy-surface-pair), storage (`CampaignSettings` JSON fields, no migration), and the render/save mechanism. **r27 check (hour12:false ban):** grepped the touched surfaces + repo-wide; only pre-existing explanatory comments in `calendar_v2_rt_hint.test.mjs` (already-shipped h23 fix context) — no live violation, nothing to flag. `go build`/`go vet`/`go test ./...` + `make test-js` all green. Branch `claude/tz-list-consolidation-anwh99`. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-O1 (verified the old lists' exact contents via a diff script before writing the union, not from memory), §T-O4 (single source of truth — this IS the mechanism), §T-B4 (the `.ai.md` refresh).
  - 2026-07-18 — **Skybox multi-instance fix (C-SKYBOX-MULTI-INSTANCE, booked r23 from #543)**: a second simultaneous `skybox` widget mount (e.g. a dashboard block + the calendar sky pane open at once, both showing the same campaign) rendered STATIC — only its server-rendered first paint, no animation. Root cause verified via a failing-first headless repro (`test/js/skybox_multi_instance.test.mjs`, booted the REAL engine in a node vm with two independent `[data-cal-sky]` trees): `cal-almanac.js`'s engine was a page-wide singleton — `document.querySelector('[data-cal-sky]')`/`'[data-cal-worldstate]'` always resolve to the FIRST match, `SKY_SURFACE`/`SKY_FRONT`/`LAYER_CACHE` were single module-level vars, and `band.__calInited` made every init() call after the first a no-op — so a second band never got a `CalParticleEngine` surface at all. **Not** a boot.js double-mount issue (skybox.js's own widget mount/destroy lifecycle was already correct and untouched; the bug was entirely inside the shared engine's own DOM-binding). Fix: `SKY_BANDS` (array, one entry per live sky root) replaces the singletons; `bindSkyBands()`/`paintSkyBands()` bind + paint every `[data-cal-sky]` root (pairing each with its own canvas by document order — every `Skybox()` render emits exactly one canvas per root); `feedSkyEngine`/`effectLayersForCache` give each band its OWN `layerCache` (a meteor closure's live streak state is per-canvas, sharing it across differently-sized canvases would desync it); `renderDayPipeline`/`renderTimePipeline`/`applySunState`/`applyMoonDesigns`/`applyMoodTint`/`setBandSizeVars` all loop every live band instead of touching just the first. Still ONE shared `CalParticleEngine` rAF loop (bands just register additional surfaces into its existing list) and ONE `worldState` (per the #543 doctrine: engine reuse, one loop, per-instance layer state) — `SKY_SURFACE`/`SKY_FRONT`/`window.__calSkyEngine` stay pointed at the first-bound band for back-compat with existing single-band callers/tests. `init()`'s band-binding gate now distinguishes "engine already live + a new band mounted alongside it" (bind the newcomer, don't touch the survivor's worldState) from "every previously-bound band is gone" (a real boosted-nav swap — full re-teardown, unchanged from before). Pins (all in the new test file): both instances tick; destroying one doesn't stop the other; reduced-motion freezes both identically; the existing single-band swap + provider-self-destroy tests (`worldstate_reinit.test.mjs`, `skybox_widget.test.mjs`) are unchanged and still green. `make test-js` — 270/270 pass (266 pre-existing + 4 new). Widget JS only — no calendar `.templ`/route/migration changes (Go build not runnable in this sandbox — missing `templ` codegen tool, pre-existing environment gap unrelated to this diff). See `internal/widgets/skybox/.ai.md` (footgun #1 updated) for the mechanism writeup. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B3 (reduced-motion still honored per-instance), §T-O4 (single worldState/rAF-loop source of truth preserved, not forked per instance); `cordinator/dispatches/chronicle/C-SKYBOX-MULTI-INSTANCE.md`; Chronicle PR #543 (doctrine cited).
  - 2026-07-18 — **Sync chip beacon upgraded from "saw" to "applied" (C-SYNC-APPLIED-BEACON)**: #548 (below) proved only what Foundry last SAW via a GET; a fetch can still fail to apply. This dispatch adds the Chronicle half of the upgrade (a separate agent codes the Foundry-module confirm call against the contract below). **New endpoint:** `POST /calendar/date/confirm` (`calendar_api_handler.go` `ConfirmDate`) in the same syncapi Bearer group as `GET /calendar/date` (`RequirePermission(PermRead)`) — body `{year,month,day}` → 204. Same dual-auth route shape as GetCurrentDate, but since this endpoint's entire purpose IS the write (no calendar payload to still serve), a session-synthesized key gets a synchronous 403 rather than #548's silent fire-and-forget no-op. **Storage:** ONE new append-only idempotent migration, syncapi chain `006_calendar_date_beacon_applied` (`ADD COLUMN IF NOT EXISTS applied_year/month/day/applied_at` on the EXISTING `sync_calendar_date_beacons` row, not a new table — extends #548's per-campaign beacon). **Create-or-update:** `repository.go` `ConfirmCalendarDateBeacon` handles a confirm landing before any served-date GET (fresh install) via `INSERT ... ON DUPLICATE KEY UPDATE` that only touches `applied_*` on the update path; the insert branch fills the pre-existing NOT-NULL `last_served_*` columns with a `0/0/0` sentinel (same "unset" convention as `calendar_api_handler.go`'s `defaultIfZero` — a real month/day is always 1-31) rather than faking a served date. `GetCalendarSyncBeacon` (handler.go) now treats `Month == 0` as "never served" and omits the served fields from the response instead of surfacing `0000-00-00`. **Chip preference:** `/calendar-sync-beacon` response gains `last_applied_date`/`last_applied_at` (no second fetch — rides the same beacon call). `computeSyncChipState` (`calendar_v2_shell.js`) gains an optional 7th param `fmAppliedDate`; internally `confirmedDate = fmAppliedDate || fmConfirmedDate` — omitting the arg (every pre-existing caller) collapses to exactly the pre-upgrade behavior, the graceful-fallback pin. `wireSkyStrip` computes `fmAppliedDate` from the beacon response gated by the same `SKY_STRIP_STALE_AFTER_MS` freshness bar as the served date. `routes_snapshot.txt` +1 line (`UPDATE_ROUTES_SNAPSHOT=1`). Tests: Go handler auth-gate + Bearer-only pin (`calendar_confirm_date_handler_test.go`), service no-throttle pin (`calendar_confirm_date_test.go`), response applied-fields + never-served-sentinel-omitted pins (`calendar_sync_beacon_handler_test.go`), 9 new JS pure-function + `wireSkyStrip` integration tests in `calendar_v2_sky_strip.test.mjs` (applied-overrides-served, applied-drives-drift, stale-applied-ignored, graceful-fallback-byte-identical). `go test ./... -short` + `make test-js` (275 JS tests, +9 new) + `golangci-lint run ./...` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (the synthetic-session-key rejection is the security-critical piece — session callers never write), §T-B2 (extends the existing syncapi-owned endpoint, no new cross-plugin coupling), §T-O4 (single beacon row/response, not a parallel "applied" table).
  - 2026-07-17 — **Sync chip drift, wired (C-SYNC-DATE-BEACON, closes #545's flagged gap)**: #545 shipped the Calendaria sync chip's DRIFT state fully built and unit-tested but dormant — no source persisted what date Foundry last saw. This dispatch adds a **served-date beacon**: syncapi's `GET /calendar/date` (`calendar_api_handler.go` `GetCurrentDate`, the endpoint FM polls) now records `{last_served_year/month/day, last_served_at}` per campaign via `SyncAPIService.RecordCalendarDateBeacon` — named "served" not "applied"/"synced" (honest: what FM last SAW, never a claim about what it did). **Storage:** new syncapi-chain migration `005_calendar_date_beacon` (`sync_calendar_date_beacons`, PK `campaign_id`) — neither `api_keys` (per-key) nor `sync_mappings` (per-object) fit the per-campaign grain, so a new table was the correct call, not a column bolt-on. **Throttle:** write skipped when the existing beacon is <60s old AND the date is unchanged; a changed date always writes immediately regardless of age (`calendarDateBeaconThrottle`, service.go). Write itself is fire-and-forget on a background context (`recordCalendarDateBeaconIfModule`, matching the `UpdateKeyLastUsed`/`LogRequest` convention) so the GET hot path never blocks on the throttle SELECT + conditional UPSERT. **CRITICAL auth gate:** `GET /calendar/date` is dual-auth (`RequireAuthOrAPIKey` — session cookie OR Bearer key); only a REAL Bearer key (`key.ID != synthKeySessionID`) may beacon, so a member browsing Chronicle's own calendar over the session path never does — pinned by 3 handler tests. **Expose:** new syncapi-owned member-read endpoint `GET /campaigns/:id/calendar-sync-beacon` (`Handler.GetCalendarSyncBeacon`, no `RequireRole` — any campaign member, mirrors `foundry_vtt`'s `/foundry-presence`) rather than reaching into the `foundry-presence` payload — avoids a new foundry_vtt↔syncapi Go coupling neither plugin currently has (T-B2). `routes_snapshot.txt` +1 line (`UPDATE_ROUTES_SNAPSHOT=1`). **Chip:** `calendar_v2_shell.js`'s `wireSkyStrip` now fetches both `/foundry-presence` (connectivity) and the beacon (confirmed date) in parallel, gates the beacon by its OWN freshness (`SKY_STRIP_STALE_AFTER_MS`, same 15-min bar as presence staleness — an old beacon can't be trusted to represent what FM currently sees) before feeding `computeSyncChipState`'s previously-dormant `fmConfirmedDate` param; `data-cal-current-date` (new SSR'd attribute, `skyStripCurrentDateString` in `calendar_v2_sky_strip_helpers.go`, reads `cal.CurrentYear/Month/Day` — the same real-time-seamed fields `GetCurrentDate` serves — NOT the navigated view date) supplies `chronicleCurrentDate`. Beacon-fetch failure degrades independently (chip still paints from presence alone). Tests: Go throttle/auth-gate/endpoint unit tests (`calendar_date_beacon_test.go`, `calendar_date_beacon_handler_test.go`, `calendar_sync_beacon_handler_test.go`) + 6 new `wireSkyStrip` integration tests in `calendar_v2_sky_strip.test.mjs` driving the real fetch-and-paint chain end-to-end (drift/in-sync/stale-beacon-ignored/no-beacon/beacon-fetch-fails/presence-fetch-fails) — #545's dormant pure-function drift tests stay as-is (still valid) and are now backed by production reachability. `go test ./... -short` + `make test-js` (238 JS tests) + `golangci-lint run ./...` all green. Cites: `cordinator/decisions/2026-05-21-core-tenets.md` §T-B1 (auth-gate is the security-critical piece), §T-B2 (syncapi-owned endpoint over cross-plugin coupling), §T-O2 (consumer-verified: read #545's PR body + the actual `RequireAuthOrAPIKey`/`GetAPIKey` source before wiring).
  - 2026-07-12 — **Six arcs merged since the 2026-07-04 docs-drift audit (Chronicle #522–#534); this entry catches status.md up (C-DOC-DRIFT-REFRESH-R2).** Plugin count is unchanged at **24** — the two new feature arcs below (real-time calendar, availability scheduler) both landed inside their existing plugin dirs (`calendar`, `sessions`), not as new plugins.
    - **Real-time calendar (P1–P3, #526/#529/#533):** `reallife`-flagged calendars go computed-not-stored via an opt-in `tracks_real_time` seam; P2 added the owner enable flow + closed the P1 merge-gate findings; P3 fixed a leap-year weekday-drift bug in the shared grid math, hardened the real-time invariants, and added a live clock.
    - **Availability scheduler (P1–P2, #530/#534, C-SCHED):** new real-world session-scheduling surface inside the `sessions` plugin. P1 shipped recurring per-member availability (zone-local wall-clock, DST-correct via the new `internal/timeutil` package) + a DM density heatmap. P2 added slot proposals (UTC-instant options, own-tables responses/tokens/notifications — migration 003), per-option accept/decline/tentative, a topbar notification bell, and a month↔week calendar view. See `internal/plugins/sessions/.ai.md`.
    - **Sidebar/nav consolidation (C-NAV-V3, both PRs, #528/#532):** unified the sidebar onto one data model + one reorder mechanic (PR1); PR2 closed the PR1 gate findings (legacy custom-section preservation, a reconcile-failure write-window risk) and added a JS CI gate.
    - **Entity-visibility parity (#525):** four anon-reachable JSON widget endpoints (`GetEntry`, `GetFieldsAPI`, `PreviewAPI`, `GetAliasesAPI`) now honor custom per-entity visibility instead of the legacy default-mode-only check.
    - **Customization Hub rescue (#524, C-CUSTOMIZE-RESCUE):** fixed four real breaks in the owner Customization Hub, all rooted in the topbar controls (topbar brand name + a first-class Image mode).
    - **Systems ref-slug fixes (#527 + #531, C-SYSTEMS-REF-SLUG-FIX + R2):** `ReferenceItem` now normalizes its ID from `slug` at load time (blank IDs were breaking slug-keyed system data); R2 added a duplicate-ID guard, event aggregation, and ZIP-preview parity.
    - Also merged in this window: public-campaign view regression fixes (#522/#523), GM-only entity-field leak fix (#517, audit M-1), the Timeline Ledger as the calendar's 4th V2 view (#519/#520), and the surface-accent pair UI (#521, same day as the entry below).
    - **In flight now:** a four-agent wave — `C-SCHED-P3` (scheduler confirm-winner → session creation), `C-CAL-RT-RECUR-FIX` (real-time × recurrence interaction fix), `C-BETA-SEC-PACK` (beta security pass), plus a parallel Foundry-module lane.
  - 2026-07-07 — **Accent trio (C-ACCENT-TRIO rev 2 / cordinator design D14 rev)**: the single campaign accent becomes three slots. Slot 1 "Chrome" = the existing `accent_color` (header + nav + global interactive) — its CSS emission is BYTE-IDENTICAL to before, pinned by `TestAccentColorCSS_ChromeByteIdentical` against a legacy-formula oracle in `internal/templates/layouts/data_accent_test.go`. Slots 2+3 = the "surface pair" (`accent_surface_1/2` in `CampaignSettings`), consumed by themed content surfaces as primary/secondary via `--color-accent-surface-N{,-hover,-light,-rgb}` tokens emitted only when set (`accentSlotCSS`, one shared derivation — no forks); unset slots inherit chrome via `var(..., var(--color-accent))` fallback chains at consumers (zero CSS bytes). Save path EXTENDS `PUT /campaigns/:id/accent-color` with an optional `slot` form value (1|2) — no new route, `routes_snapshot.txt` untouched; service `UpdateAccentSurface` (load-merge-write, tested in `service_accent_test.go`). Hub: new "Surface Accents" card in the appearance tab (`surfaceAccentRow` ×2, direct-PUT precedent + local live preview) with a D19 sample character-header card. First consumer: the calendar V2 active view pill reads surface-1 (`calendar_v2.templ` `calendarV2ViewPill`). Character-page adoption is the named follow-up (the surface pair's primary consumer per the operator — lands with the entity/sheet theming pass). Branch `claude/calendar-audit-opus-prep-efr8r5`.
  - 2026-06-27 — **AI Workflow Support: data-tracing diagnostics (Phase 0 of the DS sheet-v3 arc)**: added 6 read-only diagnostics to the operator workspace catalog (`internal/systems/operator_diag.go` + new `operator_diag_entities.go`) to debug the "sheet renders blank / data not flowing" class. **`system.file-contains`** `<sys>:<relpath>:<markers>` — reads a served file (reusing the `fingerprintFiles` traversal clamp via new `clampedPath`/`readClampedFile`) and reports marker presence (live-build *content*, not just hash; closes the gap where I'd dropped to local `git show` for `playEntrance`). **`entity.fields`** `<camp>:<idOrSlug>` + **`entity.field-coverage`** `<camp>:<typeIdOrName>` — dump a hero's stored `fields_data` / declared-field population %, via injected `EntityDiagProvider` (impl `entityDiagAdapter` in new `internal/app/operator_diag_adapters.go`, wired like `SetInstalledPackagesProvider`; reads `entityService` GetByID/GetBySlug/List/GetEntityType*). **`sync.inbound`** `<entityId>` + **`sync.recent`** — what Foundry actually SENT, from a new bounded in-memory ring buffer in syncapi (`inbound_buffer.go`, captured in `UpdateEntityFields`; no DB/migration, mirrors the loader's DiagnosticEvents), wired via `SetSyncInboundProvider`. Together a **three-way trace** (sent→stored→declared) localizes where a value dies. All redacted/admin-gated/audited by the existing workspace; no new routes (wire snapshot unaffected). Tests: `operator_diag_entities_test.go`, `inbound_buffer_test.go`. Doc: `operator-diagnostics.md` catalog table. Branch `claude/chronicle-sheet-sync-j2m9s4`. **Next (Phase 1–3):** DS character-sheet **v3** (drop `hasX` gates → always-rendered scaffold + placeholders + responsive; new COMBAT/GM-LORE boxes + header actions) and findings-driven data/sync fixes — see plan `frolicking-foraging-candle.md`.
  - 2026-06-26 — **Operator AI Workspace (in-app diagnostics batch protocol)**: turned the operator diagnostics catalog into a single copy-paste round-trip with a **human-approval gate**, modeled on the campaign `ai_workspace` plugin's export→paste→review→commit flow but admin-scoped and read-only. New page `GET /admin/diagnostics/workspace` (`internal/plugins/admin/diagnostics_workspace.templ` + handler `DiagnosticsWorkspace`/`DiagnosticsWorkspaceParse`/`DiagnosticsWorkspaceRun`, routes in admin `routes.go`): **(1)** copy a machine-readable **functions list** (`systems.FunctionsSpecJSON` — every read-only diagnostic + the exact request shape) to feed an external AI; **(2)** paste back the ONE batch object the AI composes (`{v,note,full_dump,calls[]}`, fenced ```json accepted); **(3)** *Parse & review* validates against the live catalog and shows each call with unknown-name / full-dump flags — **nothing runs until the human clicks Approve**; **(4)** *Approve & run* executes the runnable read-only diagnostics and returns ONE compact, secret-redacted doc (manifest + results + byte-count footer) to copy back. Logic in new `internal/systems/operator_batch.go` (`Catalog`, `FunctionsSpecJSON`, `ParseBatch`→`BatchPlan`, `RunBatch`); added `FullDump bool` to `Diagnostic` (gates `system.health` — must set `full_dump:true` AND approve). Safety: bounded toolset (only catalog names run; 50-call cap; `DisallowUnknownFields`), **re-parse + re-derive runnability server-side on run** (never trusts a client plan), redaction belt-and-suspenders. Dashboard "Diagnostics" card repointed to the workspace; supersedes the standalone content-negotiated HTML console. Tests: `operator_batch_test.go` (parse/codefence/full-dump-gate/unknown/errors, manifest+footer, redaction). Wire snapshot updated for the 3 admin-gated routes (T-O2: admin-only read-only diagnostics UI). Doc: `docs/operator-diagnostics.md` §"The in-app AI Workspace". Branch `claude/chronicle-sheet-sync-j2m9s4`. **Hardening pass (parallel security + gap-analysis agents, same branch/PR #507):** security audit cleared ReDoS (RE2 linear), the full-dump gate (enforced in parse AND run), Templ XSS (all escaped), CSRF (global middleware + admin gate), and plan-tampering (run re-parses; trusts no client plan). Fixes applied: **(1) audit logging** — `DiagnosticsWorkspaceRun` now logs `admin.diagnostics_batch_run` via the already-wired `securityService.LogEvent` (Option A: site-admin event w/ actor+IP+UA, counts-only — the campaign `audit.Log` path rejects empty CampaignID so it doesn't fit; new event const/label/icon in `security_model.go`). **(2) output/CPU amplifier (MED)** — `RunBatch` does one authoritative classify+**dedup** pass (identical `(name,arg)` runs once → kills the 50×-rehash vector) and caps assembled output at 256 KB w/ truncation notice. **(3)** 64 KB paste cap + `sanitizeInline` on note/name/arg echoed into the result markdown (LOW manifest-integrity). Footer now reports bytes + ~tokens. Tests added: dedup (parse+run), byte cap, sanitize, note-sanitized. Deferred: per-route rate-limiting (spec §C2; low priority behind admin gate + bounded/deduped/capped toolset).
  - 2026-06-26 — **Operator-diagnostics: deploy/serve diagnostics batch**: three new named diagnostics + three SQL probes in `internal/systems/operator_diag.go`. **`packages.installed-vs-loaded`** — the smoking-gun check: compares each installed system package's DB version to what the loader actually serves (matched by install-path prefix → sidesteps slug-vs-manifest-id), flagging `NOT loaded` (registry never picked up the install) and version `MISMATCH`. Fed by a new injected `SetInstalledPackagesProvider` (dependency inversion; wired in `routes.go` from `pkgService.ListPackages`, system packages only). **`packages.on-disk-versions`** — lists every on-disk version folder per package tagging `[installed-db]`/`[LOADED]` to expose a shadowing leftover. **`systems.load-events`** — renders the loader's `DiagnosticEvents()` (discovered/skipped/failed, incl. WS-6 EventSkipped reasons). Probes added: `packages-db-state`, `entity-type-tree`, `sync-mapping-orphans` (SQL, paste-back). Tests: render logic for all three (NOT-loaded / MISMATCH / OK, load-events skip+reason) reusing the `withLoadedSystems` fixture; `go test -race` green. Doc tables updated. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Operator-diagnostics hardening + docs (parallel-agent audit pass)**: a security/correctness audit of the new `internal/systems` diagnostics surfaced three real issues, all fixed: **(1) path traversal (HIGH)** — `fingerprintFiles` joined `dir`+manifest widget paths without clamping, so a hostile manifest `script_file:"../../etc/passwd"` could be stat/read; now `filepath.Clean`+`HasPrefix` clamp to the system dir (mirrors `WidgetScriptAPI`). **(2) unbounded read (MED)** — added an 8 MiB cap (`maxFingerprintBytes`); oversized files report `too-large` instead of being hashed (OOM guard). **(3) data race (MED)** — `globalLoader.modules`/`systemInstances` were accessed without sync while package installs re-`register()` concurrently; added `sync.RWMutex` to `SystemLoader` (writes in `register`/`RegisterSystem`, RLock in `Get/Dir/All/Count/GetSystem/AllSystems/Health`; `Health` snapshots under RLock then does disk I/O unlocked). Also widened `redactSecrets` to catch space-separated `Bearer <token>`. WS-5 `mergeNewFields` + WS-6 `preferCandidate` audited → PASS. `go test -race ./internal/systems` green. New: `docs/operator-diagnostics.md` (full feature doc) + `operator_diag_extra_test.go` (handler-level httptest, redaction/fingerprint edge cases) + traversal/size-cap tests. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Extensions deployment-health diagnostic (read-only admin)**: `GET /admin/extensions/health` (`internal/systems/health.go` + `handler.go` `ExtensionsHealthAPI`, wired on the admin group in `routes.go`) returns, per LOADED system, the version + on-disk dir the loader actually serves from (`Dir(id)`, `source`) plus a content fingerprint (size + sha256[:16] + mtime) of each widget/manifest file. Purpose: diagnose the "Admin▸Packages reports version X but the stale file still renders" class of bug from the UI without host/SSH access — if `loaded_version` ≠ the installed version the in-memory registry never picked up the install; if it matches but a file hash is the old content the extraction is wrong. Read-only by construction (only stats/hashes files the loader already serves). Tests: `health_test.go` (fingerprint exists/size/hash/missing, content-sensitivity, dedupe, loader assembly). First slice of the operator-facing "extensions health" surface. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **Character-fields API: `foundry_item_single` (WS-3 single-item collections)**: added `FoundryItemSingle` to `FieldDef` + `CharacterFieldExport`, carried through both `CharacterFieldsForAPI`/`ItemFieldsForAPI` (`internal/systems/manifest.go`). Lets a system map an "exactly one X item" field (a hero's class/ancestry/kit/subclass, which live as embedded Foundry items, not a `system.*` path) to that item's name as a scalar string — the Foundry generic adapter collapses the collection when the flag is set. Mirrors the existing `foundry_collection`/`foundry_item_type`/`foundry_item_fields` passthrough (87c3bf7). Test: `manifest_test.go` `…_CollectionAnnotations`. Consumed by Chronicle-Draw-Steel's manifest (class/ancestry/kit/subclass) + Chronicle-Foundry-Module's adapter. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-26 — **System loader: version-aware duplicate resolution (WS-6; no silent downgrade)**: the loader keyed `modules[manifest.ID]` with unconditional last-wins, so two dirs declaring the same system ID (e.g. a leftover `<slug>-1`) resolved by scan order — a stale copy could shadow the current one. New `preferCandidate`/`register` (`internal/systems/loader.go`) apply a policy in BOTH discovery paths (`DiscoverAll` bundled + `loadSingleSystem` package): highest `version` wins (numeric `versionLess`), equal-version package overlays bundled (the intended override), and an older/stale duplicate is **skipped, not loaded** — gated so it also can't re-enter via `systemInstances` instantiation. Skips emit a new `EventSkipped` (`event_log.go`) so they surface in admin diagnostics instead of vanishing. Tests: `loader_dedup_test.go` (newest-wins regardless of scan order, stale-doesn't-shadow, `preferCandidate` table). The package-manager slug scan (`registry.go`) already picked the newest version *subdir*; this closes the same hazard at the ID-collision level. Branch `claude/chronicle-sheet-sync-j2m9s4`. **Remaining WS-6 piece:** the admin "Extensions Health — installed vs loaded" view (UI; surfaces the skip events) — deferred (needs a live look).
  - 2026-06-26 — **Entity-type field reconcile in place (WS-5; closes PC-PRESET-FIELDS)**: `ApplySystemPresets` (`internal/app/preset_applier.go`) no longer skips an existing type — it indexes existing types by preset-category then name and, on a match, calls the new **`entities.ReconcileEntityTypeFields(typeID, declared)`** to additively backfill any declared schema field the type is missing (else creates as before). The merge is the pure, unit-tested `mergeNewFields` (append-missing-by-Key; never removes/reorders/overwrites → idempotent, safe on every system enable/update). This fills EXISTING heroes' type schema (e.g. Draw Steel Tyne/Orrin/Saatraaol gain backstory/abilities/etc.) **without recreating the type** (which would orphan claimed entities). Type schema only — never entity data; trigger is the existing enable path (fires on package install/update). Tests: `internal/plugins/entities/reconcile_fields_test.go`. Branch `claude/chronicle-sheet-sync-j2m9s4`.
  - 2026-06-24 — **Migration robustness (post-`000030` incident; ADR-045)**: deleting an applied migration crash-looped prod. Durable fix in three layers. **Shipped in two PRs** (the original PR #498 had already merged at the pre-restore commit, stranding the fix — see ADR-044 incident-response lesson): the restore + runtime + CI guards landed on `main` via hotfix **PR #499**; the admin **visibility** layer (the unified Database page) follows in a second PR off `claude/gallant-johnson-kd574n`. **(1) Runtime** (`internal/database/migrate_state.go` `MigrateWithBackup`, replacing the unconditional backup→migrate in `cmd/server/main.go`): back up ONLY when a migration is pending (ends the every-restart backup storm); **DB AHEAD of build → log + boot anyway** (fixes the deletion case AND ordinary image rollbacks; additive-migration assumption, backstopped by health checks); **dirty DB fails fast** with restore guidance (the old `Force(v-1)` looped forever on non-idempotent DDL); `fatalBoot` sleeps `BOOT_FAIL_BACKOFF` (45s) so unrecoverable boots don't hot-loop. **(2) CI guards** (`internal/database/migrate_test.go` + `tools/check-migration-immutability.sh`): immutability (no delete/edit of an applied migration — the guard that blocks the incident), version-pin (`ExpectedCoreMigrationVersion == max`; fixed the live 29-vs-30 drift), idempotent-DDL lint (31 historical files grandfathered), gapless numbering, plugin up/down pairs. **(3) Admin visibility — unified tabbed `/admin/database` page** (Migrations · Health · Backups · Schema, behind an at-a-glance status strip): **Migrations** = core "Core schema" card (version/dirty/pending + DB-ahead banner) + per-plugin grid + history; **Health** = the SAME `RunStartupHealthChecks` boot runs, surfaced live (pass/warn/fail pills) + `GET /admin/database/status` JSON for monitoring — `database.RunHealthChecks` split out (structured result, no logging/exit) and the boot config extracted to `app.StartupHealthCheckConfig` so boot + admin never disagree; **Backups** = artifacts with an Auto (pre-migration) vs Manual badge, restorable snapshots w/ Chronicle+schema versions, last-auto-backup recency, and create/download/restore actions reusing the existing `backup`/`restore` plugin flows (no new engine); **Schema** = the D3 diagram, lazily mounted on first tab activation so it reads a real width. Admin stays decoupled: it defines `HealthChecker`/`BackupLister` interfaces, the app layer injects adapters (`internal/app/admin_db_adapters.go`). **Policy:** migrations are APPEND-ONLY + SCHEMA-ONLY; one-time DATA fixes are idempotent reconcilers (the `app/setup_pc.go` pattern), never migrations. Docs: ADR-045, CLAUDE.md §Migrations, `.ai/conventions.md` §Migration Safety Rules #8-11, `docs/deployment.md` §7.
  - 2026-06-24 — **Extension settings framework + owner-driven PC reconciliation (ADDITIVE to the retained migration)**: the prod duplicate (a generic "Player Characters" holding the characters next to Draw Steel's empty "Heroes" `drawsteel-character`) is auto-merged on deploy by the **retained** one-time guarded migration `000030` (unambiguous case), now COMPLEMENTED by an owner-driven path for the cases it skips. ⚠️ **PROD INCIDENT + lesson:** an initial draft DELETED `000030` and its test — that **crash-looped production** on boot (`no migration found for version 30: read down for version 30: file does not exist`; golang-migrate's `file://` source must contain every version up to the DB's recorded version). Reverted (`9990134`); **a migration any live DB has applied can never be deleted.** Each extension gets a reusable **settings/onboarding page** (overlay or full page) driven by a slug-keyed `SetupProvider` registry in the `addons` plugin (wired from `internal/app/routes.go` like `PresetApplier`; state in `campaign_addons.config_json.setup`, no migration). Enabling an addon nudges the owner (existing `extensions-hub-refresh` HX-Trigger + a `chronicle:notify` toast) and surfaces a "Setup" badge on the Extensions card. First provider = player-character setup: detects existing PC entities / sub-categories / the duplicate, asks "use the system's name ('Heroes') or your own?", and **merges on demand** via the new owner-triggered, single-campaign `entities.MergeDuplicatePlayerCharacterType` (+ repo `MoveEntitiesAndDeleteType`, one tx; human-readable `apperror` when ambiguous — this also closes PC-DUP-GUARD-2). Enabling stays safe (idempotent `EnsurePlayerCharacterType` still runs). **Kept** from the prior arc: the `nest` closure in `EnsurePlayerCharacterType` and the dead palette-filter removal. Branch `claude/gallant-johnson-kd574n`. Plan: `/root/.claude/plans/drifting-jingling-boole.md`. **Still deferred:** `ApplySystemPresets` drops `preset.Fields` (PC-PRESET-FIELDS).
  - 2026-06-23 — **PC-sheet system-binding (dynamic-surface arc, 4 shipped seams)**: a system pack now fills Chronicle's player-character category with **zero core changes** — the modularity seam for the dynamic surface. Design: Cordinator `plans/2026-06-23-pc-sheet-system-binding-design.md` (§11 is the binding shape — minimal-touch). Shipped (4 commits): **(1) renderer-by-preset** — `RendererDef.PresetCategory` (`internal/systems/manifest.go`); a renderer binds by entity-type slug **XOR** `preset_category` (validated against the manifest's own `entity_presets[].category`); the registry gains a `presetRenderers` map (`show_renderer_registry.go`) and `lookupEntityShowRenderer` resolves **slug-first-then-preset** so a pack's own slug-bound type is never shadowed; `registerManifestRenderers` (`routes.go`) routes each entry to the right map. **(2) nest the system char type** — `EnsurePlayerCharacterType` Case 2 now re-parents a system's own character type (e.g. `drawsteel-character`, detected generically via `isClaimableType`) under "Characters" as the PC sub-category — **no rename** (preserve the system's terms), **no field copy** (it already carries them + renders via its own slug renderer); no generic type is made. System-less campaigns still get the generic "Player Characters" (Case 3). **(3) duplicate guard** — `CreateEntityType` rejects a manual second PC type (one per preset-category) with a `Conflict` pointing at the addon. **(4) page-renderer ≠ palette block** — `GetSystemWidgetBlockMetas` (`internal/systems/handler.go`) excludes any widget that's also a `renderers[].widget`, killing the "drop the sheet into a layout → bare name" trap (principled rule replacing the earlier slug-string filter `9a1e4d6`). No DB change (manifest JSON only). Field-adoption / rename / `CreateEntityTypeInput.Fields` from the design's earlier drafts were **dropped** by §11. Docs: `systems/.ai.md` §Renderers, `entities/.ai.md` §`EnsurePlayerCharacterType`, `docs/system-package-rendering.md` §Player-character sheet. *Next:* Draw Steel `0.0.10` (optional `preset_category:"character"` renderer; not required since its slug renderer already covers its type); operator's prod-campaign stray-duplicate migration (separate from the boot path).
  - 2026-06-22 — **Dynamic-surface frame (Wave 1) + live demo**: `Chronicle.surface` (`static/js/widgets/dynamic_surface.js`; see its `.ai.md`) is the system-agnostic frame for dynamic sheets — a motion-preset menu, an overlay stack, an expand/collapse box, a memoized data provider, a mini→full launch, and a schema-driven `mount`. The admin **Design Lab** (`/admin/design-lab`) was repurposed from the old static component catalogue into the **surface demo**: a live, seeded character sheet, with `static/js/widgets/surface_demo.js` standing in as a sample "System" that registers box renderers + mounts the schema. Backend support: `entities.UpdateFields` now broadcasts an `updated` event (GAP-1, PR #490) so field pushes reach live web/Foundry consumers; a stored sync-key resolves to Owner visibility with a loud degrade signal (§1B, PR #489). Design: Cordinator `plans/2026-06-21-dynamic-widget-ui-framework-design.md` + the `2026-06-21-*-vision` / `-responsibilities-*` set. **First production adopter:** the new per-campaign **Characters ("Cast") page** (`GET /campaigns/:id/characters`) — the party (claimed PCs, viewer's own highlighted) + active NPCs (GM-curated `cast` tag), each card linking to the entity page and launching a mini→full preview via the frame. New `service.ListClaimed`; pure `assembleCastParty` (tested); per-plugin `embed.FS` for `characters.js`; sidebar "Characters" link. Zero migration. See `entities/.ai.md` §"Characters Page" + Cordinator `plans/2026-06-22-characters-cast-page-design.md`. **Second adopter:** the surface is now a real layout **block** (`character_surface`, editable in the layout editor) and the **default page for player-character types** (`CharacterLayout()`) — a seeded "big widget" sheet (portrait + fields + a description box that reuses the role-aware `editor` widget). Frame fix: `Provider.push` no longer clobbered by `load()`. See `entities/.ai.md` §`character_surface`. The Characters page is now **addon-gated + consolidated**: a Players section (`player-character-claiming`) + an NPCs/Monsters section contributed by the `npcs` plugin via an injected `NPCSectionProvider` (featured tag-row + revealed list + DM reveal/hide); page 404s if neither addon is on, and the old `/npcs` gallery redirects in. Enabling the `player-character-claiming` addon now **auto-premakes** the claimable "Player Characters" type (with the `character_surface` layout) via `PresetApplier.ApplyAddonEnableEffects` → `EnsurePlayerCharacterType` (idempotent); a **one-time idempotent startup backfill** (`internal/app/backfill.go`, wired in `routes.go` after the preset applier) replays that effect across campaigns that enabled the addon *before* it shipped — new `addons.ListCampaignsUsingAddon` + the existing service method, no SQL hand-rolling, safe every boot. Reference fix: client reference-renderers now resolve via `GET /campaigns/:id/systems/:mod/rules-glossary` (`SystemHandler.RulesGlossaryAPI`, serves the system's raw `data/rules-glossary.json` preserving authored `slug`). **Third adopter (shipped, Chronicle-Draw-Steel):** the Draw Steel `character-sheet` widget was refactored onto `Chronicle.surface` — 11 `ds-*` box renderers, schema omits empty boxes, headless identity banner, ability cards open a power-roll overlay; mount contract unchanged. **All of the above is migration-safe** (zero schema/`.sql` changes since the last green `main`; the one new query reads existing columns). **Fourth adopter (Flagship #2 — Rulebook) — PARKED** (built then removed from the build at the operator's request — "still a travesty"; code recoverable at commit `bbe6508` for a later rework): the Chronicle-core widget `static/js/widgets/rulebook.js` (`data-widget="rulebook"`; see its `.ai.md`) turns a system's `data/rules-glossary.json` into an interactive surface — a category nav + expanding rule boxes, debounced cross-category search, and stackable `{@term}` cross-ref overlays — reusing the frame for chrome/motion/overlays. Mounted above the category grid in `SystemIndexContent`; degrades invisibly with no glossary, so zero backend/route changes. **PC sub-category migration (prod):** `EnsurePlayerCharacterType` now ensures the claimable "Player Character" type is **nested under the default "Characters" category** (`ParentTypeID`) and **re-parents a stray top-level one** an earlier build premade — via `UpdateEntityType`, preserving claimed characters + layout; the startup backfill replays it, so deploying heals prod in place (the ADR-039 shape; no manual delete/SQL). Next: browser-verify the DS sheet + the prod PR to green `main`.
  - 2026-06-19 — **Player Character Claiming ALL 4 STAGES COMPLETE (Chronicle PR #480/#481/#482 + gate-enforce; Foundry-module PR #64)**: audit visibility + database layer + UI (claim banner, owner roster, per-type toggle) + Foundry actor-sync addon-aware PC sub-type routing/claiming. Security pass: `.ai/security-audit-2026-06-19-pc-claiming.md`. See `.ai/decisions.md` (ADR-039), `docs/player-character-claiming.md` (operator guide), `entities/.ai.md` §"Player Character Claiming". Cross-repo coordination record: `cordinator/decisions/2026-06-19-pc-claim-design.md` + `cordinator/plans/2026-06-19-pc-claim-arc.md`.
  - 2026-06-11 — **Worldstate/GM overhaul arc COMPLETE; calendar code-complete** pending the operator's final visual pass. Landed post-#443: GM console r3 single-writer state machine (#456), day mini-view (#457), drawer action set incl. cross-plugin `EntityCreator` seam (#458), fragment-401 error-handler class fix (#459), demo consolidation (#460), recurrence via single `Event.OccursOn` predicate + additive `weatherDate` on the world-state PUT (#461). Next arcs queued in `cordinator/plans/ACTIVE.md` (2026-06-11 block): sweep fixes in flight, Puzzles arc designed (`cordinator/plans/2026-06-11-puzzles-arc.md`), Timeline V2 awaiting design pick.
  - chronicle#442 (+ #443 r2) — worldstate render overhaul: cal-almanac-render.css render/chrome CSS split, back+front sky canvases with layered SKY_FX, strip+full-band-sheets GM console. See `internal/plugins/calendar/.ai.md` §"Worldstate render architecture (2026-06)".
  - `cordinator/decisions/2026-05-26-chronicle-production-safety-system.md` — `RunStartupHealthChecks` rubric + docker-unavailable substitute pattern
  - `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md` — AI export pipeline design (future scope, scoping decisions locked)
  - `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md` — Draw Steel game system spin-up strategy (own security audit first)
  - `cordinator/decisions/2026-05-25-plugin-static-assets.md` — per-plugin `embed.FS` static-asset ownership (NW-2.2 Chunk F)
  - `cordinator/decisions/2026-05-23-plugin-registration.md` — lightweight `PluginRegistration` registry (NW-2.2 Chunk A)
  - `cordinator/decisions/2026-05-22-loadDescriptor-fallback.md` — Chronicle/Foundry-Module descriptor wire pin (locked, used by `internal/plugins/foundry_vtt/descriptor_fallback_test.go`)

### Bootstrap reads for a new session

In order:

1. `cordinator/decisions/2026-05-21-core-tenets.md` — binding tenets every session honors
2. `cordinator/decisions/2026-05-19-dispatch-workflow.md` — how dispatches + status reports flow
3. `cordinator/decisions/2026-05-23-decision-routing.md` — backend-vs-frontend question routing
4. This file (you're here) — high-level state + plugin index
5. `.ai/conventions.md` — code patterns
6. `.ai/architecture.md` — three-tier extension model + request flow
7. The relevant plugin's `.ai.md` if your work is plugin-scoped (see index below)

### Plugin .ai.md index (the canonical per-plugin docs)

#### Plugins (24)

| Plugin | `.ai.md` |
|---|---|
| addons | [internal/plugins/addons/.ai.md](../internal/plugins/addons/.ai.md) |
| admin | [internal/plugins/admin/.ai.md](../internal/plugins/admin/.ai.md) |
| ai_workspace | [internal/plugins/ai_workspace/.ai.md](../internal/plugins/ai_workspace/.ai.md) |
| armory | [internal/plugins/armory/.ai.md](../internal/plugins/armory/.ai.md) |
| audit | [internal/plugins/audit/.ai.md](../internal/plugins/audit/.ai.md) |
| auth | [internal/plugins/auth/.ai.md](../internal/plugins/auth/.ai.md) |
| backup | [internal/plugins/backup/.ai.md](../internal/plugins/backup/.ai.md) |
| bestiary | [internal/plugins/bestiary/.ai.md](../internal/plugins/bestiary/.ai.md) |
| calendar | [internal/plugins/calendar/.ai.md](../internal/plugins/calendar/.ai.md) |
| campaigns | [internal/plugins/campaigns/.ai.md](../internal/plugins/campaigns/.ai.md) |
| designlab | [internal/plugins/designlab/.ai.md](../internal/plugins/designlab/.ai.md) |
| entities | [internal/plugins/entities/.ai.md](../internal/plugins/entities/.ai.md) |
| foundry_vtt | [internal/plugins/foundry_vtt/.ai.md](../internal/plugins/foundry_vtt/.ai.md) |
| maps | [internal/plugins/maps/.ai.md](../internal/plugins/maps/.ai.md) |
| media | [internal/plugins/media/.ai.md](../internal/plugins/media/.ai.md) |
| npcs | [internal/plugins/npcs/.ai.md](../internal/plugins/npcs/.ai.md) |
| packages | [internal/plugins/packages/.ai.md](../internal/plugins/packages/.ai.md) |
| restore | [internal/plugins/restore/.ai.md](../internal/plugins/restore/.ai.md) |
| sessions | [internal/plugins/sessions/.ai.md](../internal/plugins/sessions/.ai.md) |
| settings | [internal/plugins/settings/.ai.md](../internal/plugins/settings/.ai.md) |
| smtp | [internal/plugins/smtp/.ai.md](../internal/plugins/smtp/.ai.md) |
| syncapi | [internal/plugins/syncapi/.ai.md](../internal/plugins/syncapi/.ai.md) |
| timeline | [internal/plugins/timeline/.ai.md](../internal/plugins/timeline/.ai.md) |
| widgetbindings | [internal/plugins/widgetbindings/.ai.md](../internal/plugins/widgetbindings/.ai.md) |

#### Widgets (11 of 11)

| Widget | `.ai.md` |
|---|---|
| attributes | [internal/widgets/attributes/.ai.md](../internal/widgets/attributes/.ai.md) |
| calendar_block | [internal/widgets/calendar_block/.ai.md](../internal/widgets/calendar_block/.ai.md) |
| calendar_v2 | [internal/widgets/calendar_v2/.ai.md](../internal/widgets/calendar_v2/.ai.md) — **READ-ONLY for the calendar-v4 wave** |
| editor | [internal/widgets/editor/.ai.md](../internal/widgets/editor/.ai.md) |
| entity_notes | [internal/widgets/entity_notes/.ai.md](../internal/widgets/entity_notes/.ai.md) |
| mentions | [internal/widgets/mentions/.ai.md](../internal/widgets/mentions/.ai.md) |
| notes | [internal/widgets/notes/.ai.md](../internal/widgets/notes/.ai.md) |
| posts | [internal/widgets/posts/.ai.md](../internal/widgets/posts/.ai.md) |
| relations | [internal/widgets/relations/.ai.md](../internal/widgets/relations/.ai.md) |
| tags | [internal/widgets/tags/.ai.md](../internal/widgets/tags/.ai.md) |
| title | [internal/widgets/title/.ai.md](../internal/widgets/title/.ai.md) |

`calendar_block` (new, `internal/widgets/calendar_block/`) currently holds only
the pinned cross-slice contract `data.go` + its reflection shape pin. Its
`.ai.md` lands with the renderer in C-CALV4-BLOCK-P1.

### Cross-cutting state (not plugin-scoped)

#### 2026-08-07 — C-SWEEP-R4 stages 26–28 (the review's own findings, closed)

The C-SWEEP-R4 adversarial review returned pass=false on ONE blocking item and
one genuine test gap. Both are closed here, plus the bookkeeping the review
flagged. Nothing in R4's substance was found false.

**Stage 26 — the branch was failing its own CI.** Stage 16's ImportReport
threading introduced four `report.Fail("calendar", …)` / `("timelines",
"timeline", …)` labels outside the owning plugin directories, so
`tools/check-plugin-isolation.sh` (T-B2 / M-B2.1) went red at stage 16 and
stayed red for nine commits, including HEAD. It runs in `.github/workflows/
ci.yml` and `make verify`. No commit claimed it passed — it simply was not run.
The labels now route through constants in
`internal/plugins/campaigns/import_report.go`, and guard **amendment R4-S26-A**
adds `const_registry_files`: exact-path matching, exempting only a bare
`Name = "slug"` declaration that is the whole line. That is strictly narrower
than the `always_allowed_prefixes` entry the guard itself suggests, which would
have exempted the entire file forever including any call site later added to it.
`tools/test-plugin-isolation.sh` (new, 7 cases, wired into CI and `make verify`)
mutation-tests the narrowness in both directions.

**Stage 27 — the preview door was fixed but unpinned.** Stage 24 applied the
same four-field fix in `builderImportResult` (commit path) and `draftCalendar`
(preview path) and tested only the first; reverting `draftCalendar` to
24/60/60/0 left the whole calendar suite green. `draftCalendar` feeds
`builderPreviewBlock` and `builderMoonAlmanac`, so an unpinned regression there
shows a preview whose leap years and moon phases disagree with the calendar
about to be created. `TestBuilderDoorIsNotLossierThanTheImporter` now runs the
preview leg too (and asserts the fixture can fail), and
`TestBuilderPreviewAndCreateAgree` ties preview to Create across all five
embedded presets as well as the odd-units payload.

**Stage 28 — traceability.** Five R4 fix ids had working, biting code but were
not greppable by id. The id now appears in the file that discharges it. Full
index:

| Fix id | Where it lives |
|---|---|
| `guards/probes-never-run-in-ci` | `tools/check-browser-probes.sh`, `Makefile`, `.github/workflows/ci.yml` |
| `promises/notes-never-exported` | `internal/app/export_notes_roundtrip_test.go`, `internal/wire/export_adapter_wiring_test.go` |
| `backend/import-silent-partial-success` | `internal/plugins/campaigns/import_report.go` + `import_report_test.go` |
| `promises/export-zip-media-dropped` | `internal/plugins/campaigns/import_zip_media_test.go` |
| `backend/syncapi-pull-1000-cap-no-cursor` | `internal/plugins/syncapi/sync_pull_cursor_test.go` |

Two review findings are recorded rather than fixed, because neither is a defect:

- **R4 stage numbers 7 and 8 do not exist.** The sequence jumps 6 → 9. A
  numbering blemish already acknowledged in the R4 report; no work is missing.
- **`tools/restore-drill.sh` (exit 2) and `tools/test-restore-drill.sh` (exit 1)
  fail only where no docker daemon is reachable.** Environmental, not a
  regression, and not caused by this branch.

#### 2026-08-07 — C-SWEEP-R4 stage 25 (deleting a calendar reset preferences it had nothing to do with)

**`calendar_active` holds four facts on one row and only one of them is about a
calendar.** `calendar_id` is the viewer's switcher choice; `sidebar_pinned`
(007), `block_layers` (014) and `bench_sections` (016) are per (user, campaign)
and merely piggyback on the same row — three signed decisions
(PR #368 stop-and-flag #3, `[LYR-3 SIGNED]`, `[BR2-5 SIGNED]`) put them there
rather than in a table of their own. `fk_calendar_active_cal` cascaded on
DELETE, and a cascade deletes the ROW, so deleting one calendar silently reset
every viewer's sidebar pin, layer set and Bench sections for the whole
campaign — including viewers who had never opened the deleted calendar.

**Migration 017 makes `calendar_id` NULLable and moves the FK to
`ON DELETE SET NULL`.** The pointer is still cleared by the delete; the three
preferences beside it survive.

**The booking said every path reverses something signed, so the migration says
what this one re-signs.** A separate prefs table is the shape three signed
refusals already rejected. The in-service reseat has no answer when the deleted
calendar was the campaign's LAST one — it loses the preferences in exactly the
case that loses the most. SET NULL re-signs ONE sentence of 006's header ("its
active-cal pointers go too") while keeping that header's actual promise verbatim:
"the next read falls back to the new default automatically". It still does. NULL
and "no row" resolve identically, which is what keeps this a schema change rather
than a semantic one.

**The reader moved with the schema, and had to.** Scanning a NULL into a plain
`string` is a `database/sql` error, so the migration on its own would have traded
a silent preference wipe for a hard 500 on every affected viewer's page.

**Testing without a database.** *(CORRECTED 2026-08-08 — the premise below was
false; see "A real MariaDB was available the whole time" at the end of this
file. The test design described here is still sound and still runs without a
database, but it was chosen under a belief that no alternative existed.)* No
MariaDB was thought to run in this build (the sibling FK
defect's tests say so already), so the pin replays the SHIPPED migration files in
version order and asserts the END STATE — 006 declares the cascade and is
immutable, so any test reading one file in isolation would assert either the bug
or nothing — and carries its own mutation test proving the replay can actually
SEE a cascade. The second half feeds the repository a real NULL through a
driver-level stand-in.

#### 2026-08-07 — C-SWEEP-R4 stage 24 (the front door threw away what the back door kept)

**The wizard is "one code path, two front doors" — and one of the doors was
lossier.** Six authored fields died between `builderDraftFromImport` and
`builderImportResult`: a moon's colour, an era's code, and `hours_per_day` /
`minutes_per_hour` / `seconds_per_minute` / `leap_year_offset`. Measured on a
Calendaria payload declaring 20/50/40 and leap offset 3:

    plain importer : hours=20 min=50 sec=40 leapOffset=3 moon=#22aa55 era code="TA"
    wizard door    : hours=24 min=60 sec=60 leapOffset=0 moon=#c0c0c0 era code=<nil>

Every one of those is a field the operator authored, the wizard DISPLAYED (the
Eras station prints the code), and then did not write.

**The booked split's open question is answered HIDDEN CARRY.** The booking noted
that carrying a moon colour or a time unit means choosing between a new visible
control on the Moons station — an authoring surface the wizard deliberately does
not offer — and an invisible hidden carry. Hidden carry is the option that
changes nothing about what the wizard ASKS: the fields ride through
`builderCarryFields` / `builderReadForm` exactly as a season's authored colour
and a month's day count already do. `builder.templ` is untouched and **no templ
regen was needed**, because the carry renders through the existing generic
hidden-input loop. "A colour is never invented here" still holds — an empty
colour stays empty, and `builderImportMoonSwatch` now covers the one case it was
ever right for: a moon the wizard itself created.

**`draftCalendar` had to move with it.** It hardcoded 24/60/60 and wrote an empty
moon colour and no era description, so fixing only the create path would have
left the wizard's own preview and export contradicting its own Create.

**One pin amended, named R4-S24-A.**
`TestBuilderPresets_RoundTripThroughBuildExport` asserted all three asymmetries
by equality rather than exemption — good discipline while they were booked, and
exactly what made this fix legible: it reds and says *"the asymmetry was fixed
(delete the exemption) or it moved (say where)"*. Each exemption is inverted into
the stronger claim that the authored value survives. The two "the payload must
still author a colour / a code" guards are kept, because without them the new
assertions would prove nothing.

**The new pin has two halves, and the second is the one that matters.** A fix
that stopped at `builderImportResult` would have passed a unit test and still
lost everything in the product, because the wizard has no server-side draft —
every preview rebuilds from the posted body. `TestBuilderDoorSurvivesTheFormRoundTrip`
replays the draft through the SHIPPED writer/reader pairing at all nine stations
in both importer modes; removing only the carry entries reds it everywhere while
the create-side test stays green.

#### 2026-08-07 — C-SWEEP-R4 stage 23 (the Elven preset had one season, three times)

**`parseCalendaria` only ever implemented one of Calendaria's two season
shapes.** Calendaria authors a season as either a day-of-year span
(`dayStart`/`dayEnd` from the start of the year) or a MONTH RANGE
(`monthStart`/`monthEnd` naming whole months, the day fields narrowing the first
and last). The parser read only the day fields, so every month-range file
collapsed. `presets/elven.json` — the Start gallery's Elven card — is one: its
three seasons differ ONLY in `monthStart`/`monthEnd` (0-2, 3-5, 6-7) and all
carry `dayStart 0 / dayEnd 45`, so all three imported as `1/1 → 1/45`. Three
identical, mutually overlapping ranges covering one of eight months; the other
seven belonged to no season at all.

**The un-signed decision in the booking was which base `monthStart` uses, and it
is settled by measurement rather than decree.** The two real exports in
`cordinator/references/calendars` genuinely disagree, so no constant is correct.
The base is detected per file from the smallest `monthStart` any season declares
— and on the three real payloads that is not a coin-flip, it is the only reading
under which each file's seasons tile its months exactly once: `forbidden-lands`
(8 months, starts 0/2/4/6) has no month 0 under a 1-based reading;
`calendar-of-therin` (15 months, starts 1/4/7/10/13) leaves its FIRST month
seasonless under a 0-based one. `monthEnd` is deliberately excluded from the
test, because therin's trailing `monthEnd 0` means "to the end of the year" and
would otherwise read as evidence of 0-basing — the exact one-month shift the
booking warned about.

**Nothing was re-authored and nothing preset-specific was added.**
`presets/elven.json` is untouched, so `builder_presets.go`'s "no preset-specific
code exists" claim still holds. A file that names no month keeps the day-of-year
reading byte-for-byte, which is the other half of the fix: teaching the parser
about `monthStart` must not reinterpret every season Chronicle has already
imported. The `seasonList` sort is re-keyed off `monthStart` so stage 13's
determinism does not regress — therin ties on every other field.

#### 2026-08-07 — C-SWEEP-R4 stage 22 (every N months fired every month)

**`OccursOn`'s monthly branch ignored `recurrence_interval`.** It checked the
day-of-month and the occurrence cap and returned, so an event authored as "every
3 months" was accepted, persisted and then expanded EVERY month. The week-based
branch has always applied its interval through `recurrenceWeeks`; monthly now
does the same, counted in months. The operator settled the booked fork on
**honour the interval**.

**Nothing below interval 2 moves.** `step` is the stored interval only when it is
greater than 1; absent, 0, 1 and negative all collapse to 1, which is
byte-for-byte the old expansion — and that is every row the shipped editor can
author, because `calendar_daycard.js::recurrenceBody` sends
`recurrence_interval: 0` for the month unit.

**The entangled cap bug is closed with it.** `RecurrenceMaxOccurrences` compared
the raw whole-month offset, so "every 3 months, 4 times" stopped after 4 MONTHS —
the 2nd occurrence — delivering half the series. It now compares `n/step`, the
0-based occurrence index, which is the same quantity `diff/stride` is on the
week-based branch. That half was booked as un-fixable until the fork was chosen.

**Two follow-ons are booked BY NAME rather than bundled in.**
`C-CALV4-MONTHLY-INTERVAL-CONTROL`: R2-2b withheld the `every [N]` field from the
month unit *because* the server ignored it, and said so in the source; that reason
is gone, so the control can return — as a UI change with its own inverse,
readout wording and request pins. `C-CAL-MONTHLY-INTERVAL-STORED-ROWS`: rows
already carrying `monthly` + `interval ≥ 2` are the only rows this re-spaces, and
no reconciler can tell a leaked interval from a deliberate one by looking at the
row, so stage 22 shipped none — what is needed first is a count from the
operator's database.

#### 2026-08-07 — C-SWEEP-R4 stage 21 (a query string could pin a core for a minute)

**`?year` on the world-state seed was an unauthenticated CPU-exhaustion vector.**
`Calendar.AbsoluteDay` summed `YearLengthForYear` from year 0, so its cost was
linear in the year — and the year arrives raw off a public query string on three
routes across two plugins (`calendar/worldstate_handler.go` `GetWorldState`,
`calendar/handler_v2.go`'s cursor, `syncapi/calendar_api_handler.go`
`GetWorldState`), all of which funnel into `BuildWorldStateSeed` → `moonSeeds` →
`AbsoluteDay`. Measured before the fix on a 12-month/366-day calendar:
`?year=2000000000` burned **53.5 s of CPU in one request**; 100000000 took 2.71 s;
250000 took 7.9 ms. No login, no campaign membership, no rate limit in between.

**The fix is the algorithm, not the input** (coordinator ruling). The year term is
now closed-form — `year·YearLength() + leapExtraDays·leapYearsBefore(year)`, where
`leapYearsBefore` counts an arithmetic progression in O(1) — so the same calls cost
~140 ns. **No clamp was invented**: a fantasy year number is authored data, a bound
would have needed the same arbitrary constant repeated at all three entry points,
and the next caller to reach `AbsoluteDay` from somewhere that is not a handler
would still have been linear. A campaign set in year 250000 — or 2000000000 — still
resolves exactly the date it resolved before. No context plumbing was needed for the
same reason: after the change there is no unbounded loop left to cancel; the
surviving month loop is bounded by `len(c.Months)`.

**The risk the booking flagged was that the leap arithmetic would not be
bit-identical**, silently shifting every moon phase and countdown.
`TestAbsoluteDayClosedFormMatchesLoop` discharges it by keeping the ORIGINAL loop
in the test file as an oracle and asserting equality across nine calendar shapes
(no modulus, offsets 0/1/3, an offset larger than the modulus, a NEGATIVE offset,
every-year leap, months with no leap days, no months at all) × 14 years including
0 and negatives × 7 months × 4 days.

**One guard was amended, and named: R4-S21-A.**
`TestBlockMoonBaseDayIsSteppedNotRecomputed` asserted that per-cell `AbsoluteDay`
is ≥5× slower than stepping off one base — true only *because* `AbsoluteDay` was
linear, which is the defect. The ratio inverted and the guard failed on a correct
tree. It is amended to the strictly stronger property it was always a proxy for:
year-independence, which forbids an O(year) `AbsoluteDay` outright rather than
tolerating one so long as a single producer routes around it. Both timing guards
race their work against a timer so a restored loop FAILS in a second instead of
hanging until the `go test` deadline.

#### 2026-08-07 — C-SWEEP-R4 stages 15–19 (the backup told four lies)

Four defects in the export/import/sync path, all reproduced before they were
touched. They share one shape: **the tool reported success while losing
data**, which is the failure mode that matters most for a self-hosted product,
because the export is the users' only safety net and an operator who is told it
worked stops checking.

**Stage 15 — campaign export omitted every shared note.** The envelope has had
a `notes` field and an `ExportNote` type since v1, and `ExportImportService`
has had `SetNoteExporter`/`SetNoteImporter` since v1. Nothing ever called them.
A nil adapter is skipped silently *by design* — some sections depend on
optional plugins — so `Export` wrote `notes: []` and `Import` read nothing, and
the two "intentionally not implemented in v1" comments in `export_adapters.go`
named exactly the repository method that was missing. Wired both halves:
`notes.ListSharedByCampaign` (the one list method in that package with **no
per-user filter**, because the shared-note corpus is campaign data — owner-
gated by its only caller, reachable from no HTTP route), plus the two adapters.
`ExportNote` grew `is_folder`, `content` and `parent_index`: checklists live
only in the legacy block content, so shipping notes without `content` would
have lost every checkbox in the campaign, and folders are referenced by index
the way `ExportEventConnection` already does, because note IDs are not stable
across instances. Import re-parents in a second pass (a folder may appear after
its children) and applies bodies through `Update` so imported HTML goes through
the sanitizer — an imported export is untrusted input. **Personal notes stay
out on purpose**: they belong to a user, and an export is handed to whoever
imports it. The wiring hole itself is now pinned in `internal/wire` by an AST
test that requires all twenty export/import setters — that is the test that
would have caught the original.

**Stage 16 — a partial import reported clean success.** Import is best-effort
on purpose: one bad row must not abandon a half-built campaign. The other half
was missing. Thirty-nine skip sites across the nine adapters each went to
`slog.Warn` and nowhere else, and the handler then redirected the operator
straight to the new campaign. `campaigns.ImportReport` now threads through the
adapters exactly the way `*IDMap` does; every skip records section, kind, name
and a user-safe reason, degraded-but-created rows included (an entity whose
body failed to apply is a loss even though the entity exists). `Import` returns
`(*Campaign, *ImportReport, error)` — a return value, not a side channel, so a
caller that ignores a partial import has to ignore it in writing. Detail is
capped at 200 records; **the count stays exact past the cap** and the summary
admits "and N more", because the count is what tells the operator whether to
trust the restore. The handler stops redirecting when anything was lost and
renders the loss list instead. A clean import redirects exactly as before.

**Stage 17 — "Export ZIP (with media)" round-tripped to nothing.** The export
side was never broken; the zip really does hold `campaign.json` plus real
bytes. Two things made it a lie. The import form's `accept` was
`.json,application/json`, so the file picker would not offer the operator the
`.zip` they had just been told to make — the handler could parse a zip, the UI
could not deliver one, and the ZIP round-tripped to **nothing at all**, not
even structural data. And media entries in an accepted zip were dropped with
one `slog.Info`. **Ruling taken: stop promising, book the rest by name** —
not "make it round-trip". Restoring the *files* is easy (`MediaService.Upload`
already takes bytes); restoring the *references* is the job, and files without
references is a **new** quiet lie in place of the old one — the library fills
up and every image stays broken. Every reference is a media ID
(`entities.image_path`, `cover_image_path`, `maps.image_id`, token
`image_path`, and `/media/<id>` inside `entry_html` across entities, posts and
notes), and the manifest cannot even be paired with the zip today because
`ExportMediaFile` carries no `Filename`. Booked whole as
**C-IMPORT-MEDIA-RESTORE** in `.ai/todo.md` with its four steps, the import-
ordering change it forces, and its acceptance test. What shipped is honesty:
the form accepts `.zip` and states both size caps, the settings copy says
plainly what each export contains and what an import does *not* give back, and
the unrestored count rides the stage-16 report ("3 media files — archived in
the zip but not re-attached on import").

**Stages 18–19 — the thousand-and-first entity could never reach the VTT.**
`POST /api/v1/campaigns/:id/sync` walked the entity list to `syncMaxPullPages`
and stopped, setting `has_more` truthfully with nothing in the request able to
act on it: `since` is a **filter** over the list, not a position in it, so the
next request re-walked the same first thousand. Kept the cap as a page size and
added the cursor — opaque on the wire (versioned base64) so it can become a
keyset later without breaking a client that hard-coded the arithmetic, bounded
so a corrupt cursor cannot ask for an absurd OFFSET, and **400 on a malformed
cursor rather than a silent reset to page 1**, since a silent reset restarts
the walk from the top and looks like it worked while the tail stays exactly as
unreachable. Offset paging is only safe here because entity list ordering
became a total order in sweep R3 stage 4 (`e.id ASC` on every clause); without
that tiebreaker this endpoint would have re-imported the duplicate/skip bug at
scale, which is why the walk test asserts no duplicates. `docs/api/openapi.yaml`
gained both fields and the rule about which `server_time` a client keeps: the
one from the **first** response of a completed walk, not the last, or anything
modified mid-walk is skipped. Stage 19 fixed the client twin in
`Chronicle-Foundry-Module` (`0d17f9c`): `JournalSync.resyncAll` and the
dashboard's `_buildEntityGroups` each had `while (hasMore && page <= 5)`
inline — a silent 500-entity ceiling — now one shared
`scripts/_entity-page-walk.mjs` bounded at 200 pages whose `truncated` flag the
GM is actually told about. Fixing the server and leaving the client capped at
500 would have left the operator exactly as stuck.

**What these do not promise.** Nothing here was exercised against a real
MariaDB, so the note round trip, the failure tally
and the cursor walk are all proven against fakes that replicate the service
contracts, not against the database. *(CORRECTED 2026-08-08: the reason given
here — "this environment has none" — was FALSE. A real MariaDB was available
the whole time; see the section at the end of this file. The tests below are
still fake-backed, which is now a gap someone can close in an afternoon rather
than an environmental limit.)* The note exporter's SQL
(`is_shared = TRUE`, `ORDER BY created_at, id`) is read, not run. The import
result panel's markup is asserted as a string, not rendered in a browser. And
media restoration is **not** fixed: a ZIP import still leaves every image
broken, and the only change is that it now says so.

#### 2026-08-07 — C-SWEEP-R4 stage 13 (the probes had never once run)

**Every real-browser probe was a silent pass, twice over.** The probes are the
only tests in the repo that look at the RENDERED result — they drive a headless
Chromium over a built page and read geometry out of the live layout. Every other
test asserts on the strings we emitted, which cannot see a container query
resolve, a Shelf collide with the Ledger, or a phone breakpoint fail to swap.

They all open `if testing.Short() { t.Skip }`, and `-short` is *exactly* the mode
CI's "Build & Test" job and `make verify` run. They then skip again with no
Chromium — and a `go test` SKIP hides inside an `ok` package line, so a machine
with no browser produced a green run indistinguishable from a measured one.

Reproduced: changing the Shelf scroller's `max-height` from 132px to 160px in
`static/css/calendar-block.css` leaves `go test ./internal/widgets/calendar_block/
-short` reporting `ok`.

`tools/check-browser-probes.sh` runs the eight covered probes **without**
`-short` and then requires a `--- PASS:` line from each **by name** — so a probe
that skipped, failed, was renamed or was deleted fails the guard, and coverage
cannot evaporate by rename. With no browser it prints a loud banner naming every
probe that did not run and exits 0; `BROWSER_PROBES_REQUIRED=1` (set by the new
`Browser Probes` CI job) makes that fatal instead. `TestProbe_MobileBreakpointSwap
InRealBrowser` also needs the Tailwind CLI and is tracked as its own named
dependency group. The guard self-tests on every run (SKIP / FAIL / ABSENT must
each fire) in the house guard idiom. Wired into `make verify` as `make
test-probes`.

Out of scope by design: the opt-in explorers and screenshot generators
(`DAYCARD_GEOMETRY`, `DAYCARD_FLOORS`, `DAYCARD_MORPH_TRACE`, `*_screenshot_gen`)
— they emit artefacts and assert nothing.

#### 2026-08-07 — C-SWEEP-R4 stage 12 (a half-applied plugin migration was terminal)

**A plugin migration that died on its second statement could never be finished.**
`execPluginMigration` splits on semicolons, executes statement by statement on a
plain `*sql.DB`, and writes the `plugin_schema_versions` row only after the LAST
one succeeds. A failure part-way therefore left the earlier statements' effects
in the database and *no record that anything happened*. The next boot replayed
from statement one and died on "duplicate column name" — because most plugin
ALTERs are not idempotent — so the operator saw an artefact of the retry instead
of the real cause, and repairing the real cause could never help.

**This is not a transaction and does not pretend to be one.** MariaDB has no
transactional DDL: every CREATE / ALTER / DROP / RENAME commits implicitly. The
error text says so rather than implying a rollback that did not happen.

Two mechanisms, in `internal/database/plugin_migration_safety.go`:

- **Pre-flight applicability check.** Every statement is validated against the
  schema catalogue *as it will stand at that point in the migration* (the
  simulation moves forward, so the normal "CREATE then ALTER" shape is not
  falsely refused). If any statement cannot succeed, the migration aborts having
  executed NOTHING — half-applied-and-unrecoverable becomes nothing-applied-and-
  actionable. Table granularity; **fails open** if `information_schema` cannot be
  read, because refusing every plugin migration over a metadata hiccup would be
  worse than the bug.
- **Partial-progress recording** in a new runtime table `plugin_migration_progress`.
  The count of statements that DID apply is written before the error returns, so
  the next boot resumes after them. Keyed by a sha256 of the migration text and
  honoured only on a byte-identical match — a resume that skipped the WRONG
  statements would be far worse than the crash-loop it replaces. Every uncertain
  answer degrades to replaying from zero, i.e. the pre-existing behaviour.

Three DB-backed regression tests (`TestPluginMigration_*`) run in the existing
`Fresh-DB Migration Replay` job and are named in its grep-for-PASS assertion, so
a skip fails the job. `TestFreshDatabase_EveryPluginSchemaApplies` still passes,
which is what rules out the pre-flight falsely aborting a real migration — a
false abort is the one outcome worse than the bug.

#### 2026-08-07 — C-SWEEP-R4 stage 11 (every fresh install shipped a dead plugin)

**`foundry_vtt`'s migration 001 crashed on its FIRST statement on every
brand-new database.** It is a consolidation migration —
`RENAME TABLE foundry_module_campaign_tokens TO foundry_vtt_campaign_tokens` —
and the plugin that created that source table was deleted in C-FMC-5c. A fresh
self-hosted install therefore hit `Error 1146`, the plugin was marked DEGRADED,
and it could never self-heal: the runner returns on the first failed migration,
so no later migration for that plugin was ever reachable.

`PreMigrationCheck` (PR #507) does **not** cover this. It only refuses when
`foundry_module_versions` exists *and has rows*; on a fresh database the table
does not exist, so the check returns `nil` and the doomed migration runs anyway.

The reason it survived: **CI never migrated an empty database anywhere.** The
restore drill loads a dump of an already-migrated one, and every other
integration test assumes `make migrate-up` already ran.

Migration 001 is immutable (ADR-044/045), so the repair is split:
`foundry_vtt.ReconcileConsolidationState` records 001 as applied on any database
where its RENAME has no source table, and new migration **002** establishes the
post-consolidation shape with idempotent DDL. Fresh install, completed upgrade
and an upgrade that crashed between 001's two statements all converge on the
same schema; a database that still HAS the predecessor table is untouched and
001 runs for real, carrying its live token rows across.

Verified by replaying the real bootstrap against genuinely empty MariaDB schemas
(`make test-freshdb`, `cmd/server/freshdb_migration_test.go`) — one from zero,
one from the pre-consolidation shape — and wired into CI as its own
`Fresh-DB Migration Replay` job that fails if either test merely *skips*.

#### 2026-08-07 — C-SWEEP-R4 stage 9 (the anonymous dm_only leak — ADR-049)

**"No authenticated user" and "trusted system caller" were the SAME value, and
the filters read that value as trusted.** `C-AUTHZ-EMPTY-USERID`, the only
high-severity unauthenticated confidentiality break R3 found, is closed.

On a PUBLIC campaign a logged-out visitor carries `role = RoleNone` and
`auth.GetUserID(c) == ""`, and the calendar/timeline visibility filters
short-circuited on exactly `userID == ""` — documented in both plugins as "the
system context". So anonymous internet traffic took the MOST privileged branch
and was served `dm_only` calendars, `dm_only` timelines and per-user-restricted
events and event links that a logged-in Player on the same campaign is
correctly denied.

**The fix is a type, not a check.** `internal/permissions/viewer.go`'s `Viewer`
carries an **unexported** `system` bit with two constructors: `RequestViewer`
(anything off an HTTP request — an empty user id means ANONYMOUS and cannot
become trusted) and `SystemViewer` (declared at the call site). Filters ask
`SkipsPerUserRules()`; **an anonymous viewer is neither an Owner nor a system
caller**, so it fails closed by construction rather than by every call site
remembering. This is the concrete form of C-CALV4-V2SUNSET **[VS-15]** — an
empty user id is an ABSENT per-user layer, never a sentinel — which matters
because R2-4 moves `GET /apps/calendar` onto the public group and widens this
exact surface.

**A third consumer, swept and fixed in the same stage:** the sync API's
`GET /calendar/events` passed `""` into the same filter on purpose, so a
Player-level API key (or a Player/Scribe member on the session door) was served
events whose `visibility_rules` restrict them to OTHER users. It now forwards
the caller's own identity (`resolveUserID`) — narrowing only, and a
sync-permission key is untouched because `resolveRole` gives it Owner.

**Behaviour deliberately preserved:** the two genuine trusted callers
(`timeline_widget_type.go`'s Scribe-gated create-or-pick picker and the
campaign timeline export adapter) now pass `permissions.SystemViewer` and see
exactly what they saw before. **Two green tests that pinned the bug as intended
were INVERTED, not deleted**, each keeping a system-path row. `[EU-5]`, a
route-level anonymous-identity floor, was **booked, not taken** (`.ai/todo.md`
§B2), as was the observation that the two per-user rate limiters skip anonymous
traffic entirely (§B3 — a cost question, and [VS-15] forbids the per-IP key
that would fix it).

#### 2026-08-07 — C-SWEEP-R4 (the absent-means-preserve contract, done once)

**The whole partial-write class is closed, product-wide, by one contract.**
Report: `cordinator/reports/chronicle/2026-08-07-C-SWEEP-R4.md`. Six stages
here plus one in `Chronicle-Foundry-Module`.

> **An ABSENT key preserves the stored value. An EXPLICIT `null` clears it. A
> present value replaces it.**

Ruled by the coordinator on 2026-08-07, honouring the `C-SIDEBAR-REORDER-RESCUE`
PR1 step 1 booking. Client-side echo-the-full-body was REFUSED as the primary
fix — it leaves the endpoint armed for the next writer, and R3 proved there is
always a next writer. Read `.ai/conventions.md` §"Partial-Update Endpoints"
before writing any update handler.

**The mechanism is `internal/patch.Field[T]`.** It records presence in
`UnmarshalJSON`, which encoding/json only calls for keys the body actually
carries — so "absent" and "null" stop being the same `nil`. `Ptr(cur)` merges
onto a nullable column, `Val(cur)` onto a NOT NULL one. Services load the row
and merge onto it; validators read the MERGED value, not the raw input.

**What this changed, by endpoint** (each pinned in all three directions):
sessions (Mark Complete no longer erases the schedule, the summary, the
in-world date and the recurrence — and the next-occurrence generator fires
again); entities (a sync push no longer un-parents; a `{name}` push no longer
un-privates a hidden character to every player, on BOTH the single and batch
doors); timeline standalone events (a rename no longer clears eight fields
including per-player visibility rules); map markers (an edit or a drag no
longer clears `pin_category`, `visibility_rules` or the Foundry pairing key);
calendar events (a five-key Foundry push no longer turns off recurrence,
all-day and the entity link).

**Three previously-green pinned expectations were AMENDED**, each named in a
comment and mutation-tested:
`TestUpdateEvent_AnOmittedIsRecurringIsAWriteOfFalse` (inverted — it pinned
the booked hazard as a deliberate non-fix),
`TestUpdateEvent_EntityIDStillClearsOnNil` (name and guarantee kept, gained the
absent-preserves half — `C-ENTITY-LINK-DESIGN` is NOT overruled, because its
guarantee was the ability to clear, and an explicit null still clears), and
`TestEventHandler_BindsTierAndAllDay` (stopped counting one substring twice now
that create and update spell the binding differently).

**Ratchet:** `internal/patch/partial_update_contract_test.go`. It does not try
to detect "assigns unguarded" — that needs cross-package data flow and is
mostly false positives. It pins the PRECONDITION: a field can only preserve an
absence if its type can represent one. Every field of a contract-governed
`Update*Input` must be presence-typed, and the whole-tree inventory of
`Update*Input` structs is frozen, so a new one must be classified out loud.
Twenty structs sit on `notYetSwept` — that list records what was LOOKED AT,
not what is safe.

**Cross-repo:** `Chronicle-Foundry-Module` `f3ffa90` rewrote `API-CONTRACT.md`
(which had documented the opposite — "absent means public"), commented each
narrow body as narrow ON PURPOSE, and added
`tools/test-partial-put-contract.mjs` so nobody "hardens" them with an echo.
The marker dialog's existing spread stays: harmless against a merging server,
load-bearing against a pre-R4 one.


#### 2026-08-07 — C-SWEEP-R3 (eight sweeps: security · partial-PUT · backend · frontend · promises · data · guards · backlog)

**Sixteen fixes shipped in fifteen stages, twenty-five findings booked for a
signature, four claims refuted.** Report:
`cordinator/reports/chronicle/2026-08-07-C-SWEEP-R3.md`. The booked set is
enumerated in `.ai/todo.md` §"Booked by sweep R3"; eight of them earned their own
dispatch under `cordinator/dispatches/chronicle/`.

**Two cross-cutting facts this sweep established, which later work should not
re-derive:**

1. **The dominant defect class in this codebase right now is the whole-replace
   PUT.** Seven independent clients — sessions "Mark Complete" and its edit modal,
   syncapi entity update, the Foundry actor push, the Timeline standalone-event
   modal, the Foundry calendar-event push, and the web marker form/drag — send
   partial bodies at endpoints that assign every field unguarded, so an omitted
   key is a WRITE. It is invisible per-site because the request structs are a
   patchwork: pointer fields nil-preserve, value fields (`bool`, `string`) clear,
   and two pointers deliberately clear-on-nil. Consequences measured this sweep
   range from lost schedules to **un-privating a hidden character entity** and
   **NULLing the Foundry pairing key**. One ruling — absent-means-preserve with an
   explicit-null form, per `C-SIDEBAR-REORDER-RESCUE` PR1 step 1 — closes all
   seven; a per-client patch contradicts that booked precedent and collides with
   the parked `C-ENTITY-LINK-DESIGN`. Booked as **`C-PARTIAL-PUT-CONTRACT`**. The
   one member that needed no ruling (the Foundry marker config dialog) shipped in
   stage 3, cross-repo.
2. **"The route is authorized" is not "the object is authorized".** Four of the
   five security fixes are the same mistake at four addresses: the middleware
   proves campaign membership and the handler then addresses a relation, a note,
   a calendar or an entity **by its own id** with no ownership or visibility
   predicate at any layer below it. Two were cross-user reads of private data
   (notes, backlinks), one was a cross-CAMPAIGN write reachable by an enumerable
   integer PK (relations), one served a `dm_only` calendar to anyone (public
   calendar reads). The pattern to copy is the one the fixes used: **resolve the
   object, 404 on a campaign mismatch, then run the plugin's canonical visibility
   gate — before the cache lookup, not after.** A fifth instance is booked and
   unfixed: the `userID == ""` "system context" sentinel also matches an anonymous
   visitor (**`C-AUTHZ-EMPTY-USERID`**, high).

Also of record: entity `LIMIT/OFFSET` paging had **no total order**, so a walk
returned 563 of 50,000 entities twice and missed 563 entirely on real MariaDB —
and both **campaign-export** walk-to-exhaustion loops inherit that reader.
Guards were strengthened twice, never weakened (the Bench CSS scope scanner now
reads by brace, `check-page-scripts.sh` now walks the open tag), and a new
whole-tree `check-widget-mounts.sh` ratchet proved every literal `data-widget`
mount in the tree now has a load path.

#### 2026-07-26 — C-CALV4-FOUNDATION-P0 (calendar-v4 wave 1, phase A)

The floor the other four wave-1 slices stand on. Additive only — no Go behaviour
change, no templ change, no route, no migration.

| Item | What |
|---|---|
| Data contract | `internal/widgets/calendar_block/data.go`, copied **byte-identically** from the coordinator pin (`cordinator/dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`). It is the ONE cross-slice artifact of the wave; three dispatches write against it. **Editing the struct is a STOP-AND-FLAG**, not a judgement call. `data_test.go` pins the field set (name + kind + composite type, in declaration order) by **reflection only** — it reads no source text, so restructuring `data.go` cannot red CI. |
| Motion tokens | `--motion-fast` / `--motion-standard` / `--motion-leisurely` were referenced fallback-only throughout the V2 animation library and **never defined**. Now defined in `static/css/input.css:146-148` at exactly their existing fallback values (150 / 200 / 300ms) — a no-op on shipped surfaces. Deliberately NOT aliased to `--dur-*` (`--dur-micro` is 120ms, not 150ms). |
| Guards B1–B4 | `tools/check-calendar-v4-lints.sh`, wired into CI. Diff-scoped against `origin/main` like the other guards. Self-tests itself on every run, so "OK" always means the rules can still fire. |
| Motion-discipline guard | `tools/check-v2-motion-discipline.sh` had existed since the V2 wave and was invoked by **nothing** — only a comment at `input.css:103` referenced it. Now wired into CI. It was already diff-scoped and passes on main; a non-diff-scoped run would flag 7 grandfathered lines across 5 files. |
| Deleted guard | `tools/check-templ-drift.sh` — diffed pathspec `'*.templ.go'` while templ emits `*_templ.go`, which are gitignored. Structurally incapable of failing; reported OK on a tree where 753 generated files had changed. **Deleted rather than repaired**: `templ generate` + `go build` in CI already catch real drift, and a guard that reads as coverage it never provided is worse than no guard. |
| `make verify` | New target chaining the full local CI sequence in CI order. Five parallel chats were each reconstructing it from `ci.yml` by hand. |
| `calendar_v2/.ai.md` | Filed at last (house rule 7). States the package is **read-only for the v4 wave** and why: its `data-visibility-*` DOM contract has two independent JS drivers in the calendar plugin, and nothing — compiler or test — catches an attribute rename. |

#### Active arc: Calendar remodel (cordinator `plans/2026-07-24-calendar-remodel-requirements.md`)

Wave 1. Source-of-record for scope + operator priorities is the cordinator plan; ADRs land here.

| Item | What | Status |
|---|---|---|
| C-CAL-ENTITY-TIES-LEAK-FIX | Entity-visibility leak on event/era tie lists | ✅ shipped PR #565 |
| C-CAL-RSVP-P1 | First-class RSVPs on calendar events + SMTP action emails (operator's #1 priority) — ADR-046, calendar migration 013 | ✅ this branch |
| C-CAL-RSVP-P2 | Players give TEMPORARY availability from the RSVP flow — "suggest another time" becomes structured windows that land in the scheduler overlay. No new tables/routes. ADR-046 amendment | ✅ this branch |
| C-CAL-MOBILE-VIEWS-FIX | Phone Month\|Agenda toggle: the Agenda pill was a dead control (linked to the URL it was already on). Step 0 **disproved** the "mobile layout never swaps" half — a 390px browser probe shows the swap always worked; the reported "7-column grid" is the shipped mini-month. Absorbs **C-ASSET-VERSIONING** (see below) | ✅ this branch |

**Operator gate still open:** RSVP *emails* need SMTP configured (Admin → `/admin/smtp` →
"Send test email"). In-app RSVP works without it — every mail seam is nil-safe.

##### calendar-v4 widgetization remodel (wave 1, 2026-07-26)

Operator directive: *"just go with a full redo of the calendar."* Source of record is
cordinator `plans/2026-07-26-calendar-v4-remodel-master-plan.md`; the design contract
(`mockups/calendar-v4.html` + 73 signed stills) is **immutable**, and the nine coordinator
rulings live in `dispatches/chronicle/C-CALV4-WAVE1-COMMON.md` §6.

| Slice | What | Status |
|---|---|---|
| C-CALV4-FOUNDATION-P0 | Phase A — the data contract + the guard rails: the pinned `BlockData` + its reflection shape pin, the three never-defined `--motion-*` tokens, guards B1–B4 and the dormant motion-discipline guard wired into CI, the vacuous templ-drift guard deleted, `make verify`, `calendar_v2/.ai.md`. Detail under its own dated heading above | ✅ PR #569 |
| C-CALV4-SPINE-P2 | Server spine: leap-aware geometry, one-pass per-viewer projection, batched eager loader, narrow interfaces, the wave's only `app/routes.go` touch | ✅ this branch |

**Two cross-slice facts a later slice must not re-derive:**

- `internal/widgets/calendar_block/data.go` is the ONE cross-slice contract, pinned
  verbatim at cordinator `dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`. It is copied
  byte-identically (including a struct-tag alignment `gofmt` would change — do NOT run
  `gofmt -w` on it; no gofmt linter is enabled in `.golangci.yml`, so it is safe).
  **Editing the struct is a stop-and-flag.**
- `calendar.BlockSpine()` is the process-wide accessor for the Block service, installed
  from `app/routes.go` via `calendar.InstallBlockSpine`. It exists because `routes.go`
  belongs to exactly one calendar-v4 slice for the whole wave, so the phase-B surfaces
  (entity-page hosting, the Bench) reach the spine without re-editing it.

**RESOLVED (r51 pin amendment, C-CALV4-SEAM-P5 stage 1, 2026-07-27):** the pin
originally typed `CalendarID` / `Mark.EventID` / `ViewerContext.UserID` /
`ViewerContext.HostEntity` as `int64` while all four are `VARCHAR(36)` UUIDs in
Chronicle, so the producer zeroed them and smuggled the calendar's identity
through `CalendarSlug`. The amended pin types them `string`; the producer now
carries the real ids and `CalendarSlug` means the slug again.

**Cross-plugin note:** `sessions.NotifyUsers` (generic notification fan-in) is new. The
notifications store was always documented as generic (T-B2); C-CAL-RSVP-P1 is its first
writer outside the scheduler. Future features should use `NotifyUsers` with their own type
constant rather than adding a bespoke `NotifyX` per feature.

#### 2026-07-26 · C-CALV4-BLOCK-P1 (calendar-v4 wave 1, W-A) — the Block, render tier

New plugin-agnostic widget package **`internal/widgets/calendar_block`** plus
**`static/css/calendar-block.css`**. Nothing outside those two paths changed;
no route, no migration, no edit to any frozen calendar file. Full detail lives in
[internal/widgets/calendar_block/.ai.md](../internal/widgets/calendar_block/.ai.md);
the load-bearing points for anyone else in this repo:

- **Sizing is CSS container queries, and it has to be.** `boot.js:163` sets
  `htmx.config.allowScriptTags = false`, so a `<script>` inside an HTMX-swapped
  fragment never executes — a JS-sized widget silently renders at the wrong
  density after any swap. Size class comes from the host wrapper
  (`container-name: cal-block`; full ≥900 · std ≥300 · mini ≥240 · else
  sub-mini), density from each cell (`container-name: cal-cell`; measured column
  ≥84px → names). **This is the pattern any future self-sizing widget should
  copy**; `internal/widgets/calendar_block/sizing.go` carries the arithmetic in
  Go for tests only and is never called at render time.
- **`static/css/calendar-block.css` is UNLAYERED and self-contained** per
  `cordinator decisions/2026-06-05-rendering-canvas-css-exemption.md`, and every
  selector is scoped under `.cal-block-host`. An unlayered sheet outranks the
  app's layered CSS, so an unscoped rule in it would restyle the whole product;
  `TestCSS_EverySelectorIsScoped` parses the sheet and enforces that.
- **A second real-browser probe exists.**
  `container_query_probe_test.go` joins
  `calendar_v2_mobile_breakpoint_probe_test.go` as a test CI never runs (both
  skip under `-short`). Measured columns at the signed host widths are tabulated
  in the widget's `.ai.md`.
- **`data.go` is a cross-slice PIN** (cordinator
  `dispatches/chronicle/C-CALV4-BLOCKDATA.go.txt`). Editing the struct is a
  stop-and-flag: C-CALV4-SPINE-P2 writes what this package reads.

#### Static asset versioning (C-ASSET-VERSIONING, shipped inside C-CAL-MOBILE-VIEWS-FIX, 2026-07-26)

Every template-emitted static URL now carries a cache-busting `?v=<digest>`.
**This is a cross-cutting convention, not a calendar detail** — a new template
that writes `src="/static/…"` by hand fails a contract test.

- **`layouts.AssetURL(path)`** (`internal/templates/layouts/assets.go`) is the
  single helper. It appends the first 10 hex of the asset's SHA-256 when the
  file resolves (on-disk `static/` root, or a plugin embed FS registered via
  `layouts.RegisterAssetFS` — wired in `app.mountPluginStatic`), and falls back
  to a per-BUILD token (executable size+mtime) when it can't. Non-`/static/`
  URLs and URLs already carrying `?`/`#` are returned untouched. Digests are
  memoized per path for the process lifetime.
- **`middleware.StaticCache(prefix)`** (registered globally in `app.New`) turns
  those tokens into policy: `immutable, max-age=1y` when `?v=` is present,
  `max-age=0, must-revalidate` otherwise. The second arm is the safety net — an
  unconverted asset degrades to "always revalidated", never to "silently stale".
  Echo's `e.Static` sets NO Cache-Control at all, so before this browsers
  applied heuristic freshness and could hold build-old assets for hours.
- **Why now:** C-CAL-MOBILE-VIEWS-FIX Step 0 reproduced a live consequence.
  `md:contents` was a NEW Tailwind utility in C-CAL-MOBILE-AGENDA; rendering
  post-deploy HTML against a pre-deploy `app.css` drops the Week/Day/Timeline
  pills from the **desktop** calendar entirely, with every DOM test green. That
  is the "calendar looks old after deploy" class, demonstrated rather than
  assumed.
- **Guard:** `TestTemplatesUseAssetURL` walks every `.templ` in the repo and
  fails on a bare `src="/static/`/`href="/static/`. All 101 existing call sites
  across 17 templates were converted.

#### Closed arc: NW-2.2 plugin-isolation refactor

Per `cordinator/reports/chronicle/2026-05-23-c-plugin-isolation-audit.md` §3 (7 chunks A-G + D2-cleanup):

| Chunk | What | Status |
|---|---|---|
| A | Lightweight `PluginRegistration` registry (`internal/plugins/registry.go`); foundry_vtt + smtp pilots | ✅ shipped PR #334 |
| B | Magic-string consolidation (4 code + 2 templ sites) | ✅ shipped PR #332 |
| C | Cross-plugin import discipline docs (this file's §Cross-plugin imports above) | ✅ shipped PR #333 |
| D | Plugin-specific UI back into owning plugin (4 sub-refactors: banner / dashboard sync block / settings guide / show-banner fragment) | ✅ shipped PR #338 |
| D2-cleanup | Drop unused `fmBanner` + `maps.FoundryPresence` chains exposed by Chunk D; preserves campaigns.FoundryPresenceLookup (live diagnostic) | ✅ shipped PR #342 |
| E | Per-plugin `.ai.md` split + `status.md` shrink via archive-and-thin-index | ✅ shipped PR #335 |
| F | Per-plugin static-asset ownership via `embed.FS` (calendar pilot; other plugins migrate opportunistically) | ✅ shipped PR #336 |
| G | Packages plugin per-row foundry UI fragment; pattern for D. Blocks 2-4 deferred to G2-wave per reshape pattern | ✅ shipped PR #337 |

#### Closed arc: Wave 2 security work (2026-05-22 → 2026-05-26)

| Chunk | What | Status |
|---|---|---|
| 1 + 5 | Password-reset log scrub (`auth.hashEmail`) + `database.SafeIdent` DDL helper | ✅ shipped PR #331 |
| 2 (Phase 2B) | Focused AST middleware-pin for the Foundry public rate-limit invariant (`internal/wire/foundry_public_ratelimit_test.go`) | ✅ shipped PR #339 |
| 3-AMENDED | `syncapi.RequireJSONContentType` middleware + `v1Multipart` sub-group skip pattern (D-C3.1) | ✅ shipped PR #344 |
| 4-AMENDED | `loadDescriptor` fallback snapshot test pinning Chronicle defaults against canonical `chronicle-package.json` from Foundry-Module | ✅ shipped PR #343 |
| 6-AMENDED | Egress HTML sanitization on the 6 `/api/v1/*` GET handlers via `internal/plugins/syncapi/egress_sanitize.go` (D-C6.1); D4=(c) backup/restore lossless preserved | ✅ shipped PR #345 |
| 7 | File-level sanitize-on-write invariant (`internal/sanitize/invariant_test.go` + snapshot) | ✅ shipped PR #340 |
| 8 | `.ai/conventions.md §Security` consolidated reference | ✅ shipped PR #341 |

Wiki All-Pages mobile-layout cosmetic fix (`data-entity-id` on the shared `EntityTableRow` to match the bulk-actions widget's contract): ✅ shipped PR #346.

#### Open work

- **C-SEC-CHUNK-2-PHASE-2C** — full middleware-chain capture for every route via `golang.org/x/tools/go/packages`. Deferred from PR #339's reshape.
- **C-SEC-CHUNK-7-PHASE-2** — method-level sanitize invariant with flow analysis + helper tracing. Deferred from PR #340's reshape.
- **G2-wave** — packages plugin per-row foundry UI Blocks 2-4 (deferred from PR #337).
- **F2-wave** — remaining plugins migrate to per-plugin `embed.FS` static assets (deferred from PR #336; calendar was the pilot).
- **NW-2.3** — move `/foundry-presence` endpoint into the foundry_vtt plugin (currently lives on campaigns; was preserved by D2-cleanup as a parallel structure).
- **Draw Steel spin-up** — pending its own security audit per `cordinator/decisions/2026-05-26-draw-steel-spin-up-strategy.md`.
- **AI Export Pipeline** — scoping locked per `cordinator/decisions/2026-05-26-ai-export-pipeline-design.md`; implementation pending.
- **Plugin Host interface design pass** — tracked in `cordinator/decisions/2026-05-23-plugin-registration.md`. Deferred from Chunk A.
- **C-CAL-WORLDSTATE-EFFECTS-SYSTEM** — synced world-state animation (Almanac sky-band + hourglass) on `/demo/calendar/almanac`, mock-data only. Spec in `docs/design/world-state-effects/`. **Wave 0 + Wave 1 + Wave 2a/2b merged to main; Wave 2c (mood-tint) merged (PR #395)** — this closes the Wave 2 MUST set. Shipped: `worldState` + `setWorldState` pub/sub spine + unified `EFFECTS` registry (PR #388); sun supersession to inline `lorc/sun.svg` + hourglass interior (heightmap sand + day/night + glass/wood materials, PR #389/#390); 10-effect weather/celestial bundle (PR #391); ~28-option moon library w/ vendored Noto/Twemoji + 12 procedurals (PR #394); mood-tint wash (PR #395, merged — Wave 2 MUST closed). **Wave 3 (time-control verb layer) shipped** — `timepieceFill`/`atmospherePaused` state + verbs (+1hr/+1day/long-rest/custom/set-time/step-back/atmosphere-pause) tweened on the shared rAF (`CalParticleEngine.addTick`), fill caps ~1/3 → reuse dawn/dusk flip; reusable mechanics in `window.__calTimeControl`. Tests: `test/js/*.test.mjs` (`make test-js`) + Go static guards in `internal/templates/demo/calendar_*_test.go`. Visual verification is the operator's local gate (build env has no headless browser). **After Wave 3:** Waves 4–5 incremental effect long-tail + the production GM Live Control Panel (post-deadline). Queue in `.ai/todo.md §2`.
- **C-TIMELINE-V2-DESIGN-1-TUNER** — the "FM Tuner" timeline showcase on `/demo/timeline/tuner`, mock-data only, page-separated (own `cal-timeline-tuner.{css,js}`), raw SVG + CSS transforms (NO D3). Lead of two candidate timeline designs (Ledger alternate not yet built). Radio-dial etched-metal time axis through the canvas middle with adaptive tick notches (7 zoom levels millennia→days); swim-lanes above/below (entity/category/tier grouping); era gradient bands + watermarks; hover-revealed entity-color-coded connection arcs + show-all toggle; self-contained effect registries with `timelineAxisRender` + `timelineBackdropRender` hooks; §J2 restrained atmospheric backdrop (weather + non-routine celestial always; sun+moons ONLY on special-moon days); §J1 cursor-sync DOM-event protocol (`cal:cursor-change`/`cal:event-create`/`cal:date-jump`, loop-prevented, 50ms drag throttle) — **Almanac amended to emit/listen too** (small `cal-almanac.js` delta + `window.__calCursorSync`). Exempt-OKLCH canvas CSS carries the rendering-canvas exemption marker. Tests: `test/js/tuner.test.mjs` + `test/js/cursor_sync.test.mjs` (+ shared-harness event-bus addition) + Go render/discipline guards in `internal/templates/demo/calendar_timeline_tuner_test.go`. Visual verification is the operator's local gate. **Merged (PR #414).**
- **C-CAL-WORLDSTATE-WIDGETS** — Phase 6 widgetization: graduates the showcase worldState renderers into a production widget + an entity-page block, completing "all three views entity-able" (calendar #411/#413, timeline Tuner #414, worldstate here). New `entity_worldstate` block (`internal/plugins/calendar/entity_worldstate_block.*`) renders the "mini shelf view" (sky band + hourglass-on-shelf) — campaign-level, `Contexts:["template","dashboard"]`, Singleton, friendly empty(Create-calendar CTA)/unavailable states mirroring #413. The `worldState` **provider singleton** (`static/js/widgets/worldstate_provider.js`) is the one source of truth per page: ONE `/calendar/world-state` fetch regardless of widget count (or ZERO when a server seed is embedded), `subscribe`/`onError`/`current`/`push`, shared rAF, reduced-motion, self-destroy on last unsubscribe. The `worldstate` **widget** (`static/js/widgets/worldstate.js`, `Chronicle.register`) drives the SHARED engine (`cal-almanac.js`) via `window.__calSetWorldState` — engine reused, not rebuilt. Rendering canvas reuses the already-exempt `cal-almanac.css` (did NOT duplicate the marked exempt CSS). Tests: `test/js/worldstate_provider.test.mjs` + `worldstate_widget.test.mjs` + Go block tests; widget docs in `static/js/widgets/worldstate.ai.md`. **Merged (PR #415).** Wave-4 per-entity configurable attachment remains OUT of scope (post-deadline widget framework).
- **C-ENTITY-PERMISSIONS-UX** — three entities-plugin presentation changes (one PR): (1) entity card's single **3-state visibility badge** (Everyone `fa-globe` / DM-Only `fa-lock` / Custom `fa-shield-halved`), Scribe+ gated, cards only — `entityVisibilityBadge` in `entity_card.templ`; (2) **inline permission editor** — `permissions.js` gains an opt-in `data-layout="inline"` (the edit-form mount uses it) that renders the widget as a compact summary row expanding in place via the `grid-template-rows 0fr↔1fr` animation (reduced-motion safe), reusing 100% of the grant/load/save path (C-PERMISSIONS-SAVE-FIX intact); slide-in card unchanged for other callers; (3) read-only **Category › Sub-category lineage** in the edit form (`entityTypeLineage`), with `ParentTypeName` now populated via a LEFT JOIN in `entityTypeRepository.FindByID`. Tests: `entity_permissions_ux_test.go` (badge states + player-hidden, inline-layout opt-in, lineage with/without parent) + `test/js/permissions_inline.test.mjs` (inline build/expand/collapse/disclosure + slide-in regression). **Merged (PR #416).** Visual feel of the inline expand is the operator's local gate.
- **C-MAPS-EDITOR-PIN-AND-ICON-PARITY** (operator priority #3) — Chronicle-side of a cross-repo dispatch. **Part A (icon parity):** `internal/plugins/maps/marker_icons.go` is now the canonical source of truth for the 39-icon marker vocabulary; the editor picker renders from `MarkerIconGroups()` and `GET /campaigns/:id/maps/marker-icons` exposes `{default,icons,groups}` as the contract the **Foundry** sync module aligns to (Chronicle authoritative — Option 1 / §A4). **Part B (pin editing in-editor):** double-clicking the map (Scribe+) drops a pin without the separate "Place Marker" toggle (`doubleClickZoom` disabled to avoid the zoom conflict; toggle + marker-list management preserved); shared `MapEditorBody` → applies to the full map page AND the entity-page embed. Tests: `marker_icons_test.go` (catalog integrity/validation/groups, select-from-catalog, inline-create affordances, player-gating, marker-icons API). **FLAGGED:** the Foundry-side translation table (`scripts/map-sync.mjs`) + the create→Foundry→edit→Chronicle round-trip are a **separate Foundry repo/PR** (out of this session's repo scope) — can't be built or round-trip-verified here. **Merged (PR #417).** Inline-pin gesture feel is the operator's local gate.
- **C-AUTH-LOGIN-CSRF-FIX** (login-blocking, HIGH) — fresh logins were failing the double-submit CSRF check with a raw "invalid CSRF token". **Root cause:** `internal/middleware/csrf.go` named the cookie by scheme (`__Host-chronicle_csrf` HTTPS / bare HTTP); behind a TLS-terminating proxy the scheme derived on the POST could differ from the GET that set the cookie, so the lookup missed it and compared the form token against a fresh value → 403. **Fix:** read the cookie under BOTH names (`readExistingCSRF`) + hardened `schemeIsSecure` (parses comma-list `X-Forwarded-Proto`). **Part B:** friendly no-jargon 403 (`middleware.CSRFFriendlyMessage`, flows through `ErrorPage`/HTMX toast) + login auto-recovery (stale-token login POST → `GET /login?expired=1` via HX-Redirect/303 → re-issues token + friendly banner). Tests: `internal/middleware/csrf_test.go` (set→submit, both scheme-flip directions, recovery HTMX+plain, friendly-403, API skip). **Shipped.** ⚠️ Operator confirmed a real proxied login post-merge (CI can't reproduce the proxy/scheme condition).
- **C-APPS-CAL-DASH-W1** (E1 Wave 1 of 4) — the Calendars management dashboard as a **dedicated page** (`GET /campaigns/:id/apps/calendar`, Owner), reached from the Extensions hub's per-app "Open dashboard" entry for calendar (now a dedicated-page link via `campaigns.ExtensionDashboardPageURL`; the inline-panel mechanism stays for apps without a dedicated page). **List + detail-pane**: left list via `ListCalendars`; right detail **composes** the existing CRUD (open / settings / setup-wizard / delete / active-switch — no new CRUD) + a **read-only associations panel**. Two new reads, **no migrations**: `EntitiesForCalendar` (entity-ties, joined through `calendar_events`/`calendar_eras` since the link tables carry no calendar_id — corrects the audit) + timeline `ListByCalendar`→`ListTimelinesForCalendar`, exposed to calendar via a **service-interface adapter** (`calendar.TimelineLister`, wired in `app/routes.go` — no repo cross-import, respects plugin isolation). Friendly empty/error states (#413 pattern). Files: `calendar/app_dashboard.{go,templ}`, `entity_ties_repository.go`, `timeline/{repository,service}.go`. Tests: `calendar/app_dashboard_test.go` (+ `EntitiesForCalendar` passthrough), `timeline/list_by_calendar_test.go`, updated hub tests. **Merged (PR #419).**
- **C-APPS-CAL-DASH-W2** (E1 Wave 2 of 4) — live "see in action" embeds in the W1 detail pane, reusing shipped surfaces (no new widgets): LIVE worldstate band (`worldStateBandV2` — the production sky+hourglass the #415 block also wraps) **only when the selected calendar is the campaign's ACTIVE one** (engine-singleton nuance DEFAULT — non-active shows a friendly "set active" note, no widget surgery, no stop-flag needed); the engine-free month grid lazy-loaded via the existing `/calendars/:calId/embed` route (any calendar); per-associated-timeline `timeline-viz` widget mounts (reusing the shipped widget; D3 loaded at page level when timelines exist). **Design call (flagged):** dashboard selection is now a **full navigation** (list rows are plain hrefs, not HTMX detail swaps) because `htmx.config.allowScriptTags=false` means engine/D3 `<script>`s in a swapped fragment won't run, and the engine inits from its seed at load — full-load makes "one live worldstate surface + clean teardown" automatic. No new routes/migrations. Tests: `calendar/app_dashboard_test.go` (active-vs-non-active worldstate branch, grid lazy-load, timeline previews, D3 gating, full-nav rows); reused-widget mount/teardown is covered by the existing `worldstate_*`/boot.js lifecycle. **Merged (PR #420).** Visual feel is the operator's local gate.
- **C-WIDGET-BINDING-P1-SPINE** (the real Wave-4, P1 of 4; supersedes E1 W3/W4 — W1/W2 stand) — generic **host ↔ widget-type ↔ data-instance** binding. New **`widgetbindings` plugin**: a polymorphic **FK-free** `widget_bindings` table (`host_type` ∈ entity/entity_type/dashboard from day one; `widget_type` = registry slug; unique per host+type), a declarative `WidgetType` **Registry** (the dynamic-not-hardcoded answer; namespace guarded in app code, no DB enum), and a `Service` whose **precedence resolver** runs own-binding → entity-type template → **default (= today's behavior)** and returns `{InstanceID, Source}`. **Integrity kit (AND, not OR):** per-plugin delete hook (`OnInstanceDeleted`) + always-on render-time orphan guard (in `Resolve`) + periodic `Sweep`; **campaign scope** is in the repo signature (unscoped read unrepresentable) and checked on **host AND resolved instance** (the leakage vector; MariaDB has no RLS). **Calendar retrofitted** (`calendar_widget_type.go` + `EntityCalendarBlock` takes a resolved `calendarID`; unbound resolves to the campaign default = identical to pre-framework → #411–#420 unchanged). Built-now-dormant: the **entity-type template inheritance** path (unit-tested, surfaced as data in P4). **ADR-038** records why polymorphic-FK-free (the core-before-plugin migration rule makes the integrity-preserving alternatives un-collectable + forces per-type schema churn we forbid). Tests: `widgetbindings/service_test.go` (CRUD, precedence, **directional cascade** type≠>own per Foundry #9818, orphan guard ×3, campaign-scope on bind+resolve, source layer, all-host-types-storable). Sanitize snapshot +1 line (no HTML surface). **Merged (PR #421).**
- **C-WIDGET-BINDING-P2-WORLDSTATE-TIMELINE** (P2 of 4) — retrofits **worldstate** + **timeline** onto the framework as WidgetTypes (no new tables). **Worldstate's instance is a calendar id** (it's a view over a calendar's clock) under a **distinct `"worldstate"` slug** (shares `calendarInstanceBacking` with calendar), so a host can point its hourglass at a different calendar than its calendar embed. **Timeline's instance is a timeline record**; because the entity timeline block is a campaign-level **preview list** (not a single timeline), the timeline type has **no single default** — unbound keeps the list, bound renders the one timeline (via the shipped `timeline-viz` mount). `EntityWorldStateBlock(svc,cc,userID,calendarID)` + `BlockTimeline(cc,timelineID)` take a resolved id; unbound = identical to today. **Delete hooks wired** (P1 left them unconnected): a `BindingCleaner` interface injected into the calendar + timeline services via a type-asserted `SetBindingCleaner` (interfaces unchanged) → `DeleteCalendar` sweeps `calendar`+`worldstate` (both reference calendar ids), `DeleteTimeline` sweeps `timeline`. `InstanceExists` hardened with `errors.As` (the services wrap not-found via `%w` and `apperror.SafeCode` doesn't unwrap → a wrapped 404 must still be sweepable). Two new slugs are append-only registry entries (no schema). Tests: `calendar/widget_type_test.go` + `timeline/widget_type_test.go` (slug, in/cross-campaign + not-found + transient-error guard, default-vs-no-default, delete-hook per type). **Merged (PR #422).** P3 maps + dashboard-as-host · P4 create-or-pick UI.
- **PC-CLAIM-2** (Player Character Claiming, Stage 2 of 4) — addon + claimable flag. Registered the `player-character-claiming` built-in addon (CategoryPlugin/StatusActive, `fa-user-check`; startup seeder upserts it). Migration `000029` adds `entity_types.claimable BOOLEAN NULL AFTER parent_type_id` (NULL=unset→legacy heuristic; TRUE/FALSE=explicit Owner choice). `Claimable *bool` plumbed through `EntityType` + create/update inputs & form DTOs. **Repo hardening:** collapsed the former six-way entity_type SELECT/scan duplication into a single `entityTypeColumns` const + `scanEntityType(rowScanner)` helper every read routes through (`FindByID` resolves `ParentTypeName` via a follow-up PK lookup so it shares the one scan path) — removes the scan-order drift footgun the dispatch flagged; `claimable` added to the Create INSERT + Update SET. `isClaimableType` honours the flag (explicit wins; NULL → preset/slug heuristic). `CreateEntityType` gates player-character sub-types (`PresetCategory=="player_character"` or `slug=="player-character"`) on the addon via a service-injected `AddonChecker` (`entityService.SetAddonChecker`, wired in `app/routes.go`), rejecting with an "enable the Player Character Claiming addon" apperror when off and defaulting `claimable=true` when on. Tests: table-driven `isClaimableType` (flag precedence) + `CreateEntityType` gate in `service_test.go`, and a live-DB `TestEntityTypeRepository_Integration` (all six reads + INSERT/UPDATE) so `make test-int` proves the new scans. Verified `templ generate` / `go build ./...` / `make test-unit` (43 pkgs) / `make test-int` / `migrate-up`+`down` round-trip. **Merged (PR #481).** Next: Stage 3 UI (per-type toggle, owner overview) · Stage 4 Foundry actor-sync mapping.
- **PC-CLAIM-3** (Player Character Claiming, Stage 3 of 4) — the UI for the addon + flag. **Owner on the character page:** the claim block is extracted to `claim_banner.templ` (`claimBanner`) and now shows "Claimed by &lt;DisplayName&gt;" when owned (Show handler resolves `owner_user_id` → `CampaignMember.DisplayName` via the existing `memberLister`, "a player" fallback for a stale owner). **GM owner overview:** new `category_owner_roster.templ` (`claimRosterPanel`) renders on a claimable category dashboard for **Scribe+** — every character with its owner, a reassign `<select>` (members) and an Unclaim button, both PUT `…/entities/:eid/owner` via `Chronicle.apiFetch`; the Index handler assembles a `ClaimRoster` (full type list capped 100 + members + owner-name map) only when Scribe+ AND `isClaimableType` AND addon-on, threaded through `CategoryDashboardContent` *inside* `#entity-list` so the existing search/sort outerHTML swap is preserved. **Per-type toggle:** `EntityTypeCard` + the Add-Category form gain a "Players can claim entities of this type" checkbox **only when the addon is on** — the quick-edit save rides `claimable` on its PUT (initialised from the effective `isClaimableType`), the create form posts `claimable=true` when checked; `EntityTypesPage`/`CreateEntityType` compute `claimingEnabled`. **Claim gating:** the unclaimed banner now requires addon-on **AND** `isClaimableType(type)`. Plugin isolation preserved (reused the in-package `AddonChecker` + `memberLister`; no new cross-plugin import). Tests: `claim_overview_test.go` (pure helpers + isolated component renders for the banner/roster/type-card gating). Verified `templ generate` / `go build ./...` / `make test-unit` (43 pkgs) / `go vet`. **Merged (PR #482).** Next: Stage 4 Foundry actor-sync addon-aware mapping.
- **C-WIDGET-BINDING-P3a-MAPS** (P3a of 4) — read-side maps retrofit; maps is the original `entity.map_id` precedent the framework generalizes. New `maps/map_widget_type.go` registers a `"map"` WidgetType (instance = a map id; `InstanceExists` = `GetMap` + campaign-scope, `errors.As` wrapped-404 guard; **no campaign default** — the legacy fallback lives in the closure). The `map_editor` block closure resolves the map id through the framework with a **legacy fallback**: default = today's `entity.map_id`; a `widget_bindings` row (widget_type="map") **wins** when present; **unbound = identical to today** (column drives the embed/picker/empty branches). `DeleteMap` delete-hook wired via the same type-asserted `SetBindingCleaner` seam (interfaces + mocks untouched); the legacy `entity.map_id` is independently SET-NULLed by `fk_entities_map_id` (maps migration 005). **No migration, no schema change** — the `entity.map_id` backfill→bindings + column drop is a **deferred** span-the-layers migration (after P3b). Tests: `maps/map_widget_type_test.go` (slug, in/cross-campaign + not-found + transient guard, no-default, binding-wins, delete-hook). **Shipped.** Deferred: **P3b** dashboard-as-host (unify `DashboardBlockSwitch` onto `BlockRegistry.Render` so `host_type='dashboard'` resolves) · the `entity.map_id` migration · **P4** create-or-pick UI.
- **C-CAL-WORLDSTATE-WIRE** (2026-07-26) — `calendar.worldstate.changed` + `calendar.weather.zones.changed` were **publisher-side dead letters**: live emitters (`worldstate_service.go` `SetWorldState`, `service.go` `SetWeatherZones`) but no `case` in `calendarEventPublisherAdapter.PublishCalendarEvent` (`internal/app/routes.go`), so both hit its `default: return` and were discarded before the bus. Every meteor, eclipse and weather-zone edit the operator authored had never once reached a WebSocket client — which is why "celestial events unfindable/unsyncable" had no trace to follow. Traced by the Foundry executor in Chronicle-Foundry-Module PR #82. **Three additive parts:** (1) new public types `ws.MsgCalendarWorldstateChanged` / `ws.MsgCalendarWeatherZonesChanged` + the adapter cases, with `internal/app/routes_calendar_ws_test.go` pinning the WHOLE calendar mapping (a `default: return` is a silent-drop machine — the next emitter added without a case must fail CI, not a session); (2) `WorldStateChangePayload` — a strict superset of the old `{date, moodTint}` (those paths preserved because the module shipped `formatWorldstateLine` against them) adding celestial `events[]` with the **stable `calendar_celestial_events` id**, `moons[]`, `weather` and an `audience` marker, **SPLIT BY AUDIENCE**: player-safe always, plus a full copy under the internal `calendar.worldstate.changed.dm` name that the adapter maps to the same public type with `RequiresDM` set (the hub gates per-message, so one message cannot be rich for the GM and redacted for the table); (3) `GET /api/v1/campaigns/:id/calendar/world-state` on the Bearer syncapi group, dm_only-gated by `resolveRole` exactly as every other syncapi calendar read. Enrichment is best-effort — a failed load degrades to the minimal payload rather than dropping the signal. `WorldStateEvent` gained `id` + `visibility`; the showcase parity test is unaffected (it pins top-level + moon keys). **ADR-047**; `internal/plugins/calendar/.ai.md` §"World State on the Wire"; `internal/plugins/syncapi/.ai.md`. Tests: `worldstate_wire_test.go`, `routes_calendar_ws_test.go`, `calendar_worldstate_handler_test.go`; `routes_snapshot.txt` +1.
- **C-CALV4-TIEFIX-PB** (2026-07-26, calendar-v4 wave 1 verify-then-fix carve-out) — two live bugs under the v4 remodel's entity-page calendar Block, both confirmed by source reading before fixing. **Bug 1:** `EventsForEntity` (`entity_ties_repository.go`) scanned 37 destinations against 39 selected columns (`eventCols`' 38 + `l.participation_role`) — missing `&evt.RecurrenceDayOfWeek`/`&evt.CollectRSVPs`, the same landmine class C-CAL-RSVP-P1 closed in `scanEvents` but never ported to this file — so every entity-side tie read failed at runtime and the entity-page calendar embed silently showed no tied events. Fixed + `event_scan_contract_test.go` widened (now file-parameterized) to cover this third consumer, proven to fail pre-fix / pass post-fix. Also added `EventsForEntityFiltered` (new method, existing signature untouched) closing a calendar-level visibility gap this path never enforced (a `dm_only` *calendar*'s event was reachable through an entity tie) — composed from existing repo methods rather than widening `CalendarRepository`, per COMMON's Bounds; not yet wired into `entity_calendar_block.go` (owned by a different slice). **Bug 2:** timeline `EventCount` (`internal/plugins/timeline/repository.go` `List`/`ListByCalendar`) used unconditional `COUNT(*)` subqueries while the row-returning `ListEventLinks`/`ListStandaloneEvents` filtered by visibility — a player-visible count/row mismatch that differenced into a count of hidden events. Fixed by folding the same predicate into both subqueries; **PARTIAL** — `visibility_rules` (Go-side `canUserView`) remains a residual gap SQL can't see, stated honestly per COMMON ruling #9, booked for the coordinator. **§18-C confirmed already closed** (PR #565) — not re-fixed. See `internal/plugins/calendar/.ai.md` §"Entity-side tie reads" and `internal/plugins/timeline/.ai.md` §"EventCount visibility fix".
- **C-CALV4-BENCH-P4** (2026-07-28, calendar-v4 wave 1 phase B) — the nav Calendar tab lands on **the Bench**. `GET /campaigns/:id/apps/calendar` keeps its path exactly (the nav target and the Extensions hub both carry it as bare strings, one pinned literally; a path change forces a `routes_snapshot.txt` regeneration) and `AppDashboard` becomes a thin handler delegating to new files: `calendar/bench.{go,templ}`, `bench_test.go`, and the unlayered self-contained `static/css/calendar-bench.css`. **The proportion rule is structural:** `benchClassify` promotes ONE primary calendar and AT MOST one real-world calendar to full Blocks; everything else is a subordinate row, at every width and under every sort key. **Permission is absence, and counted:** a player's ribbon is three tiles (Today / Next up / Session) — Sync, Needs attention and Horizon are never BUILT for them, so there is nothing in the DOM to grey or hide. **Honesty states:** the fault prints where the date would go with no date element (the warn TREATMENT is GM-only, the ROW is not — a player's own misconfigured calendar must not vanish silently); design-ahead surfaces carry the signed `needs backend` chip instead of the mockup's fabricated "RSVP 3 / 5", "9 days fogged" and hardcoded "all 11"; the sync denominator never drops in any state. **NEXT UP is viewer-filtered at the source** via P2's `UpcomingAcrossCalendars` — never `UpcomingByCalendar`, which filters base visibility in SQL only and leaks a `visibility_rules` event's NAME to a player — and `benchSortKeys` feeds the shipped `?sort=nextevent` control from that same filtered index rather than reopening the leak. **No N+1** on the page every player lands on: hydration goes through the spine's batched `EagerLoadCalendars` rather than the nine-queries-per-calendar `eagerLoad`. The owner/player list split (`ListCalendars` vs `ListVisibleCalendars`) is preserved verbatim. Host layer set = moons/eras/weeknums/ledger/shelf per the DEF ruling (`cordinator decisions/2026-07-28-calv4-def-and-zone-chips-ruling.md` §1); `moongraph`/`horizon` omitted and measured, not assumed. The four `app_dashboard_*_test.go` pin sets refreshed intent-first (route-page assertions now render the Bench; retained-but-unrouted card-grid component tests marked as such, never deleted). No new routes, no migrations, `data.go` byte-pin clean, `routes_snapshot.txt` byte-identical. Headless fidelity shots at 1232px and 358px, light and dark, GM and player. **Booked, not fixed:** `/apps/calendar` rides the authenticated group while `/calendar/v2` is public-capable, so an anonymous visitor to a PUBLIC campaign lands on `/login` — a route-group change plus a snapshot regeneration, i.e. a wave-2 slice. See `internal/plugins/calendar/.ai.md` §"The Bench".
- **C-CALV4-SHELF-P7** (2026-07-28, calendar-v4 wave 2, W-E) — **zone D is filled and the Block is architecturally complete.** `shelf_stub.templ` → `shelf.templ` (marker kept: the seam suite keys on `data-zone="shelf"`) plus a new `almanac.templ`. **Upcoming** reuses the Ledger's row primitive VERBATIM off the same `newLedgerView` pass — month-scoped exactly as drawn ([S1]), capped at four later rows and SAYING it capped — so the count oracle's new seventh reading is an equality rather than a hope. **Filters** ships the tab and not the engine ([S2]): nothing in Chronicle backs a filter and no preference store exists, so the panel is one `needs backend` chip with no controls and no pin field, and W-F is NAMED as the filling wave. A player gets neither the tab nor the panel ([S10]) — the needs-backend-audience ruling one level down, as absence rather than a disabled control, and the first per-role difference inside a chrome strip. **The Almanac** is the slice's gate and L21's other half: pin **r53** adds `MonthGeometry.Almanac`, a per-month celestial register produced **UNCAPPED by `MoonCap`** ([S5]) — the grid's three-moon ceiling is legitimate only because every declared body is carried here at full width, and before this the fourth moon existed in the database, was counted by `MoonsDeclared`, and was drawn nowhere. Three sub-tabs (Tonight · Month · Moons), every number computed from the month's REAL day count: the mockup's four thirty-day literals are FIXED and not ported, and `TestCSS_NoLiteralWeekLength` now covers the lane, which takes its day columns from `grid-auto-flow` so no month length exists as a literal or a variable `repeat()` anywhere in the sheet. No epithet ([S6]) and no node-window bracket — neither has a column. The nameplate badge's `— all of them are in the Almanac` tail is RESTORED and gated: a Block with the Shelf hidden (`noShelf`) or the shelf layer off still withholds it. **Three findings the string suites could not see**, all caught by a new browser probe written red-first: the §12 std collision came back with a filled Shelf and was answered inside the Block's own std geometry (the Shelf yields; no host layer key dropped, no pinned host file opened); W-B's day-pick ladder FILTER reached the Shelf's rows and made choosing a day reflow the Block 618→532px (one word, plus a guard that now checks the filter selector); and the strip clipped its last sub-tab at 358px instead of scrolling. Plus a harness defect: W-B's probe isolated only the day radio group, so with a third group on the page every box but the last measured a collapsed Shelf — the collision gate was measuring a state the product cannot reach. **DEF still `["moons"]`, both host layer sets unchanged, no new zone flags, no routes, no migrations, no JS**; `bench_test.go`'s chip pin re-stated as an ENUMERATION ([S12]) naming every surviving chip by file:line. ADR-048 §10-§14. See `internal/widgets/calendar_block/.ai.md` and `internal/plugins/calendar/.ai.md` §"calendar-v4 Shelf + the Almanac register".

### Archive

`.ai/archive/` holds historical docs that have served their purpose:

- `status-2026-04-25-pre-shrink.md` — the 1198-line chronological session log that lived at `.ai/status.md` until 2026-05-23. Pre-Phase-4 session recaps live here.
- `phase-d-plan.md` — Phase D sprint plan (Phase D shipped)
- `security-audit-2026-03-06.md` — the original security audit (superseded by `cordinator/reports/chronicle/2026-05-22-c-security-audit.md`)
- `plan.md` — Sprint V-2 (backlinks panel + entity aliases) implementation plan (work shipped)
- `plan-drawsteel-2026-03.md` — Draw Steel system module implementation plan (work shipped; moved here from a stray repo-root `plan.md` by C-DOC-DRIFT-REFRESH-R2)
- `todo-completed-2026-06-10.md` — completed-todo archive (moved 2026-06-10)

### IMPORTANT RULES (mirrored from CLAUDE.md)

Per `cordinator/decisions/2026-05-19-dispatch-workflow.md`:

1. Session-work deliverables → committed PR on the target repo + a Cordinator status report (`reports/chronicle/YYYY-MM-DD-<dispatch>.md` on the Cordinator repo's `main` branch).
2. Plugin-scoped status updates → append to the owning plugin's `.ai.md` "Recent Work" section. Don't bloat this file.
3. Cross-cutting decisions → new file in `cordinator/decisions/` + cite from code.
4. This file's "Cross-cutting state" section gets updated when an arc advances or a release ships.
