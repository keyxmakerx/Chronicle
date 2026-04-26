# Chronicle Deployment Runbook

The 2 AM operator's reference for installing, upgrading, backing up,
restoring, and troubleshooting a Chronicle instance.

If you're tagging or upgrading to 0.0.1, read §2, §6, and §10 first.

## Contents

1. [TL;DR](#1-tldr)
2. [System requirements](#2-system-requirements)
3. [Persistence inventory](#3-persistence-inventory)
4. [Install](#4-install)
5. [Configuration](#5-configuration)
6. [Upgrade / redeploy](#6-upgrade--redeploy)
7. [Rollback](#7-rollback)
8. [Backup procedure](#8-backup-procedure)
9. [Restore procedure](#9-restore-procedure)
10. [Troubleshooting common boot failures](#10-troubleshooting-common-boot-failures)
11. [Security checklist](#11-security-checklist)
12. [Out of scope for 0.0.1](#12-out-of-scope-for-001)

---

## 1. TL;DR

```sh
git clone https://github.com/keyxmakerx/chronicle.git && cd chronicle
cp .env.example .env
# Set SECRET_KEY, DB_PASSWORD, MYSQL_ROOT_PASSWORD, MYSQL_PASSWORD in .env.
# Generate SECRET_KEY with: openssl rand -base64 32
docker compose up -d
docker compose logs -f chronicle  # wait for "health check summary passed=N"
make backup-check                 # verify the backup pipeline before you need it
```

Open `http://localhost:8080`, register the first user (becomes site admin),
read §11 before exposing this anywhere.

## 2. System requirements

- **Host:** Linux with Docker 24+ and Compose v2. Tested on Debian / Ubuntu /
  Alpine. macOS dev fine via Docker Desktop; production should be Linux.
- **CPU/RAM:** 1 vCPU + 1 GB RAM minimum, 2 GB recommended for a campaign
  with media uploads.
- **Disk:** 5 GB minimum. Volumes grow with media; plan capacity around
  uploads and installed system packages.
- **Bundled services:** MariaDB 10.11+ via the chronicle-db service, Redis 7+
  via chronicle-redis. If you BYO either, see §5.
- **Outbound network:** required at runtime only when an admin installs a
  package (GitHub fetch). Otherwise self-contained.

## 3. Persistence inventory

Drives the rest of this doc. **Bold = must back up.**

| State | Location (compose) | Must back up? | If lost |
|---|---|---|---|
| **MariaDB tablespace** | volume `chronicle-dbdata` (`/var/lib/mysql`) | **Yes — primary** | Total data loss; site unrecoverable. |
| **User uploads** | volume `chronicle-data` at `/app/data/media/uploads` | **Yes** | Broken image references in entities; users must re-upload. |
| **User avatars** | volume `chronicle-data` at `/app/data/media/avatars` | **Yes** | Profile pictures revert to default. |
| Installed system packages | `/app/data/media/packages/systems/` | No (re-fetchable) | Admin re-installs via Admin → Packages. |
| Foundry module package | `/app/data/media/packages/foundry-module/` | No (re-fetchable) | Same as above. |
| Pre-migration auto-backups | `/app/data/backups/chronicle_pre_migrate_*.sql.gz` | Operator's call | Loses safety net for the next migration window. |
| Operator backups | `/app/data/backups/chronicle_db_*` etc. | n/a (the backups themselves) | n/a |
| Redis AOF (sessions) | volume `chronicle-redisdata` | Optional (sessions only) | All users logged out; no data lost. |
| Migration version pointer | `schema_migrations` row in MariaDB | Covered by DB dump | Auto-replayed by `golang-migrate` on next boot if matching DB content present. |
| **Secrets** (`SECRET_KEY`, DB passwords) | `.env` on host | **Yes — out of band** | Sessions invalidated; DB auth breaks. |

Everything operator-controlled lives in three named volumes
(`chronicle-data`, `chronicle-dbdata`, `chronicle-redisdata`) and one host
file (`.env`). Back those up, you can resurrect anything else.

The MariaDB tablespace row covers all relational state, including the
player-to-character claim relationships introduced in 0.0.2
(`entities.owner_user_id`, migration 22). `mysqldump --single-transaction`
captures the column transparently — no separate handling — and `restore.sh`
brings it back identically. If a player has claimed a character, that link
is in the DB dump; if it isn't in the dump, the link didn't exist at backup
time.

## 4. Install

### Docker Compose (primary path)

```sh
git clone https://github.com/keyxmakerx/chronicle.git
cd chronicle
cp .env.example .env
${EDITOR:-vi} .env       # see §5
docker compose up -d
docker compose logs -f chronicle
```

Wait for `health check summary passed=N warnings=0 failures=0`. Hit
`http://localhost:8080`. The first registered user becomes site admin —
register yours immediately if the host is reachable from the public
internet.

### Bare metal

Requires Go 1.24+, MariaDB 10.11+, Redis 7+, and a build of Tailwind +
templ — see `Makefile` for the targets. Production bare-metal is not
the recommended path; the compose stack pins versions and ships a
correct `mariadb-client` for backups.

### Cosmos Cloud

`docker-compose.yml` carries the `cosmos-stack` labels needed for
Cosmos auto-discovery. Import the compose file directly; Cosmos handles
TLS termination and routing.

## 5. Configuration

Every env var Chronicle reads. **Bold = required in production.**

| Var | Default | Notes |
|---|---|---|
| `ENV` | `development` | Set to `production` in prod; raises security audit warnings to errors. |
| `PORT` | `8080` | Container exposes 8080; change the port mapping in compose, not this. |
| **`BASE_URL`** | `http://localhost:8080` | Production must be `https://...`; HTTP in production is flagged by the security audit. |
| `LOG_LEVEL` | `debug` | `info` / `warn` / `error` for production. |
| `DB_HOST` | `localhost:3306` | `host:port` format; compose sets `chronicle-db:3306`. |
| `DB_USER` | `chronicle` | |
| **`DB_PASSWORD`** | `chronicle` | Must change in production. The audit explicitly rejects `chronicle`, `password`, `secret`, `changeme`, `root`, `admin`. |
| `DB_NAME` | `chronicle` | |
| `DB_TLS_MODE` | (disabled) | `required` / `skip-verify` / `preferred`. Production: `required`. |
| `DATABASE_URL` | (empty) | Optional override; full DSN. Bypasses the individual `DB_*` vars. |
| `DB_MAX_OPEN_CONNS` | `25` | |
| `DB_MAX_IDLE_CONNS` | `5` | |
| `DB_CONN_MAX_LIFETIME` | `5m` | |
| `REDIS_URL` | `redis://localhost:6379` | |
| **`SECRET_KEY`** | (none — required) | 32+ bytes base64. Generate: `openssl rand -base64 32`. PASETO signing key for sessions; rotating it logs everyone out. |
| `SESSION_TTL` | `720h` | |
| `EXTENSIONS_PATH` | `./extensions` | User-installable content extensions. |
| `MAX_UPLOAD_SIZE` | `10MB` | |
| `MEDIA_PATH` | `./data/media` | Resolves to `/app/data/media` in the container. |
| `MEDIA_SIGNING_SECRET` | (auto) | Auto-generated if empty; HMAC-SHA256 for signed media URLs. |
| `MEDIA_SERVE_RATE_LIMIT` | `300` | Requests/min/IP for `GET /media/:id`. |
| `BACKUP_DIR` | `/app/data/backups` (compose) / empty (bare) | Empty disables the in-process pre-migration backup safety net AND the admin backup/restore UIs. Compose defaults it on. |
| `BACKUP_RETENTION_DAYS` | `7` | Used by `scripts/backup.sh`. The in-process rotator uses a separate hardcoded 7d for `chronicle_pre_migrate_*` artifacts. |
| `BACKUP_REQUIRED` | `0` | When `1` or `true`, the in-process pre-migration capture is mandatory: any failure (mysqldump missing, dump zero bytes, manifest write fails) aborts startup before migrations apply. Use in production. The default fail-open behavior (warn + proceed) preserves the legacy semantics for development setups that don't have `mariadb-client` installed. |
| `BACKUP_SCRIPT_PATH` | `/app/scripts/backup.sh` | Used by the admin "Run backup" button. |
| `RESTORE_SCRIPT_PATH` | `/app/scripts/restore.sh` | Used by the admin restore page. |
| `CHRONICLE_VERSION` | `unknown` | Stamped into the pre-migration manifest's `chronicle_version=` line. Set by Docker build args / release pipeline; unset is fine for development. |
| `MYSQL_ROOT_PASSWORD` | (compose) | Compose-only; sets root password for the bundled MariaDB. |
| `MYSQL_PASSWORD` | (compose) | Compose-only; must match `DB_PASSWORD`. |

## 6. Upgrade / redeploy

```sh
make backup                                            # 1. operator snapshot
docker compose pull                                    # 2. fetch new images
docker compose up -d --no-deps chronicle               # 3. swap chronicle only
docker compose logs -f chronicle                       # 4. watch the boot
```

In step 4 you should see, in order:

```
creating pre-migration backup file=/app/data/backups/chronicle_pre_migrate_<TS>.sql.gz
pre-migration backup completed file=...
migrations applied
migration version validated version=N
health check summary passed=K warnings=0 failures=0
```

If you see `pre-migration backup skipped: mysqldump not found`, your image
predates 0.0.1; rebuild with `docker compose build --no-cache chronicle`.

If `failures=0` doesn't appear and the chronicle container exits, the
release is broken — go to §7.

The `--no-deps` flag is intentional: keep MariaDB and Redis up across the
swap. They get restarted only when their image changes, which is rare.

## 7. Rollback

Three scenarios. All use the existing health-check gate plus the
pre-migration backup. **No new server code is ever needed for a rollback.**

### Scenario A — server failed health checks at boot (most common)

The chronicle container `os.Exit(1)`'d cleanly without serving any
traffic. The DB schema may or may not have advanced.

```sh
docker compose logs chronicle | grep -E 'health check|migration|critical column'
```

Read which check failed. If it's a migration version mismatch and you
need to roll back the schema:

As of the symmetry refactor, pre-migration captures emit the same
manifest format as `scripts/backup.sh` (`chronicle_pre_migrate_manifest_<TS>.txt`
plus per-artifact db/media/redis files with sha256 verification). That
means `scripts/restore.sh --manifest <path>` can roll back from a
pre-migration snapshot directly:

```sh
# Find the most recent pre-migration manifest (one per boot that ran
# migrations on a non-empty BackupDir).
docker compose exec -T chronicle ls -lt /app/data/backups/chronicle_pre_migrate_manifest_*.txt | head -3

# Stop chronicle, leave DB + Redis up.
docker compose stop chronicle

# Restore from the pre-migration bundle. restore.sh verifies sha256
# of each artifact before touching the live DB.
docker compose run --rm chronicle sh /app/scripts/restore.sh \
  --manifest /app/data/backups/chronicle_pre_migrate_manifest_<TS>.txt \
  --yes --force

# Pin the previous image tag in docker-compose.yml or your registry, then:
docker compose up -d chronicle
```

The same approach works from the **admin restore UI** at `/admin/restore`
— pre-migration manifests appear in the list alongside operator-triggered
backups, distinguished by a `chronicle_pre_migrate=1` line in the manifest
body (the listing UI labels them as such).

For DB-only rollbacks against a legacy `chronicle_pre_migrate_<TS>.sql.gz`
file (taken before the symmetry refactor), the manual `gunzip | mysql`
approach still works:

```sh
docker compose exec -T chronicle sh -c \
  'gunzip -c /app/data/backups/chronicle_pre_migrate_<TS>.sql.gz \
     | MYSQL_PWD="$DB_PASSWORD" mysql -h "$DB_HOST" -u "$DB_USER" "$DB_NAME"'
```

#### Worked example — rolling back across the 0.0.2 → 0.0.1 boundary (migration 22)

0.0.2 adds migration 22 (`entities.owner_user_id`, the player-character
claim column). Two failure modes cross this boundary:

- **0.0.2 binary boots, migration 22 succeeds, but a regression appears
  later.** Roll the image tag back to 0.0.1 *without* restoring the DB.
  The 0.0.1 binary's `ExpectedMigrationVersion` is 21 and `RunStartupHealthChecks`
  will refuse to start against a schema at version 22 — that's the gate
  doing its job. To downgrade safely, restore the most recent
  `chronicle_pre_migrate_*.sql.gz` (taken just before migration 22 ran)
  using the steps above, then pin `chronicle:0.0.1` and restart. Any
  claim relationships created on 0.0.2 are dropped by this rollback —
  that's intrinsic to undoing migration 22, not a bug in the procedure.
- **0.0.2 binary fails health checks at boot.** `os.Exit(1)` happens
  before any traffic is served. Use the standard Scenario A flow above;
  the pre-migration backup is the version-21 schema and any
  `entities.owner_user_id` column added by the failed 0.0.2 boot will be
  rolled away with it.

There is **no forward-compat fallback**: a 0.0.1 binary will not boot
against a 0.0.2 schema. Either match the binary to the schema, or
restore the schema to match the older binary.

### Scenario B — server is up but a feature is broken

No DB restore needed unless the broken release introduced a destructive
migration (in which case use Scenario A). Roll the image tag back:

```sh
# Edit docker-compose.yml: image: ghcr.io/.../chronicle:<previous-tag>
docker compose up -d --no-deps chronicle
```

### Scenario C — data corruption / accidental destructive admin action

Full restore from the latest operator backup pair:

```sh
docker compose stop chronicle
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"
# Type RESTORE when prompted.
docker compose start chronicle
```

`RunStartupHealthChecks` validates the restored schema against the running
image's `ExpectedMigrationVersion`. If the manifest is from a version that
requires a different code revision, chronicle refuses to start — pin a
matching image tag and try again.

## 8. Backup procedure

`scripts/backup.sh` snapshots DB + media + (optionally) Redis. Driven via
`make backup`:

```sh
make backup                                  # full snapshot
make backup BACKUP_ARGS="--no-media"          # DB only
make backup BACKUP_ARGS="--no-redis --retention 30"
make backup-check                             # validate without writing
make backup-list                              # see what's in the volume
```

Output goes to `$BACKUP_DIR` (default `/app/data/backups` inside the
chronicle-data volume). Each run produces:

- `chronicle_db_<TS>.sql.gz` — `mysqldump --single-transaction --routines --triggers` piped to gzip.
- `chronicle_media_<TS>.tar.gz` — `tar -czf` over `MEDIA_PATH`.
- `chronicle_redis_<TS>.rdb` — `redis-cli --rdb`, when `redis-cli` is on PATH (best effort; sessions are survivable).
- `chronicle_manifest_<TS>.txt` — sha256 + chronicle version + migration version. Drives `restore.sh`.

`scripts/backup.sh` rotates artifacts older than `BACKUP_RETENTION_DAYS`
(default 7). The pre-migration `chronicle_pre_migrate_*` files are
managed by a separate Go-side rotator and are never touched by the
script.

### Cron example (daily at 03:00, host crontab)

```cron
0 3 * * * cd /opt/chronicle && /usr/bin/make backup >> /var/log/chronicle-backup.log 2>&1
```

The `make backup` target uses `docker compose exec -T` so it works
without a TTY.

### Offsite copy

Anything in `$BACKUP_DIR` is fair game. Pick one:

- `rsync` to a backup host:
  ```sh
  rsync -avz /var/lib/docker/volumes/chronicle_chronicle-data/_data/backups/ backups@host:/srv/chronicle/
  ```
- `rclone` to S3/B2/etc. Run after `make backup` in cron.
- Bind-mount `/app/data/backups` over a path that's already on a
  replicated filesystem.

Backups inside `chronicle-data` survive container rebuild, but **not**
volume deletion (`docker compose down -v`). Always have at least one
offsite copy before you tag a release or run a migration you're nervous
about.

## 9. Restore procedure

`scripts/restore.sh` is gated; it refuses to run unless several
preconditions are met. Walk-through:

```sh
# 1. Identify the manifest you want to restore.
make backup-list
# Pick chronicle_manifest_<TS>.txt with the most recent timestamp you trust.

# 2. Stop the chronicle container. Restore over a running server is
# never safe — the script refuses if /healthz answers.
docker compose stop chronicle

# 3. Run the restore. RESTORE_ARGS is required:
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"
# Type "RESTORE" at the prompt.

# 4. Start chronicle back up. RunStartupHealthChecks validates the
# schema; if your image is incompatible with the restored migration
# version, it'll refuse to boot and tell you so in the logs.
docker compose start chronicle
docker compose logs -f chronicle
```

### Verifying a 0.0.2+ restore

After restore, confirm the player-character claim data round-tripped.
Pre-restore claims must reappear post-restore — if they don't, the dump
was taken before the claim was made (expected) or the dump's at a schema
version below 22 (the binary will have refused to boot already). To
verify manually:

```sh
# Quick row-count check.
docker compose exec -T chronicle-db sh -c \
  'MYSQL_PWD="$MARIADB_PASSWORD" mysql -u "$MARIADB_USER" "$MARIADB_DATABASE" \
     -e "SELECT COUNT(*) AS claimed_characters FROM entities WHERE owner_user_id IS NOT NULL;"'
```

End-to-end verification: log in as a player whose owned character
predates the backup, navigate to `My Characters`
(`GET /campaigns/:id/me`), and confirm the character card appears. If
the player was claiming a character on 0.0.2 and the card is missing
post-restore, the claim was created after the dump was taken — not a
restore bug.

### Common restore arguments

- `--db-only` — skip media tarball extraction (use after a corruption-only incident).
- `--media-only` — skip DB restore.
- `--yes` — skip the interactive RESTORE prompt. Required for unattended use.
- `--force` — allow restore over a non-empty target. Default refuses if the DB has tables or `MEDIA_PATH` is non-empty. Use with care.

### Unhappy paths

| Symptom | Likely cause | Fix |
|---|---|---|
| `RESTORE=failed reason=server_running` | Chronicle is still answering `/healthz`. | `docker compose stop chronicle` and retry. |
| `RESTORE=failed reason=sha_mismatch` | Manifest is paired with a different artifact than the one on disk. | Use a different manifest, or re-run `backup.sh` to regenerate. |
| `RESTORE=failed reason=target_not_empty` | Existing data in target DB or media dir. | Confirm intent, then re-run with `--force`. |
| `RESTORE=failed reason=tarball_unsafe` | Media tarball contains absolute paths or `..` traversal. | Don't use this artifact; it's tampered or corrupted. |

### Redis restore

The script does **not** automate the Redis dump replacement — doing so
requires Docker control from inside the chronicle container, which the
container doesn't have. To restore Redis sessions:

```sh
docker compose stop chronicle-redis
docker run --rm -v chronicle_chronicle-redisdata:/data \
  -v "$PWD":/restore alpine sh -c 'cp /restore/chronicle_redis_<TS>.rdb /data/dump.rdb'
docker compose start chronicle-redis
```

Most operators skip this — sessions regenerate on next login.

## 10. Troubleshooting common boot failures

Each row keyed off the actual log string emitted by current code, so
grep-and-find works. Lines with embedded `<...>` are placeholders.

### `migration <N> in DIRTY state` / `forcing migration version <N>`

The DB is mid-migration. Source: `internal/database/migrate.go`.
Chronicle auto-recovers: it forces the version back to a clean state and
retries. If you see this loop repeatedly, the migration itself is the
problem — go to §7 Scenario A and roll back from `chronicle_pre_migrate_*`.

### `database at migration <N> but code requires <M>`

Source: `internal/database/healthcheck.go` `checkMigrationVersion`.
The image is too new for the DB or the DB is too new for the image. If
you just upgraded and the DB is older: that's expected — wait for the
boot to finish migrations. If it's been hung for > 30s, the migration is
stuck; check `docker compose logs chronicle-db`. If the DB is newer than
the code (you rolled back the image), restore the matching pre-migration
backup (§7 Scenario A).

### `<K> critical column(s) missing`

Source: `internal/database/healthcheck.go` `checkCriticalColumns`. Migrations
haven't run, or a column was dropped manually. Run `make migrate-up`
from a host shell. If the migration itself is failing, check
`docker compose logs chronicle` for the migration error and roll back
per §7.

### `pre-migration backup skipped: mysqldump not found`

Source: `internal/database/healthcheck.go` `PreMigrationBackup`. The
chronicle image was built before 0.0.1 and is missing `mariadb-client`.
Rebuild with `docker compose build --no-cache chronicle` and retry.
Until you do, your upgrades have no automatic safety net.

### `pre-migration backup failed (non-fatal)`

Same source. The backup attempted but failed — usually because
`BACKUP_DIR` isn't writable or the disk is full. Check
`docker compose exec chronicle df -h /app/data` and the directory's
ownership. Boot continues without a backup; **fix this before the next
migration window.**

### `WARNING: Cannot create /app/data/media or /app/data/backups`

Source: `docker-entrypoint.sh`. The bind-mount on the host isn't owned
by the container's UID. Fix:

```sh
sudo chown -R 1000:1000 /path/to/host/chronicle-data
```

### `SECRET_KEY must be set`

Source: `docker-compose.yml`'s `${SECRET_KEY:?...}` syntax. You haven't
set the var. `openssl rand -base64 32 > /tmp/k && grep -v ^SECRET_KEY .env > .env.new && echo "SECRET_KEY=$(cat /tmp/k)" >> .env.new && mv .env.new .env`.

### Chronicle's `depends_on` waits forever for `chronicle-db`

The MariaDB container is unhealthy. Top suspect: `MYSQL_PASSWORD` and
`DB_PASSWORD` don't match. They must be identical (compose's MariaDB
container creates the user with `MYSQL_PASSWORD`; chronicle authenticates
with `DB_PASSWORD`). Less common: disk full at the host.

### `health check summary passed=K warnings=N failures=M` with `failures>0`

The boot will fail. Read the preceding `slog.Error` lines for the
specific check that failed. The summary is the last log line before
`os.Exit(1)`; everything actionable is above it.

## 11. Security checklist

Run through this before exposing Chronicle anywhere reachable.

- [ ] `SECRET_KEY` is 32+ bytes from `openssl rand -base64 32` and not
      committed anywhere.
- [ ] `DB_PASSWORD` is not `chronicle`, `password`, `secret`, `changeme`,
      `root`, or `admin`. The startup audit rejects these in production.
- [ ] `MYSQL_ROOT_PASSWORD` is not the default `rootsecret`.
- [ ] `MYSQL_PASSWORD` matches `DB_PASSWORD` exactly.
- [ ] `BASE_URL` starts with `https://`. The audit warns on `http://` in
      production because CSRF cookies are then ineffective.
- [ ] `ENV=production` is set so the audit warnings don't get suppressed.
- [ ] `DB_TLS_MODE=required` if the DB is on a different host. Optional
      if DB and chronicle are on the same Docker bridge.
- [ ] Daily `make backup` cron job + offsite copy of `$BACKUP_DIR`
      working and tested.
- [ ] First registered user is the legitimate site admin, not a test
      account left over from setup.
- [ ] Reverse proxy / Cosmos Cloud is enforcing TLS and not letting raw
      port 8080 leak to the public internet.

## 12. Out of scope for 0.0.1

These are deliberate non-goals; expect them in later releases:

- **Point-in-time recovery / binlog replay.** Backups are nightly
  snapshots, not continuous.
- **Snapshot replication / streaming standby.** Single-instance only.
- **Encrypted-at-rest backup artifacts.** Encrypt the offsite copy if
  needed (`gpg`, age, etc.).
- **Direct S3 / B2 / GCS shipping from `backup.sh`.** Use rclone in cron.
- **Automated DR drills.** §9 walks through a manual restore; automate
  it on your side if you need it scheduled.
- **Restore from the admin UI.** Restore is a sysadmin operation by
  design; it's destructive and requires `chronicle` to be stopped.

If any of those are blockers for your deployment, file an issue.
