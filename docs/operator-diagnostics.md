# Operator Diagnostics

Admin-gated, read-only endpoints that report what a running instance actually
serves: the loaded version of each system, the directory it serves from, and a
content fingerprint of every served file. They answer "Admin > Packages says
v0.13.0, so why does the old widget render?" without shell access.

They are a catalog of small named checks. Run one at a time and paste the
result to an AI assistant, or use the in-app
[AI helper](#the-in-app-ai-helper-batch) to pick several and run them as one batch.

Source: `internal/systems/health.go`, `operator_diag*.go`, `operator_batch.go`,
`operator_picker.go`; page in `internal/plugins/admin/diagnostics_workspace.templ`
and `diagnostics_picker.templ`; routes in
`internal/app/routes.go` and `internal/plugins/admin/routes.go`.

## What it catches

A package install needs three things to line up: the new version is extracted
to disk, the in-memory registry picks it up on rescan, and the browser fetches
the new bytes. Admin > Packages only knows what it installed, not what the
loader serves.

- `loaded_version` differs from the installed version: the registry never
  picked up the install; rescan or restart.
- `loaded_version` matches but a file hash is old content: bad extraction, or a
  duplicate version folder shadows the new one.
- A file is `MISSING`: bad extraction.

## Endpoints

Both are on the admin group (`/admin`): admin-gated, read-only.

### `GET /admin/extensions/health` (JSON)

Health of every loaded system, for tooling or comparing two installs.

```json
{
  "systems": [
    {
      "id": "drawsteel",
      "name": "Draw Steel",
      "loaded_version": "0.13.0",
      "source": "package",
      "dir": "/app/data/media/packages/systems/drawsteel/0.13.0",
      "files": [
        {
          "path": "manifest.json",
          "exists": true,
          "size": 4821,
          "sha256": "9f1c2a7b3d4e5f60",
          "mtime": "2026-06-24T18:02:11Z"
        }
      ]
    }
  ]
}
```

| Field | Meaning |
|---|---|
| `id` / `name` | Loaded system id and display name. |
| `loaded_version` | Version the loader serves; may differ from the installed version. |
| `source` | `package` (Admin > Packages) or `bundled` (found in the server's own systems directory at startup; usually absent). |
| `dir` | Directory the loader serves from. |
| `files[]` | One per served file: manifest, declared widget scripts, text-renderer files. |
| `files[].exists` | `false`: the served dir lacks the file. |
| `files[].size` | Bytes. |
| `files[].sha256` | First 16 hex chars of the SHA-256. |
| `files[].mtime` | RFC3339 UTC. |

### `GET /admin/diagnostics` (markdown)

Returns `text/markdown`. With no `?name` it returns the catalog menu; with
`?name=` it runs one diagnostic. An unknown name returns 404 pointing at the
catalog.

```
GET /admin/diagnostics                          # catalog menu
GET /admin/diagnostics?name=system.versions
GET /admin/diagnostics?name=system.files&arg=drawsteel
GET /admin/diagnostics?name=system.health       # full dump, opt-in
GET /admin/diagnostics?name=probes              # probe library
```

Running `?name=system.versions`:

```markdown
## system.versions

- `drawsteel` v**0.13.0** (package) — `/app/data/media/packages/systems/drawsteel/0.13.0`
```

## Diagnostics

The authoritative list is `diagnosticCatalog()`; `GET /admin/diagnostics`
prints it. Request one at a time; `system.health` is the full dump and is
opt-in.

### `host.*`: which build is serving

`system.*` and `packages.*` describe what is served; `host.*` identifies the
process serving it, and avoids two misreadings:

1. **Image label read as process identity.** `docker inspect <tag>` describes
   whichever image holds the tag now, not the one the container was created
   from. `host.build` reads identity from inside the process.
2. **Empty `grep /app/static` read as missing code.** Front-end files come from
   two places: the on-disk static root, and each plugin's `//go:embed` filesystem
   compiled into the binary and served at `/static/plugins/<slug>/`. Only the
   first is greppable, so an empty grep for a plugin asset is expected.
   `host.embedded` lists the compiled-in assets; `host.assets` and `host.widgets`
   report the two scopes separately.

| `name` | Arg | What you get |
|---|---|---|
| `host.build` | none | "Did my deploy land?" Source revision from VCS stamps (or "not stamped"; absent is not stale), `CHRONICLE_VERSION`, executable path/size/mtime, Go toolchain, process start and uptime, hostname, PID. |
| `host.deploy-check` | `[<marker[,marker2]>]` | Run after a deploy: build identity, bellwether assets that change on nearly every build, installed-vs-loaded package summary. Markers are searched in the on-disk root and embedded assets, reported separately. |
| `host.runtime` | none | Uptime, goroutines, NumCPU/GOMAXPROCS, memory, GC activity. |
| `host.errors` | `[<count>]` | "What broke overnight?" Newest first: time, age, status, method, route template, error. What is recorded: ADR-051. |
| `host.errors-summary` | none | The same ring grouped by route and status with counts and first/last seen; usually the better first read. |
| `host.assets` | `[<path-substring>]` | "Is my new CSS/JS served?" Per file: size, sha256[:16], mtime, served `?v=`; flags a served token that does not match the on-disk bytes. `FullDump`: hashes every file listed. |
| `host.asset-contains` | `<relpath>:<marker[,marker2]>` | Marker check on one on-disk file (content, not just hash). Paths outside the static root are refused. |
| `host.embedded` | `[<plugin-slug>]` | Every `//go:embed` plugin asset the binary serves. |
| `host.embedded-contains` | `<slug>:<relpath>:<marker[,…]>` | Marker check on compiled-in assets. |
| `host.widgets` | `[<name-substring>]` | Widgets have no version, so identity is content fingerprint plus build time. Walks both storage mechanisms and names which each result came from. |
| `host.plugins` | none | Per plugin: static mount and URL prefix, embedded asset count/size, whether it has migrations, applied vs available schema version. Chronicle has no plugin loader; a missing row is not a missing feature. |

`host.embedded`, `host.widgets`, `host.plugins`, `host.errors` and
`host.errors-summary` read app state through providers (`systems.Set*Provider`,
wired in `RegisterRoutes`). An unwired provider prints "provider not wired",
which is not the same as "none". `internal/app/operator_diag_wiring_test.go`
fails CI if a declared provider has no call site on the boot path.

### Other diagnostics

Cheapest and most common first.

| `name` | Arg | What you get |
|---|---|---|
| `system.versions` | none | One line per loaded system: id, served version, source, served dir. First check for "is the new version live?" |
| `system.files` | `<system-id>` | `size · sha256[:16] · mtime` per widget/manifest file; gone files show `MISSING`. No arg lists loaded ids. |
| `system.health` | none | Full served-reality dump (all systems, all file fingerprints). Large. |
| `packages.installed-vs-loaded` | none | Each installed package's DB version against what the loader serves (matched by install path); flags `NOT loaded` and `MISMATCH`. |
| `packages.on-disk-versions` | none | Every on-disk version folder per package, tagged `[installed-db]` and `[LOADED]`; shows a stale folder shadowing the newest. |
| `packages.prune-preview` | none | Dry run: version folders safe to delete (all but newest, DB-installed and loaded), with sizes and reclaimable total. Deletes nothing. |
| `systems.load-events` | none | Loader's in-memory event log, newest first: `discovered`, `skipped` (duplicate ignored, with reason), `failed`. |
| `system.file-contains` | `<system-id>:<relpath>:<marker[,marker2]>` | Whether a served file (clamped to its system dir) contains each marker, e.g. `drawsteel:widgets/character-sheet.js:playEntrance`. |
| `campaigns.list` | none | Campaigns with id, name, slug. Entry point for `entity.*` and `campaign.*`. |
| `entity.types` | `<campaignId>` | Entity types: id, slug, preset category, entity count. |
| `entity.find` | `<campaignId>:<nameQuery>` | Entities by name/slug: id, name, slug, type. |
| `entity.fields` | `<campaignId>:<entityIdOrSlug>` | One entity's stored field map (redacted, value-capped). Empty `fields_data` is the "renders blank" signature. |
| `entity.sync-mappings` | `<campaignId>:<entityIdOrSlug>` | Links to an external (Foundry) actor: external system/id, chronicle id, last sync. No mappings means nothing syncs to it. |
| `entity.field-coverage` | `<campaignId>:<typeIdOrName>` | Per entity type, how many declared fields are non-empty across its entities (emptiest first, with %). |
| `campaign.surfaces` | `<campaignId>` | Which calendar route a URL renders, read from the live Echo table, flagged CURRENT / LEGACY / REDIRECT, plus sidebar link targets. |
| `campaign.config` | `<campaignId>` | Enabled addons and block types in `dashboard_layout` / `owner_dashboard_layout` and each entity template: hand-placed or seeded. |
| `calendar.stats` | `<campaignId>` | Calendar plugin state, counts only: addon enabled, calendars, events, moons, eras, event kinds, plugin schema health (applied and latest migration), Foundry sync state. |
| `sync.inbound` | `<campaignId>:<entityIdOrSlug>` | Latest inbound payloads from an external client (e.g. Foundry) for this entity. In-memory ring; resets on restart. |
| `sync.recent` | none | Latest inbound payloads across all entities: "is anything syncing?" |
| `probes` | none | The probe library (below). |

Three-way trace for a missing value: `sync.inbound` (Foundry sent it) ->
`entity.fields` (Chronicle stored it) -> `entity.field-coverage` (schema
declares it). Arrived but not stored is a storage bug; never arrived is a
mapping gap; stored but blank on the sheet is a rendering problem.

`campaign.*` answers why this campaign looks as it does, not which code runs.
"Campaign provider not wired" means nobody was asked; fix the wiring before
concluding anything.

## Probes

`probes` returns commands for state the server cannot report: what the browser
loads, what is on disk, what the logs say, which image the container runs.
Chronicle never executes them; the operator runs them and pastes the output
into the `PASTE OUTPUT BELOW:` slot. Each declares where it runs: `docker`
(host shell), `browser-console` (DevTools), `sql` (DB container) or `url`
(admin URL).

Placeholders: `<chronicle>` / `<db>` container names, `<media>` the
in-container media path (from `system.versions`), `<campaignId>`.

Two probes are **TRAP**s: operators reach for these commands anyway, so each
prints the command with the way it misleads and the `host.*` diagnostic that
answers better.

| ID | Where | What it tells you |
|---|---|---|
| `image-digest` | docker | **TRAP.** Image the container runs vs image the tag points at now; if they differ, labels describe a different artifact. Prefer `host.build`. |
| `plugin-asset-grep` | docker | **TRAP.** Greps the container filesystem for a plugin asset; empty for every `//go:embed` asset, so it proves nothing. Prefer `host.embedded` / `host.embedded-contains`. |
| `container-restart-time` | docker | Whether the container was recreated for this deploy; a new image changes nothing until the container is. |
| `binary-in-container` | docker | Executable mtime and container clock from outside the process: cross-checks `host.build` and rules out clock skew. |
| `served-widget-version` | browser-console | `?v=` on each served widget URL. If it lags Admin > Packages, the registry never picked up the install. |
| `page-asset-tokens` | browser-console | Every versioned asset URL on the page, i.e. what the browser holds. |
| `served-widget-content` | browser-console | Fetches a served widget and checks for a marker: new build or cached copy. |
| `package-version-dirs` | docker | Every installed version folder; several means a stale one may shadow the newest. |
| `package-file-marker` | docker | `grep -rl` for a new-build marker across install dirs: which folder has the new code. |
| `chronicle-logs` | docker | Package install, "replacing system with preferred copy", "ignoring duplicate system" and boot rescan lines. |
| `plugin-schema-versions` | sql | Applied plugin migrations from the DB; cross-check `host.plugins`. |
| `packages-db-state` | sql | `packages` table's installed/pinned system versions and install paths; cross-check `packages.installed-vs-loaded`. |
| `entity-type-tree` | sql | Entity types and per-type counts for a campaign: duplicate preset categories. |
| `sync-mapping-orphans` | sql | Sync mappings pointing at deleted entities. |

## Security model

1. **Read-only.** Diagnostics `os.Stat` and hash files the loader already
   serves and read in-process state. They write and execute nothing.
2. **Redaction.** Every output passes through `redactSecrets`, whose regex
   (`secretLine`) replaces the value on any `key: value` / `key=value` line whose
   key contains `password`, `passwd`, `secret`, `token`, `api key`, `access key`,
   `private key`, `authorization` or `bearer` (including prefixed names like
   `DB_PASSWORD`) with `[REDACTED]`. Diagnostics are secret-free anyway; this is a
   backstop against one that echoes a config value later.
3. **Admin-gated.** Routes sit on `adminGroup` in `internal/app/routes.go`,
   inheriting admin auth.
4. **Probes are suggested, never executed.** No code path leads from a
   diagnostic request to docker, shell or SQL execution.

## Adding a diagnostic or probe

The renderer, route and redaction do not change; add one struct.

Diagnostic: append to `diagnosticCatalog()` in
`internal/systems/operator_diag.go`. `renderCatalog` lists it, `RunDiagnostic`
dispatches to it and `redactSecrets` covers it. `Run` must stay read-only.

```go
{
    Name:    "system.something",          // dotted id the assistant requests
    Title:   "Human title",
    Desc:    "One line: what you get / when to use it.",
    ArgHint: "<some-id>",                 // "" if no argument
    Run: func(arg string) string {        // markdown, pre-redaction
        var b strings.Builder
        b.WriteString("## system.something\n\n")
        return b.String()
    },
},
```

Probe: append to `defaultProbes()`. It appears under `?name=probes` with its
`Why`, a fenced command and a paste slot. Use `<placeholder>` tokens for values
the operator fills in.

```go
{
    ID:      "my-probe",
    Title:   "What this probe reveals",
    Where:   ProbeDocker,                 // ProbeDocker | ProbeConsole | ProbeSQL | ProbeURL
    Command: `docker exec <chronicle> ...`,
    Why:     "Why an operator would run this and what the output proves.",
},
```

## Worked example: stale package install

Draw Steel v0.13.0 is installed but the character sheet lacks a feature from
it. Go cheapest first; paste back only the step that surprises you.

1. `GET /admin/diagnostics?name=system.versions`: `loaded_version` below
   installed means the registry never picked up the install (rescan or restart,
   not a file problem).
2. `GET /admin/diagnostics?name=system.files&arg=drawsteel`: no `MISSING` and
   matching hashes mean extraction is fine. A hash matching old content means
   bad extraction; use the `package-*` probes to find the folder.
3. Browser: run `served-widget-version`. An old `?v=` is a stale cached URL
   (hard refresh). `served-widget-content` confirms the fetched bytes carry the
   new marker.
4. Host: if versions agree but the code is old, a duplicate version folder
   shadows the new one. `package-version-dirs`, then `package-file-marker`, find
   which folder has the new code; compare with the served `dir` from step 1.
   `chronicle-logs` shows the loader's own account; `image-digest` rules out a
   stale backend image.

## The in-app AI helper (batch)

`GET /admin/diagnostics/workspace` (Site admin ▸ Health ▸ AI helper) turns
"request a diagnostic, run it, paste it back" into one batch round-trip with a
human approval gate. A request comes from one of three places:

- **An everyday question.** Six questions ("Did my deploy land?", "A sheet or
  page shows blank", …) each unroll a sentence of what Chronicle will look at,
  with a campaign or page blank where needed, and the checks they run, each
  untickable. `DiagnosticQuestions` in `internal/systems/operator_picker.go`.
- **Picked by hand.** "Pick checks yourself" opens every check in plain-word
  folds (`DiagnosticGroups`), with Find ("/" outside a text field) and a tray
  that holds the request and its blanks. A blank left empty becomes the
  placeholder the review step asks about.
- **An outside AI.** "Copy checks for your AI" copies the machine-readable
  functions list; "Paste an AI's request" takes the one batch object it
  composes (a fenced ` ```json ` block is accepted).

The first two post to `/compose`, where `ComposeBatch` writes the same batch
object an AI would, so every source meets the same checks:

1. **Review and approve**: the request is validated against the catalog and
   shows one row per call, flagging unknown names and full-dump requests, in a
   sheet at the foot of the window. Nothing runs until *Approve & run*; a human
   reads the toolset first, which is the prompt-injection boundary. Ticking a
   check marked "large" is the full-dump opt-in.
2. **Copy the result**: one secret-redacted document (manifest of what ran or
   was skipped, each result, byte-count footer).

Request format:

```json
{
  "v": 1,
  "note": "why does Draw Steel serve the old sheet?",
  "full_dump": false,
  "calls": [
    { "name": "system.versions" },
    { "name": "system.files", "arg": "drawsteel" },
    { "name": "packages.installed-vs-loaded" }
  ]
}
```

| Field | Meaning |
|---|---|
| `v` | Spec version, currently `1`; may be omitted. A mismatch is rejected. |
| `note` | Optional; echoed into the result for audit. |
| `full_dump` | Must be `true` for a function marked `full_dump` (e.g. `system.health`). Default `false`. |
| `calls[]` | `{ "name", "arg"? }`, at most 50. |

Rules:

- **Bounded toolset.** Only live-catalog names run; unknown names show as
  skipped. Unknown top-level keys are rejected. Caps: 50 calls, 64 KB paste.
- **Re-validated on run.** Approve re-parses the original request text and
  re-derives runnability and deduplication from the live catalog, so a forged
  client plan cannot smuggle in a gated call.
- **Deduplicated.** Identical `(name, arg)` calls run once; repeats show as
  `duplicate`.
- **Output-capped** at about 256 KB with a truncation notice; the footer gives
  byte size and a rough token estimate.
- **Read-only, redacted, admin-gated** like the catalog. `note`, `name` and
  `arg` are sanitized before they are echoed.
- **Full dump needs both** `full_dump: true` and approval of the plan.
- **Audited.** Each run logs `admin.diagnostics_batch_run` to the admin
  activity feed with actor, IP and counts, never the payload.

Routes (admin-gated, `internal/plugins/admin/routes.go`): `GET
/admin/diagnostics/workspace` (page), `POST /admin/diagnostics/workspace/parse`
(review a pasted request), `POST /admin/diagnostics/workspace/compose` (review
a picked one), `POST /admin/diagnostics/workspace/run` (result). Logic:
`FunctionsSpecJSON`, `ParseBatch`, `RunBatch` in
`internal/systems/operator_batch.go`; `ComposeBatch` and the questions and
groups in `operator_picker.go`. Per-route rate limiting is not
implemented.
