# Restore Drill

A backup you've never restored is a hope, not a backup. This is the one
command that turns "we have backups" into "we've proven the backups work" --
without touching your live database.

Cites: `cordinator/plans/2026-07-10-beta-transition-plan.md` §2 item 0.6
(the beta plan's single largest data-loss risk); `cordinator/decisions/2026-05-21-core-tenets.md`
§T-O1 (verify before claim) and §T-B1 (security -- the drill never widens
the live DB's attack surface).

## The one command

```sh
./tools/restore-drill.sh
```

Run it from your Chronicle deployment directory (where `docker-compose.yml`
lives), with the stack up. No flags needed. It:

1. Finds your newest backup (from the `chronicle` container's
   `/app/data/backups`, same place `make backup-list` looks).
2. Spins up a brand-new, disposable MariaDB container -- **never** your
   live `chronicle-db`, never a host port, never anything on the same
   network or volume as your real deployment.
3. Loads that backup's database dump into the throwaway container.
4. Checks: the migrations table is present and at a plausible version,
   the core tables (`users`, `campaigns`, `entities`, `calendar_events`)
   have rows, and one foreign-key relationship
   (`entities.campaign_id -> campaigns.id`) is intact.
5. Prints one line: green `RESTORE DRILL: PASS` or red `RESTORE DRILL:
   FAIL: <why>`.
6. Always tears the throwaway container down -- pass, fail, or Ctrl-C.

Takes under a minute. Your live database is never opened for writes, never
stopped, never even connected to for anything but reading `docker-compose.yml`
to match its MariaDB image.

## When to run it

- **Monthly**, as a standing habit -- put it on the same calendar reminder as
  checking `make backup-list`.
- **Before every upgrade** -- redeploys and migrations are exactly when a bad
  backup would hurt most, and exactly when you want five seconds of
  confidence beforehand.
- **Right now, once**, if you've never run it. That's the whole point of
  this tool existing.

## What PASS means

The newest backup on disk is a real, loadable, structurally sane Chronicle
database: schema at the version your code expects, core tables populated,
referential integrity intact. It does **not** prove your media/uploads
tarball or Redis dump are fine (those aren't touched by this drill --
see "What this doesn't check" below), and it doesn't replace actually
opening a restored instance in a browser occasionally.

## What to do on FAIL

The message after `RESTORE DRILL: FAIL:` says exactly why. Common cases:

| Reason | Meaning | What to do |
|---|---|---|
| `no backups found in /app/data/backups` | `make backup` has never run, or the chronicle service is down | Run `make backup` now; check `docker compose ps` |
| `schema_migrations.dirty=1` | The backup was taken mid-migration | Don't trust this backup; check for a newer one, or re-run `make backup` once the live DB is healthy |
| `'<table>' has 0 rows` | The dump is empty or truncated | Something is wrong with `scripts/backup.sh`'s run -- check `make backup-check` and the cron log |
| `entities row(s) reference a non-existent campaign_id` | Referential integrity broken in the dump | Treat this backup as untrustworthy; look for an earlier good one and investigate what corrupted this one |
| `sha256 mismatch` | The backup file was copied or transferred incompletely | Re-copy it, or regenerate with `make backup` |
| `docker-compose.yml not found` / `docker daemon not reachable` | Wrong directory, or Docker isn't running | `cd` to your Chronicle deployment directory; confirm `docker info` works |

A FAIL means: **don't assume this backup would save you.** Investigate before
your next `make backup` cron run overwrites the evidence (backups rotate
after `BACKUP_RETENTION_DAYS`, default 7 -- see `docs/deployment.md` §8).

## What this doesn't check

- **Media/uploads tarball** -- the drill only restores the DB dump. If you
  want to sanity-check the media tarball too, `tar -tzf
  chronicle_media_<TS>.tar.gz | head` lists its contents without extracting.
- **Redis** -- sessions and rate-limit counters only; losing them on restore
  just logs everyone out. Nothing worth drilling (see `scripts/backup.sh`'s
  header comment and `docs/deployment.md` §3).
- **Application-level correctness** -- the drill checks structure and row
  counts, not "does this specific campaign's data look right." For that,
  `docs/deployment.md` §9 "Verifying a 0.0.2+ restore" has a manual
  spot-check.
- **A REAL restore** -- this drill never touches your live deployment. See
  the next section for that.

## Testing a specific backup

```sh
./tools/restore-drill.sh --file /app/data/backups/chronicle_manifest_<TS>.txt
./tools/restore-drill.sh --file /path/to/a/copied/chronicle_db_<TS>.sql.gz
```

Useful for checking an offsite copy, or a backup from before the newest one.
A manifest path gets sha256-verified against its paired dump; a raw
`.sql`/`.sql.gz` file is loaded as-is (no manifest to verify against, and
the script says so).

---

## ⚠️ Restoring FOR REAL (dangerous -- this touches your live deployment)

Everything above is a drill: throwaway container, zero risk to production.
**This section is not a drill.** It stops your live app and replaces your
live database. Only do this when you actually need to recover from data
loss or corruption.

```sh
docker compose stop chronicle
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"
# Type RESTORE when prompted.
docker compose start chronicle
```

That's `scripts/restore.sh` via the existing `make restore` target -- see
`docs/deployment.md` §9 "Restore procedure" for the full walkthrough,
including `--db-only`/`--force`/`--yes` flags and post-restore verification.
Running a `restore-drill.sh` PASS against the same manifest first is exactly
the confidence check you want before typing `RESTORE` for real.
