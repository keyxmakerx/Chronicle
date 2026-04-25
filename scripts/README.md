# Chronicle Operator Scripts

Operator-runnable shell scripts for backup, restore, and other lifecycle
tasks. Distinct from anything in `cmd/` — these are sysadmin utilities,
not parts of the running server.

## Style policy

- POSIX `sh` (`#!/bin/sh`). The chronicle Docker image is Alpine, where
  `/bin/sh` is `ash`; bashisms break there. Where bash-only behavior is
  needed (rare), the script must declare `#!/bin/bash` and explain why
  in a header comment.
- `set -eu` at the top. No `pipefail` (not POSIX).
- Quote every variable expansion.
- Run `shellcheck` on any change.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Operator error (bad args, missing required env) |
| 2 | Precondition failure (target not empty, server still running, etc.) |
| 3 | Backend tool failure (mysqldump, tar, gzip, etc.) |

## Invocation patterns

**In-container (primary, for compose deployments):**
```sh
docker compose exec -T chronicle /app/scripts/backup.sh
```
The `-T` is required when invoking from cron (no TTY). The container
already has `mysqldump`, gzip, the volume mount, and DB credentials in
its env, so no extra plumbing is needed.

**Standalone host (bare-metal / non-compose):**
```sh
DB_HOST=...:3306 DB_USER=... DB_PASSWORD=... DB_NAME=chronicle \
  MEDIA_PATH=/var/lib/chronicle/media BACKUP_DIR=/var/backups/chronicle \
  ./scripts/backup.sh
```
Same script; reads its env contract directly. Requires `mysqldump`,
`tar`, `gzip` on PATH on the host.

## Scripts

| Script | What it does |
|---|---|
| `backup.sh` | Snapshot the DB + media to timestamped artifacts under `$BACKUP_DIR`, with a SHA-256 manifest. Rotates older than `$BACKUP_RETENTION_DAYS`. |
| `restore.sh` | Restore a paired backup set from a manifest. Multiple safety gates; requires explicit confirmation. |

Both scripts support `--check` (validate environment only, exit 0/non-0).
Use it in CI or on a fresh stack to verify the runbook before you need it.

See `docs/deployment.md` for the full operator runbook, including cron
examples, offsite copy guidance, and the rollback procedure.
