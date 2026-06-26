# Operator Diagnostics

This document describes Chronicle's **operator diagnostics** — a pair of
admin-gated, read-only endpoints that report *the deployment reality* of a
running Chronicle instance: which version of each system the loader is
**actually serving**, the on-disk directory it serves from, and a content
fingerprint of every served file.

If you operate a Chronicle instance and have ever asked *"Admin▸Packages says
this system is v0.13.0, so why is the old widget still rendering?"* — this is
the tool for that. It answers the underlying question without SSH or shell
access to the host.

It is also the operator-facing analogue of the campaign **AI-Export**: a catalog
of named, targeted diagnostics you run one at a time and **paste back to an AI
assistant**, so the assistant gets exactly the small slice of state it asked for
rather than a giant dump. (The vision is captured in the debug-cockpit & AI-assist
capability spec, §B.)

Source: `internal/systems/health.go`, `internal/systems/operator_diag.go`,
routes in `internal/app/routes.go` (registered on the admin route group).

---

## The problem it solves

When a system package is installed or updated, several things have to line up:

1. The new version has to be **extracted** to disk under the package media dir.
2. The **in-memory system registry** has to pick up that new version on rescan.
3. The browser has to **fetch the new bytes** (not a stale `?v=` cache).

When one of those steps silently fails, Admin▸Packages can report the new
version while a stale copy is what actually renders. The UI can't tell you which
step broke, because the UI only knows what it *installed* — not what the loader
*serves*. Operator diagnostics close that gap: they report the served reality
(loaded version + served dir + per-file `size · sha256 · mtime`), so you can
prove which build is live.

- **`loaded_version` disagrees with the installed version** → the in-memory
  registry never picked up the install (a stale registry; needs a rescan/restart).
- **`loaded_version` agrees but a file's hash is the old content** → the
  extraction is wrong (a botched copy, or a duplicate version folder shadowing
  the new one).
- **A file is `MISSING`** → a botched extraction.

---

## The two endpoints

Both are registered on the **admin route group** (prefix `/admin`) and are
therefore admin-gated. Both are **read-only**.

### 1. `GET /admin/extensions/health` — machine-readable health (JSON)

