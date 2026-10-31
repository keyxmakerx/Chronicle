# Restore Drill

Proves the newest backup restores, without touching the live database.

```sh
./tools/restore-drill.sh
```

Run it from the deployment directory (where `docker-compose.yml` lives) with
the stack up. It:

1. Finds the newest backup in the `chronicle` container's `/app/data/backups`
   (the place `make backup-list` lists).
2. Starts a disposable MariaDB container, matching the image in
   `docker-compose.yml`. It never uses `chronicle-db`, a host port, or the
   live network or volume.
3. Loads the dump into it.
4. Checks: `schema_migrations` is present, not dirty, and not newer than the
   checkout's newest migration; `users`, `campaigns` and `entities` have rows;
   `calendar_events` has rows if the table exists; no `entities` row points at a
   missing campaign.
5. Prints `RESTORE DRILL: PASS` or `RESTORE DRILL: FAIL: <why>`.
6. Removes the container on pass, fail or Ctrl-C.

Run it monthly and before every upgrade.

PASS means the dump loads, the schema version is sane, core tables are populated
and the spot foreign key holds. It does not check the media tarball, Redis, or
whether a particular campaign's data is right.

## On FAIL

| Reason | Meaning | Action |
|---|---|---|
| `no backups found in /app/data/backups` | `make backup` never ran, or `chronicle` is down | `make backup`; check `docker compose ps` |
| `schema_migrations.dirty=1` | Backup taken mid-migration | Use a newer backup, or re-run `make backup` once the live DB is healthy |
| `'<table>' has 0 rows` | Dump empty or truncated | Check `make backup-check` and the cron log |
| `entities row(s) reference a non-existent campaign_id` | Referential integrity broken | Distrust this backup; find an earlier one |
| `sha256 mismatch` | Incomplete copy | Re-copy, or `make backup` |
| `docker-compose.yml not found` / `docker daemon not reachable` | Wrong directory, or Docker not running | `cd` to the deployment directory; check `docker info` |

Investigate before cron rotates the evidence away: backups are kept for
`BACKUP_RETENTION_DAYS` (default 7, `docs/deployment.md` §8).

## Not covered

- **Media tarball:** `tar -tzf chronicle_media_<TS>.tar.gz | head` lists it.
- **Redis:** holds sessions and rate-limit counters only; losing it logs
  everyone out (`docs/deployment.md` §3).
- **Application correctness:** see "Verifying a restore" in `docs/deployment.md` §9.

## A specific backup

```sh
./tools/restore-drill.sh --file /app/data/backups/chronicle_manifest_<TS>.txt
./tools/restore-drill.sh --file /path/to/a/copied/chronicle_db_<TS>.sql.gz
```

A manifest path is sha256-verified against its paired dump (both files must sit
together). A raw `.sql` or `.sql.gz` is loaded without verification.

## Restoring for real

This stops the live app and replaces the live database.

```sh
docker compose stop chronicle
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"
# Type RESTORE when prompted.
docker compose start chronicle
```

`make restore` runs `scripts/restore.sh`; flags and post-restore checks are in
`docs/deployment.md` §9. Drill the same manifest first.
