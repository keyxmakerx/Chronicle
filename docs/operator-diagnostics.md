# Operator Diagnostics

Chronicle's **operator diagnostics** are a pair of admin-gated, read-only
endpoints that report *the deployment reality* of a running instance: which
version of each system the loader is **actually serving**, the on-disk
directory it serves from, and a content fingerprint of every served file. Use
them to answer *"Admin▸Packages says this system is v0.13.0, so why is the
old widget still rendering?"* without SSH or shell access to the host.

They are the operator-facing analogue of the campaign **AI-Export**: a
catalog of named, targeted diagnostics you run one at a time and **paste back
to an AI assistant**, so it gets exactly the slice of state it asked for
rather than a giant dump. The **in-app AI Workspace**
(`/admin/diagnostics/workspace`) turns that into a single copy-paste
round-trip — see [The in-app AI Workspace](#the-in-app-ai-workspace-batch).

Source: `internal/systems/health.go`, `internal/systems/operator_diag.go`,
`internal/systems/operator_batch.go`, the workspace UI in
`internal/plugins/admin/diagnostics_workspace.templ` (+ handler/routes in the
admin plugin), and the markdown/JSON endpoints in `internal/app/routes.go`
(all on the admin route group).

---

## The problem it solves

Installing or updating a system package requires three things to line up: the
new version is **extracted** to disk, the **in-memory registry** picks it up
on rescan, and the browser **fetches the new bytes** (not a stale `?v=`
cache). When one silently fails, Admin▸Packages can report the new version
while a stale copy renders — the UI only knows what it *installed*, not what
the loader *serves*. Operator diagnostics report the served reality (loaded
version + served dir + per-file `size · sha256 · mtime`) to prove which build
is live:

- **`loaded_version` disagrees with the installed version** → the registry
  never picked up the install; needs a rescan/restart.
- **`loaded_version` agrees but a file's hash is old content** → bad
  extraction (a botched copy, or a duplicate version folder shadowing the new one).
- **A file is `MISSING`** → a botched extraction.

---

## The two endpoints

Both are registered on the **admin route group** (prefix `/admin`), so both
are admin-gated and read-only.

### 1. `GET /admin/extensions/health` — machine-readable health (JSON)

Deployment health for every **loaded** system, as JSON — for tooling,
monitoring, or scripted comparison between two installs.

```
GET /admin/extensions/health
```

Example response:

```json
{
  "systems": [
    {
      "id": "drawsteel",
      "name": "Draw Steel",
      "loaded_version": "0.13.0",
      "source": "package",
      "dir": "/app/media/packages/systems/drawsteel/0.13.0",
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

Field reference:

| Field            | Meaning |
|------------------|---------|
| `id` / `name`    | The loaded system's id and display name. |
| `loaded_version` | The version the loader resolved — what is *actually being served*, which may differ from the installed version in Admin▸Packages. |
| `source`         | `package` (installed via Admin > Packages) or `bundled` (found in the server's own systems directory at startup rather than installed as a package; usually absent, since systems are external packages). |
| `dir`            | The on-disk directory the loader serves this system's files from. |
| `files[]`        | One entry per served file (manifest + every declared widget script + text-renderer file). |
| `files[].exists` | `false` means the served dir is missing that file — itself diagnostic. |
| `files[].size`   | File size in bytes. |
| `files[].sha256` | First 16 hex chars of the content SHA-256 — enough to compare two installs without shipping the whole file. |
| `files[].mtime`  | File mtime, RFC3339 UTC. |

### 2. `GET /admin/diagnostics` — the diagnostics catalog (markdown)

The human/AI-facing front door. Returns **markdown** (`text/markdown`). With
no `?name`, it returns a tiny **catalog menu** (no payload data); with
`?name=...` it runs exactly one named diagnostic and returns its small,
redacted result.

```
GET /admin/diagnostics                          # the catalog (menu only)
GET /admin/diagnostics?name=system.versions     # run one diagnostic
GET /admin/diagnostics?name=system.files&arg=drawsteel
GET /admin/diagnostics?name=system.health       # full dump (opt-in)
GET /admin/diagnostics?name=probes              # the probe library
```

An unknown `name` returns `404` with a message pointing back at the catalog.
Example catalog (no `?name`):

```markdown
# Chronicle Operator Diagnostics — catalog

Read-only, secret-redacted. The AI assistant names ONE diagnostic; you run it
and paste the (small, targeted) result back. Run with
`GET /admin/diagnostics?name=<name>[&arg=<arg>]`.

- **`system.versions`** — One line per loaded system: id, served version, source, served dir. …
- **`system.files`** `<system-id>` — size + sha256[:16] + mtime of each widget/manifest file …
- **`system.health`** — The complete served-reality dump. …
- **`probes`** — docker / browser-console / SQL / admin-URL commands …
```

Running one diagnostic (`?name=system.versions`):

```markdown
## system.versions

- `drawsteel` v**0.13.0** (package) — `/app/media/packages/systems/drawsteel/0.13.0`
```

---

## The catalog model

`/admin/diagnostics` is a **catalog** of small named checks rather than one
monolithic export, so an AI assistant requests **one named diagnostic at a
time** (e.g. *"run `system.files drawsteel`"*) instead of wasting context on
a giant dump. The operator runs just that and pastes back a small, targeted
result. The full dump (`system.health`) still exists but is **opt-in** —
requested by name only when a targeted diagnostic won't do.

### The `host.*` family — Chronicle fingerprinting ITSELF

Every `system.*` and `packages.*` diagnostic describes **what is being
served**, never **which build is doing the serving** — that gap is what
`host.*` closes, guarding against two misreadings:

1. **A Docker image label read as a running process's identity.** `docker
   inspect <tag>` answers for whichever image holds that tag *now*, never the
   image a running container was created from. **`host.build` reads the
   identity from inside the process instead**, where nothing can have
   relabelled it.
2. **An empty `grep /app/static` read as missing code.** Chronicle serves its
   front end from two mechanisms: the on-disk static root, and each plugin's
   `//go:embed`-ed filesystem compiled *into the binary*, served at
   `/static/plugins/<slug>/`. Only the first is `grep`-able, so an empty grep
   for a plugin asset is **expected**, not evidence. **`host.embedded` lists
   what is inside the binary**; `host.assets` / `host.widgets` report the two
   scopes separately rather than conflating them.

| `name`                   | Arg                                  | What you get |
|--------------------------|--------------------------------------|--------------|
| `host.build`             | —                                    | **THE "did my deploy actually land?" check.** Source revision from compiled-in VCS stamps (or *"not stamped"* — **absent is not stale**), `CHRONICLE_VERSION`, executable path/size/mtime, Go toolchain, process start + uptime, hostname, PID. Notes which fields to trust and why image labels aren't evidence. |
| `host.deploy-check`      | `[<marker[,marker2]>]`               | **The one thing to run after a deploy.** Build identity + bellwether assets that move on almost every build + the installed-vs-loaded package summary. Markers are searched across both the on-disk root and embedded assets, reported separately. |
| `host.runtime`           | —                                    | Uptime, goroutines, NumCPU/GOMAXPROCS, a compact memory slice, GC activity — "is it leaking / wedged / thrashing GC?" |
| `host.errors`            | `[<count>]`                          | **THE "what broke overnight?" check.** Newest first: time + age, status, method, route template, error. See ADR-051 for what is/isn't recorded. |
| `host.errors-summary`    | —                                    | The same ring grouped by route + status with counts and first/last seen — usually the better first read. |
| `host.assets`            | `[<path-substring>]`                 | **THE "is my new CSS/JS actually being served?" check.** Per file: size, sha256[:16], mtime, and the served `?v=`, flagging any served token that doesn't match its on-disk bytes. `FullDump` (hashes every file listed). |
| `host.asset-contains`    | `<relpath>:<marker[,marker2]>`       | Marker check for one on-disk file — confirms the served build's **content**, not just its hash. Traversal outside the static root is refused. |
| `host.embedded`          | `[<plugin-slug>]`                    | Every `//go:embed`-ed plugin asset this binary serves. **Not on disk** — an empty `grep` is expected, not a finding. |
| `host.embedded-contains` | `<slug>:<relpath>:<marker[,…]>`      | Marker check for assets compiled into the binary — the only way to ask "does the shipped build contain X?" when the bytes can't be grepped. |
| `host.widgets`           | `[<name-substring>]`                 | **Widgets carry no version number**, so identity is a content fingerprint + build time. Walks both storage mechanisms and says which one each result came from. |
| `host.plugins`           | —                                    | Per plugin: static mount + URL prefix, embedded asset count/size, whether it contributed migrations, applied-vs-available schema version. **Chronicle has no plugin loader** — a missing row isn't a missing feature. |

> **Providers.** `host.embedded`, `host.widgets`, `host.plugins`, `host.errors`
> and `host.errors-summary` read app-layer state via provider injection
> (`systems.Set*Provider`, wired in `RegisterRoutes`). An unwired provider
> prints **"provider not wired"**, never the same as "there are none".
> `internal/app/operator_diag_wiring_test.go` fails CI if a declared provider
> has no call site on the boot path.

### Current diagnostics

The named diagnostics in the catalog today (from `diagnosticCatalog()`),
ordered cheapest / most common first. The `host.*` family above sorts ahead
of everything here.

| `name`            | Arg           | What you get |
|-------------------|---------------|--------------|
| `system.versions` | —             | One compact line per loaded system: id, served version, source, served dir. **The first thing to check for "is the new version live?"** |
| `system.files`    | `<system-id>` | `size · sha256[:16] · mtime` of each widget/manifest file for one system. Proves which build the loader serves. Files that are gone render as `MISSING`. With no arg it lists the loaded ids. |
| `system.health`   | —             | The full served-reality dump (all systems + all file fingerprints). Larger — request only when a targeted diagnostic isn't enough. |
| `packages.installed-vs-loaded` | — | **THE check for "Admin▸Packages says X but the old file renders":** compares each installed system package's DB version to what the loader actually serves (matched by install path). Flags `NOT loaded` and version `MISMATCH`. Requires the packages provider (wired at startup). |
| `packages.on-disk-versions` | — | Lists every on-disk version folder per package, tagging `[installed-db]` and `[LOADED]` — surfaces a stale folder shadowing the newest. |
| `packages.prune-preview` | — | Read-only dry run: on-disk version folders safe to delete (everything except newest, DB-installed, and currently-loaded), with sizes and a reclaimable total. Deletes nothing. |
| `systems.load-events` | —          | The loader's in-memory event log (newest first): `discovered` / `skipped` (a duplicate ignored, with the reason) / `failed`. Answers "did the new version load, and if a copy was skipped, why?" |
| `system.file-contains` | `<system-id>:<relpath>:<marker[,marker2]>` | Reads a served file (clamped to its system dir) and reports whether each marker string is present — confirms the live build's **content**, not just its hash. (E.g. `drawsteel:widgets/character-sheet.js:playEntrance`.) |
| `campaigns.list`  | —             | All campaigns with their ids (name + slug) — the **entry point** for the `entity.*` and `campaign.*` diagnostics, which need a campaign id. Run this first if you don't know it. |
| `entity.types`    | `<campaignId>` | A campaign's entity types (id, slug, preset category, entity count) — discover the right type to pass to `entity.field-coverage` without knowing ids. |
| `entity.find`     | `<campaignId>:<nameQuery>` | Search a campaign's entities by name/slug → id, name, slug, type. Find a hero's id without the URL. |
| `entity.fields`   | `<campaignId>:<entityIdOrSlug>` | Dumps one entity's stored field key→value map (redacted, value-capped). **THE check for "is this hero's data actually populated?"** Empty `fields_data` is the "renders blank" signature. Requires the entity provider (wired at startup). |
| `entity.sync-mappings` | `<campaignId>:<entityIdOrSlug>` | Is this entity **linked to an external (Foundry) actor?** Shows its sync mappings (external system/id ↔ chronicle id, last sync). **No mappings = nothing will ever sync to it** — the root cause of a permanently-blank hero. |
| `entity.field-coverage` | `<campaignId>:<typeIdOrName>` | For one entity type, how many of its declared fields are non-empty across its entities (emptiest first, with %). Surfaces "declared but never populated" — the backfill/sync smell. |
| `campaign.surfaces` | `<campaignId>` | Which calendar route a URL actually renders, read from the LIVE Echo table, flagged CURRENT / LEGACY / REDIRECT, plus sidebar link targets. THE check for "which calendar am I looking at?" and "the deploy landed but I still see the old thing." |
| `campaign.config` | `<campaignId>` | Enabled addons and the block types placed in `dashboard_layout` / `owner_dashboard_layout` and on each entity template — establishes hand-placed vs. seeded by a default layout or migration. |
| `sync.inbound`    | `<campaignId>:<entityIdOrSlug>` | The most recent **inbound** sync payloads an external client (e.g. the Foundry module) sent for this entity. Compare against `entity.fields`: arriving-but-not-stored → a storage bug; not arriving → a Foundry mapping gap. In-memory ring (no DB; rolls over on restart). |
| `sync.recent`     | —             | The last several inbound payloads across **all** entities — a quick "is anything syncing at all?" check. |
| `probes`          | —             | The run-and-paste-back probe library (below). |

> **Three-way data trace.** `sync.inbound` (Foundry **sent**) → `entity.fields` (Chronicle **stored**) → `entity.field-coverage` (schema **declared**) pinpoints exactly where a value dies: arrived-but-unstored = a Chronicle storage bug; never-arrived = a Foundry/adapter mapping gap; stored-but-sheet-blank = a rendering problem. The `campaign.*` diagnostics answer a different question from `host.*` and `system.*`: not which code is running, but why THIS CAMPAIGN looks like this. A "Campaign provider not wired" result means nobody was asked, not that the campaign has no addons/blocks — fix the wiring in `RegisterRoutes` before drawing any conclusion.

### Current probes

`probes` returns a curated library of commands for state the **server cannot
self-report** — what the browser loads, what's on disk, what the logs say,
which image the container runs. Chronicle **never executes these**; they are
commands *you* run and paste back (the response includes a `PASTE OUTPUT
BELOW:` slot per probe). Each declares *where* it runs: `docker` (host
shell), `browser-console` (DevTools), `sql` (DB container), or `url` (admin
URL).

Placeholders you substitute locally: `<chronicle>` / `<db>` container names,
`<media>` the in-container media path (see the served dir from
`system.versions`), `<campaignId>` the campaign UUID.

The probes today (from `defaultProbes()`). Two are labelled **TRAP**: an
operator reaches for these commands regardless of whether the library lists
them, so each prints the command *together with the way it misleads* and
names the `host.*` diagnostic that answers better.

| ID | Where | What it tells you |
|----|-------|-------------------|
| `image-digest`          | docker | **TRAP.** Which image the container RUNS vs which image the tag points at NOW. If those differ, the labels describe a different artifact than the one running — this is the command that most often produces that wrong conclusion. Prefer `host.build`. |
| `plugin-asset-grep`     | docker | **TRAP.** Grepping the container filesystem for a plugin's asset. It returns empty for every `//go:embed`-ed asset, which is most plugin front-end code. An empty result is not evidence. Prefer `host.embedded` / `host.embedded-contains`. |
| `container-restart-time`| docker | Was the container ever recreated for this deploy? If `StartedAt` predates it, nothing was replaced — a new image changes nothing until something recreates the container. |
| `binary-in-container`   | docker | Executable mtime + container clock, from outside the process — an independent cross-check of `host.build`, and the cheapest way to rule out clock skew (every age and uptime is computed against that clock). |
| `served-widget-version` | browser-console | The `?v=` on each served widget URL = the version the loader serves. If it lags Admin▸Packages, the in-memory registry never picked up the install. |
| `page-asset-tokens`     | browser-console | Every versioned asset URL on the current page — what the browser is *actually* holding, which no server-side diagnostic can see. |
| `served-widget-content` | browser-console | Fetches a served widget and checks for an expected marker — confirms whether the bytes the browser receives are the new build or a stale/cached copy. |
| `package-version-dirs`  | docker | Lists every installed version folder on disk. Multiple folders → a stale one may shadow the newest. |
| `package-file-marker`   | docker | `grep -rl` for a new-build marker across the install dirs — pinpoints which on-disk version folder actually contains the new code. |
| `chronicle-logs`        | docker | Recent Chronicle logs: package install, "replacing system with preferred copy", "ignoring duplicate system", and boot rescan lines — what the loader did with the new version. |
| `plugin-schema-versions`| sql | Applied plugin migrations, from the database itself — cross-check against `host.plugins`, which reads the migration runner's in-process health record. |
| `packages-db-state`     | sql | The `packages` table's view of installed/pinned system versions + install paths — cross-check against `packages.installed-vs-loaded`. |
| `entity-type-tree`      | sql | Entity types + per-type entity counts for a campaign — surfaces duplicate preset categories and guides a merge/reconcile. |
| `sync-mapping-orphans`  | sql | Sync mappings pointing at deleted entities — broken links that fail on the next sync. |

---

## Security model

Four properties hold by construction, making these safe to expose to an
operator and, indirectly, to an AI assistant via copy-paste:

1. **Read-only by construction.** Diagnostics only `os.Stat` and hash files
   the loader **already serves** — they never write, mutate, or execute
   anything on the host, and touch no campaign data. `health.go` is pure I/O
   over the loaded systems' own directories.

2. **Secret redaction (defense-in-depth).** Every diagnostic's output passes
   through `redactSecrets`, whose regex (`secretLine`) scrubs `key: value` /
   `key=value` lines with a credential-bearing key — `password`, `passwd`,
   `secret`, `token`, `api[-_ ]key`, `access[-_ ]key`, `private[-_ ]key`,
   `authorization`, `bearer`, including prefixed env names like
   `DB_PASSWORD` — replacing the value with `[REDACTED]` through end-of-line
   (leaving prose like "secretive" and bare `sha256:` lines alone). The
   diagnostics are secret-free anyway; this is a backstop against a future one
   that accidentally echoes a config value.

3. **Admin-gated.** Both routes sit on the admin route group (`adminGroup`,
   prefix `/admin`) in `internal/app/routes.go`, inheriting the admin
   auth/authz middleware. Non-admins can't reach them.

4. **Probes are suggested, never executed.** The probe library is a set of
   *commands for the operator to run*. Chronicle emits them as text (with a
   `PASTE OUTPUT BELOW:` slot) and never runs them itself. There is no code path
   from a diagnostic request to a `docker` / shell / SQL execution — the operator
   stays in the loop for anything that reaches outside the loader's served files.

---

## How to add a new diagnostic or probe

The catalog is **modular and templated**: the renderer, route, and redaction
never change. Adding a check is appending one struct.

### Add a diagnostic

Append a `Diagnostic` to the slice returned by `diagnosticCatalog()` in
`internal/systems/operator_diag.go`:

```go
{
    Name:    "system.something",          // dotted id the assistant requests
    Title:   "Human title",
    Desc:    "One line: what you get / when to use it.",
    ArgHint: "<some-id>",                 // "" if it takes no argument
    Run: func(arg string) string {        // returns markdown (pre-redaction)
        var b strings.Builder
        b.WriteString("## system.something\n\n")
        // ...read-only logic, e.g. iterate LoadedHealth()...
        return b.String()
    },
},
```

That's the whole change. `renderCatalog` automatically lists it in the menu,
`RunDiagnostic` dispatches `?name=system.something` to it, and the result is
passed through `redactSecrets` for free. Keep `Run` **read-only** — stat/hash/read
the loader's own state only.

### Add a probe

Append a `Probe` to the slice returned by `defaultProbes()`:

```go
{
    ID:      "my-probe",
    Title:   "What this probe reveals",
    Where:   ProbeDocker,                 // ProbeDocker | ProbeConsole | ProbeSQL | ProbeURL
    Command: `docker exec <chronicle> ...`,// may carry <placeholder> tokens
    Why:     "Why an operator would run this and what the output proves.",
},
```

It shows up automatically under `?name=probes`, rendered with its `Why`, a
fenced command block, and a paste slot. Use the `<placeholder>` convention for
anything the operator fills in locally.

---

## Worked example: diagnosing a stale package install

Symptom: you installed Draw Steel **v0.13.0** but the character sheet is
missing a feature that shipped in it. Walk the diagnostics cheapest to most
specific, pasting back only the step that surprises you:

1. **`system.versions`** — is the new version even live? `loaded_version`
   below installed → **the registry never picked up the install**; fix is
   rescan/restart, not the files. If it reads the new version, continue.
   ```
   GET /admin/diagnostics?name=system.versions
   ```
2. **`system.files drawsteel`** — if the version is right, is the *content*
   right? No `MISSING` file and hashes match → extraction is fine, move on.
   A hash matching the *old* content means bad extraction — check the
   `package-*` probes for the bad folder.
   ```
   GET /admin/diagnostics?name=system.files&arg=drawsteel
   ```
3. **`probes`, browser side** — the server says 0.13.0; does the browser
   *load* 0.13.0? Run `served-widget-version` in DevTools: if the `?v=`
   still carries the old version, it's a stale cached URL (hard refresh).
   Then `served-widget-content` confirms the fetched bytes carry the new
   build's marker.
4. **`probes`, host side** — versions agree everywhere but the code is still
   wrong: a **duplicate version folder is shadowing the new one**. Run
   `package-version-dirs`, then `package-file-marker` (`grep -rl <marker>`)
   to find which folder actually has the new code; compare it to the served
   `dir` from step 1. `chronicle-logs` shows the loader's own account
   ("ignoring duplicate system", rescan lines); `image-digest` rules out a
   stale backend image if merged backend changes also aren't live.

---

## The in-app AI Workspace (batch)

The two endpoints above are the raw machine surface. The **AI Workspace** at
`GET /admin/diagnostics/workspace` is the human-friendly front end, modeled
on the campaign **AI Workspace** import flow (export → paste → review →
commit). It collapses "request a diagnostic, run it, paste it back" into a
single batch round-trip with a **human-approval gate**, so the AI can ask for
a dozen checks at once without you running each by hand or ever touching the
server directly.

Four steps, all on one page:

1. **Copy the functions list.** The page renders a compact, machine-readable
   JSON *functions spec* (every read-only diagnostic + the exact request shape).
   You copy it and hand it to your external AI.
2. **Paste the AI's request.** The AI replies with **one** batch object naming
   the functions it wants. You paste it into the box (a fenced ` ```json ` block
   is accepted).
3. **Review & approve.** *Parse & review* validates the object against the
   catalog and shows exactly what will run — one row per call, with unknown
   names and heavy *full-dump* requests flagged. **Nothing runs until you click
   *Approve & run*.** This is the prompt-injection containment boundary: a human
   reads the toolset before it executes.
4. **Copy the result.** On approval the runnable, read-only diagnostics execute
   server-side and you get **one** compact, secret-redacted document (a manifest
   of what ran/was skipped, each result, and a byte-count footer) to copy back
   to the AI.

### Request format (what the AI composes)

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

| Field       | Meaning |
|-------------|---------|
| `v`         | Spec version (currently `1`; omittable). A mismatch is rejected. |
| `note`      | Optional free text — what the AI is investigating. Echoed into the result for audit. |
| `full_dump` | **The security gate for heavy diagnostics.** A function marked `full_dump` in the spec (e.g. `system.health`) won't run unless this is `true` (default `false`), so a stray full dump can't flood your context. |
| `calls[]`   | The diagnostics to run, each `{ "name", "arg"? }`. Cap: 50 per batch. |

### Validation & safety

- **Bounded toolset.** Only names in the live catalog run; unknown names
  surface as skipped rows, not an error. Unknown top-level keys are rejected
  so a typo can't silently drop a field. Caps: 50 calls per batch, 64 KB per paste.
- **Re-validated on run.** Approve re-parses the original pasted text
  server-side and re-derives runnability and deduplication from the live
  catalog — never trusts a client-built plan, so a forged one can't smuggle a
  gated call through.
- **Deduplicated.** Identical `(name, arg)` calls run once (second+ show as
  `duplicate` in the manifest), so a batch can't amplify into repeated
  expensive file-hash sweeps.
- **Output-capped.** The assembled result is capped (~256 KB) with a
  truncation notice, so even an authorized full dump stays compact; the
  footer reports byte size and a rough token estimate.
- **Read-only + redacted + admin-gated**, exactly as the underlying catalog —
  every result passes through `redactSecrets`, and `note`/`name`/`arg` echoed
  into the result are sanitized so a crafted value can't corrupt the manifest.
- **Full dump is opt-in twice:** the AI must set `full_dump: true` *and* you
  must approve the plan that contains it.
- **Audited.** Every run is logged to the admin activity feed
  (`admin.diagnostics_batch_run`) with actor, IP, and counts, never the payload.

> Not yet implemented: per-route rate-limiting — lower priority given the
> admin gate plus the bounded/deduped/capped toolset above.

Routes (admin-gated, in `internal/plugins/admin/routes.go`): `GET
/admin/diagnostics/workspace` (page), `POST /admin/diagnostics/workspace/parse`
(review fragment), `POST /admin/diagnostics/workspace/run` (result fragment).
The batch logic lives in `internal/systems/operator_batch.go`
(`FunctionsSpecJSON`, `ParseBatch`, `RunBatch`).