Returns deployment health for every **loaded** system as JSON. Use this for
tooling, monitoring, or scripted comparison between two installs.

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
        },
        {
          "path": "widgets/character-sheet.js",
          "exists": true,
          "size": 38211,
          "sha256": "a1b2c3d4e5f60718",
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
| `source`         | `bundled` (shipped in the binary's `internal/systems`) or `package` (installed via the package manager). |
| `dir`            | The on-disk directory the loader serves this system's files from. |
| `files[]`        | One entry per served file (manifest + every declared widget script + text-renderer file). |
| `files[].exists` | `false` means the served dir is missing that file — itself diagnostic. |
| `files[].size`   | File size in bytes. |
| `files[].sha256` | First 16 hex chars of the content SHA-256 — enough to compare two installs without shipping the whole file. |
| `files[].mtime`  | File mtime, RFC3339 UTC. |

### 2. `GET /admin/diagnostics` — the diagnostics catalog (markdown)

The human/AI-facing front door. Returns **markdown** (`text/markdown`). With no
`?name`, it returns a tiny **catalog menu** (no payload data). With `?name=...`
it runs exactly one named diagnostic and returns its small, redacted result.

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

Example of running one diagnostic (`?name=system.versions`):

```markdown
## system.versions

- `drawsteel` v**0.13.0** (package) — `/app/media/packages/systems/drawsteel/0.13.0`
- `dnd5e` v**1.2.0** (package) — `/app/media/packages/systems/dnd5e/1.2.0`
```

---

## The catalog model

Rather than a single monolithic export, `/admin/diagnostics` is a **catalog** of
small named checks. The motivation: a giant dump wastes an AI assistant's
context with data it never asked for. Instead the assistant requests **one named
diagnostic at a time** (e.g. *"run `system.files drawsteel`"*), the operator runs
just that, and pastes back a small, targeted result. The full dump
(`system.health`) still exists but is **opt-in** — requested by name only when a
targeted diagnostic won't do.

### Current diagnostics

These are the named diagnostics in the catalog today (from
`diagnosticCatalog()`), ordered cheapest / most common first:

| `name`            | Arg           | What you get |
|-------------------|---------------|--------------|
| `system.versions` | —             | One compact line per loaded system: id, served version, source, served dir. **The first thing to check for "is the new version live?"** |
| `system.files`    | `<system-id>` | `size · sha256[:16] · mtime` of each widget/manifest file for one system. Proves which build the loader serves. Files that are gone render as `MISSING`. With no arg it lists the loaded ids. |
| `system.health`   | —             | The full served-reality dump (all systems + all file fingerprints). Larger — request only when a targeted diagnostic isn't enough. |
| `packages.installed-vs-loaded` | — | **THE check for "Admin▸Packages says X but the old file renders":** compares each installed system package's DB version to what the loader actually serves (matched by install path). Flags `NOT loaded` (the registry never picked up the install) and version `MISMATCH`. Requires the packages provider (wired at startup). |
| `packages.on-disk-versions` | — | Lists every on-disk version folder per package, tagging `[installed-db]` (the DB's version) and `[LOADED]` (what the loader serves) — surfaces a stale folder shadowing the newest. |
| `systems.load-events` | —          | The loader's in-memory event log (newest first): `discovered` / `skipped` (a duplicate ignored, with the reason) / `failed`. Answers "did the new version load, and if a copy was skipped, why?" |
| `probes`          | —             | The run-and-paste-back probe library (below). |

### Current probes

`probes` returns a curated library of commands for state the **server cannot
self-report** — what the browser actually loads, what's on disk, what the logs
say, which image the container runs. Chronicle **never executes these** — they
are commands *you* run and paste the output back (the response even includes a
`PASTE OUTPUT BELOW:` slot per probe). Each probe declares *where* it runs:
`docker` (host shell), `browser-console` (DevTools), `sql` (DB container), or
`url` (admin URL).

Placeholders you substitute locally: `<chronicle>` / `<db>` container names,
`<media>` the in-container media path (see the served dir from `system.versions`),
`<campaignId>` the campaign UUID.

The probes today (from `defaultProbes()`):

| ID | Where | What it tells you |
|----|-------|-------------------|
| `served-widget-version` | browser-console | The `?v=` on each served widget URL = the version the loader serves. If it lags Admin▸Packages, the in-memory registry never picked up the install. |
| `served-widget-content` | browser-console | Fetches a served widget and checks for an expected marker — confirms whether the bytes the browser receives are the new build or a stale/cached copy. |
| `package-version-dirs`  | docker | Lists every installed version folder on disk. Multiple folders → a stale one may shadow the newest. |
| `package-file-marker`   | docker | `grep -rl` for a new-build marker across the install dirs — pinpoints which on-disk version folder actually contains the new code. |
| `chronicle-logs`        | docker | Recent Chronicle logs: package install, "replacing system with preferred copy", "ignoring duplicate system", and boot rescan lines — what the loader did with the new version. |
| `image-digest`          | docker | Which Chronicle image the container runs — a stale image explains merged backend changes not being live. |
| `packages-db-state`     | sql | The `packages` table's view of installed/pinned system versions + install paths — cross-check against `packages.installed-vs-loaded`. |
| `entity-type-tree`      | sql | Entity types + per-type entity counts for a campaign — surfaces duplicate preset categories and guides a merge/reconcile. |
| `sync-mapping-orphans`  | sql | Sync mappings pointing at deleted entities — broken links that fail on the next sync. |

---

## Security model

Operator diagnostics are designed to be safe to expose to an operator (and,
indirectly, to an AI assistant via copy-paste). Four properties hold by
construction:

1. **Read-only by construction.** The diagnostics only `os.Stat` and hash files
   the loader **already serves** — they never write, mutate, or execute anything
   on the host, and they touch no campaign data. `health.go` is pure I/O over the
   loaded systems' own directories.

2. **Secret redaction (defense-in-depth).** Every diagnostic's output passes
   through `redactSecrets` before it leaves the server. A regex
   (`secretLine`) scrubs `key: value` / `key=value` lines whose key looks
   credential-bearing — `password`, `passwd`, `secret`, `token`, `api[-_ ]key`,
   `access[-_ ]key`, `private[-_ ]key`, `authorization`, `bearer`, including
   prefixed env names like `DB_PASSWORD` / `MY_API_KEY` — replacing the value
   with `[REDACTED]` through end-of-line. The systems diagnostics are secret-free
   *anyway*; redaction is a backstop so a *future* diagnostic that accidentally
   echoes a config value can't leak a credential. (It is careful to leave prose
   like "secretive" and bare `sha256:` hash lines alone.)

3. **Admin-gated.** Both routes are registered on the admin route group
   (`adminGroup`, prefix `/admin`) in `internal/app/routes.go`, so they inherit
   the admin authentication/authorization middleware. Non-admins can't reach
   them.

4. **Probes are suggested, never executed.** The probe library is a set of
   *commands for the operator to run*. Chronicle emits them as text (with a
   `PASTE OUTPUT BELOW:` slot) and never runs them itself. There is no code path
   from a diagnostic request to a `docker` / shell / SQL execution — the operator
   stays in the loop for anything that reaches outside the loader's served files.

---

## How to add a new diagnostic or probe

The catalog is deliberately **modular and templated**: the renderer, route, and
redaction never change. Adding a check is **appending one struct**.

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

Symptom: you installed Draw Steel **v0.13.0** (Admin▸Packages confirms it), but
the character sheet in the browser is missing a feature you know shipped in that
version. Walk the diagnostics from cheapest to most specific.

**Step 1 — `system.versions`.** Is the new version even live?

```
GET /admin/diagnostics?name=system.versions
```

```markdown
- `drawsteel` v**0.12.1** (package) — `/app/media/packages/systems/drawsteel/0.12.1`
```

`loaded_version` is **0.12.1**, not 0.13.0. The loader is serving an *older*
version than Admin▸Packages installed → the **in-memory registry never picked up
the install**. The fix is on the registry side (rescan/restart), not the files.
If instead it had read `v0.13.0`, move on to Step 2.

**Step 2 — `system.files drawsteel`.** If the version is right, is the *content*
right?

```
GET /admin/diagnostics?name=system.files&arg=drawsteel
```

```markdown
loaded v**0.13.0** from `/app/media/packages/systems/drawsteel/0.13.0`

- `manifest.json` — 4821 · `9f1c2a7b3d4e5f60` · 2026-06-24T18:02:11Z
- `widgets/character-sheet.js` — 38211 · `a1b2c3d4e5f60718` · 2026-06-24T18:02:11Z
```

Version matches and no file is `MISSING`. If a hash here is the *old* content,
the extraction is wrong — jump to the `package-*` probes to find the bad folder.

**Step 3 — `probes`, browser side.** The server says it's serving 0.13.0; does
the browser actually *load* 0.13.0?

```
GET /admin/diagnostics?name=probes
```

Run `served-widget-version` in the page's DevTools console. If the
`character-sheet.js?v=...` URL still carries `0.12.1`, the browser holds a stale
cached URL — a hard refresh / cache bust fixes it. Then run
`served-widget-content` to confirm the fetched bytes contain the new build's
marker.

**Step 4 — `probes`, host side.** If versions agree everywhere but the code is
still wrong, you likely have a **duplicate version folder shadowing the new one**.
Run `package-version-dirs` (lists every installed folder), then
`package-file-marker` (`grep -rl <marker>` — shows *which* folder actually has the
new code). Compare that folder to the served `dir` from Step 1: if the new code
lives in a folder the loader **isn't** serving, a stale duplicate is winning.
Finally `chronicle-logs` shows the loader's own account ("ignoring duplicate
system", "replacing system with preferred copy", rescan lines), and `image-digest`
rules out a stale backend image if merged *backend* changes also aren't live.

Reading the result, in short:

- **version wrong** (Step 1) → stale registry; rescan/restart.
- **version right, hash wrong / `MISSING`** (Step 2) → bad extraction; check the
  on-disk folders with the `package-*` probes.
- **server right, browser `?v=` stale** (Step 3) → cache; hard refresh.
- **everything agrees but old code on disk in another folder** (Step 4) → a
  duplicate version folder is shadowing the new one.

Paste only the step that surprised you back to the assistant — that's the whole
point of the catalog.
