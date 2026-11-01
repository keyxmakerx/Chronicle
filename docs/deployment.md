# Chronicle Deployment Runbook

Install, upgrade, back up, restore and troubleshoot a Chronicle instance.

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
12. [Out of scope](#12-out-of-scope)

---

## 1. TL;DR

```sh
git clone https://github.com/keyxmakerx/chronicle.git && cd chronicle
cp .env.example .env
# Set SECRET_KEY, DB_PASSWORD, MYSQL_ROOT_PASSWORD in .env.
# Generate SECRET_KEY with: openssl rand -base64 32
docker compose up -d
docker compose logs -f chronicle  # wait for "health check summary passed=N"
make backup-check                 # verify the backup pipeline before you need it
./tools/restore-drill.sh          # prove the first backup restores (docs/RESTORE-DRILL.md)
```

Open `http://localhost:8080` and register the first user (becomes site admin).
Read §11 before exposing it.

## 2. System requirements

- **Host:** Linux with Docker 24+ and Compose v2. macOS via Docker Desktop is
  fine for dev.
- **CPU/RAM:** 1 vCPU + 1 GB minimum; 2 GB for a campaign with media uploads.
- **Disk:** 5 GB minimum; volumes grow with media and installed packages.
- **Services:** MariaDB 10.11+ (`chronicle-db`) and Redis 7+
  (`chronicle-redis`), bundled. To bring your own, see §5.
- **Outbound network:** only when an admin installs a package (GitHub fetch).

## 3. Persistence inventory

**Bold = must back up.**

| State | Location (compose) | Back up? | If lost |
|---|---|---|---|
| **MariaDB tablespace** | volume `chronicle-dbdata` (`/var/lib/mysql`) | **Yes, primary** | Total data loss. |
| **User uploads** | volume `chronicle-data`, `/app/data/media/uploads` | **Yes** | Broken image references. |
| **User avatars** | volume `chronicle-data`, `/app/data/media/avatars` | **Yes** | Avatars revert to default. |
| Installed system packages | `/app/data/media/packages/systems/` | No | Re-install via Admin > Packages. |
| Foundry module package | `/app/data/media/packages/foundry-module/` | No | Same. |
| Pre-migration auto-backups | `/app/data/backups/chronicle_pre_migrate_*` | Operator's call | No safety net for the next migration. |
| Operator backups | `/app/data/backups/chronicle_db_*` etc. | n/a | n/a |
| Redis AOF (sessions) | volume `chronicle-redisdata` | Optional | Everyone logged out. |
| Migration version | `schema_migrations` in MariaDB | In the DB dump | n/a |
| **Secrets** (`SECRET_KEY`, DB passwords) | `.env` on host | **Yes, out of band** | Sessions invalidated; stored two-factor and sign-in provider secrets unreadable; DB auth breaks. |

State lives in three named volumes (`chronicle-data`, `chronicle-dbdata`,
`chronicle-redisdata`) and `.env`. Relational state, including player-character
claims (`entities.owner_user_id`), is in the dump and restored with it.

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

Wait for `health check summary passed=N warnings=0 failures=0`, then open
`http://localhost:8080`. The first registered user becomes site admin; register
yours at once if the host is publicly reachable.

### Bare metal

Needs Go 1.27+, MariaDB 10.11+, Redis 7+, Tailwind and templ (targets in
`Makefile`). Not recommended for production: the compose stack pins versions
and ships a matching `mariadb-client` for backups.

### Cosmos Cloud

`docker-compose.yml` carries the `cosmos-stack` labels. Import it directly;
Cosmos handles TLS and routing.

## 5. Configuration

Every env var Chronicle reads. **Bold = required in production.**

| Var | Default | Notes |
|---|---|---|
| `ENV` | `development` (compose: `production`) | `production` raises security audit warnings to errors. |
| `PORT` | `8080` | Change the compose port mapping, not this. |
| **`BASE_URL`** | `http://localhost:8080` | Production must be `https://`. Also the base of the sign-in provider redirect: register `BASE_URL` + `/login/oidc/callback` with the provider. |
| `LOG_LEVEL` | `debug` | `info` / `warn` / `error` in production. |
| `DB_HOST` | `localhost:3306` | `host:port`; compose sets `chronicle-db:3306`. |
| `DB_USER` | `chronicle` | |
| **`DB_PASSWORD`** | `chronicle` | The audit rejects `chronicle`, `password`, `secret`, `changeme`, `root`, `admin`. |
| `DB_NAME` | `chronicle` | |
| `DB_TLS_MODE` | (disabled) | `required` / `skip-verify` / `preferred`. Production with a remote DB: `required`. |
| `DATABASE_URL` | (empty) | Full DSN; bypasses the `DB_*` vars. |
| `DB_MAX_OPEN_CONNS` | `25` | |
| `DB_MAX_IDLE_CONNS` | `5` | |
| `DB_CONN_MAX_LIFETIME` | `5m` | |
| `REDIS_URL` | `redis://localhost:6379` | |
| **`SECRET_KEY`** | required | 32+ bytes base64 (`openssl rand -base64 32`). PASETO session signing key; rotating it logs everyone out. It also derives the keys for stored two-factor secrets and the sign-in provider's client secret: after a rotation users sign in with a recovery code (or an admin switches their two-factor off), and the provider secret is re-entered under Admin > Sign-ins & sessions > Sign-in provider. |
| `SESSION_TTL` | `720h` | |
| `EXTENSIONS_PATH` | `./extensions` | User-installable content extensions. |
| `MAX_UPLOAD_SIZE` | `10485760` | Plain integer bytes; `10MB` does not parse and silently falls back to the default. |
| `MEDIA_PATH` | `./data/media` | `/app/data/media` in the container. |
| `MEDIA_SIGNING_SECRET` | (auto) | HMAC-SHA256 key for signed media URLs; generated if empty. |
| `MEDIA_SIGNING_SECRET_FILE` | `./data/.signing-secret` | Where the generated secret is persisted. |
| `MEDIA_SERVE_RATE_LIMIT` | `300` | Requests/min/IP for `GET /media/:id`; per-IP only works if `TRUSTED_PROXY_CIDRS` is right. |
| `TRUSTED_PROXY_CIDRS` | loopback + private ranges | Comma-separated CIDRs or addresses whose `X-Real-IP` / `X-Forwarded-For` are believed. **Replaces** the default. See "Client IP behind a reverse proxy". |
| `BACKUP_DIR` | `/app/data/backups` | Empty is unsupported: the admin UI reports "not configured" and the pre-migration backup is skipped. |
| `BACKUP_RETENTION_DAYS` | `7` | Used by `scripts/backup.sh`. The pre-migration rotator has its own fixed 7 days. |
| `BACKUP_REQUIRED` | `0` (binary), `1` (compose) | `1`/`true` makes the pre-migration backup mandatory: any failure (no `mysqldump`, empty dump, manifest write failure) aborts startup before migrations apply, for pending core and plugin migrations. Default is warn and proceed. Use `1` in production. |
| `BOOT_FAIL_BACKOFF` | `45s` | Sleep before `os.Exit(1)` on an unrecoverable boot error, so `restart: unless-stopped` retries about once a minute instead of looping hot and repeating pre-migration backups. Use `2s` in dev. |
| `BACKUP_SCRIPT_PATH` | `/app/scripts/backup.sh` | Used by the admin "Run backup" button. |
| `RESTORE_SCRIPT_PATH` | `/app/scripts/restore.sh` | Used by the admin restore page. |
| `CHRONICLE_VERSION` | empty except on tag builds | Read by `GET /api/version` (first, then the compiled-in VCS revision, then the main module version, then `unknown`), the `host.build` diagnostic, and the pre-migration manifest's `chronicle_version=` line. CI sets it only for `v*` tag builds. |
| `GITHUB_TOKEN` | (empty) | Optional; raises the GitHub API rate limit for update checks. |
| `MYSQL_ROOT_PASSWORD` | compose, **required** | Sets the bundled MariaDB root password on first initialisation only; compose refuses to start without it. An older install may still have `rootsecret`; see below. |
| `MYSQL_PASSWORD` | derived | Compose passes `DB_PASSWORD` to the bundled MariaDB; do not set separately. |

### Rotating the root password

The MariaDB image reads `MYSQL_ROOT_PASSWORD` only on an empty data volume, so
editing `.env` does not change an existing database. The DB port is not
published, so this is defence in depth. To rotate:

```
docker compose exec -T chronicle-db mariadb -uroot -p"$OLD_ROOT_PASSWORD" \
  -e "ALTER USER 'root'@'localhost' IDENTIFIED BY '<new>'; ALTER USER 'root'@'%' IDENTIFIED BY '<new>'; FLUSH PRIVILEGES;"
```

Then set the new value in `.env` so the healthcheck and backup tooling agree.

### Client IP behind a reverse proxy

Echo's `IPExtractor` believes `X-Real-IP` / `X-Forwarded-For` **only** from a
peer inside `TRUSTED_PROXY_CIDRS`; otherwise it records the peer. Per-IP rate
limits, the media serve limit and the address on every audit row depend on it.

The default covers loopback and private ranges: right for a proxy on the same
Docker network, wrong for one arriving over a **mesh VPN** (e.g.
`100.64.0.0/10`). Then every visitor is recorded as the proxy and shares one
rate-limit bucket and one audit address.

Check the running service:

```bash
docker compose logs --tail 500 chronicle \
  | grep -oE '"?remote_ip"?[=:]"?[^ ",}]+' | sort | uniq -c | sort -rn | head
```

One private address on nearly every row means the headers are not believed.
Add the proxy's own address, not its range: every host in a trusted range can
dictate the client IP of any request it relays.

```
TRUSTED_PROXY_CIDRS=127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fd00::/8,<proxy-address>
```

Then forge a header from outside the network and re-read the logs:

```bash
curl -s -o /dev/null -H 'X-Forwarded-For: 203.0.113.99' https://your-instance/
```

If `203.0.113.99` appears, the proxy appends to `X-Forwarded-For` instead of
replacing it, so any visitor can choose their recorded address. Revert and fix
the proxy to set `X-Real-IP` or overwrite `X-Forwarded-For`. If the real client
address appears, the setup is sound.

An unparseable entry stops the server at startup, naming the value.

## 6. Upgrade / redeploy

```sh
make backup                                            # 1. operator snapshot
docker compose pull                                    # 2. fetch new images
docker compose up -d --no-deps chronicle               # 3. swap chronicle only
docker compose logs -f chronicle                       # 4. watch the boot
curl -s localhost:8080/api/version                     # 5. confirm what is RUNNING
```

Step 5 is the only one that reports the running software; steps 1-4 can all
succeed while the container never changes. `--no-deps` keeps MariaDB and Redis
up.

A server built from a version tag tells its site admins on Admin > Home when a
newer release is out, from a once-a-day look at the GitHub releases. Admin >
Security > Overview turns that look off for a server that must not call out.

`docker compose up -d` alone is not an upgrade: it does not re-pull a tag that
exists locally. The `chronicle` service sets `pull_policy: always`, but keep
the explicit `pull`, whose output shows whether anything new arrived.

The `chronicle` service has no `build:` section, so a local build can never be
tagged onto the published `ghcr.io/<org>/chronicle:latest`. To run from source,
use the override, which tags `chronicle:local`:

```sh
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
# or: make docker-all-local
```

Step 4 logs, in order (backup lines only when the release ships pending
migrations, otherwise `no pending migrations`; a plugin-only release logs
`pending plugin migration(s) detected — backing up before applying` between the
health checks and the plugin migrations):

```
creating pre-migration backup file=/app/data/backups/chronicle_pre_migrate_<TS>.sql.gz
pre-migration backup completed file=...
migrations applied
migration version validated version=N
health check summary passed=K warnings=0 failures=0
```

If `failures=0` is missing and the container exits, the release is broken: go
to §7. For `pre-migration backup skipped: mysqldump not found`, see §10.

### One-way migrations: the calendar clean slate

Three plugin migrations delete the old calendar's data:
`calendar/019_calv5_clean_slate`, `timeline/002_calv5_clear_calendar_links`,
`sessions/006_calv5_clear_availability`. Their down files are deliberately
empty, so the data returns only from a backup. If still pending on an install:
run `make backup`, set `BACKUP_REQUIRED=1` (pending plugin migrations trigger the
snapshot too), and deploy only a build at or after commit `1bda7d6`; build
`bfcaf24` shipped a `019` that dropped tables other plugins reference, and a DB
that booted it recorded version 19 permanently. Watch the log for `019`, `002`,
`006`, then confirm the version with step 5.

Rolling back to an older image afterwards still boots (ADR-045, §7) but the
calendar data stays gone; restore the `chronicle_pre_migrate_db_<timestamp>.sql.gz`
from that boot (§9).

### Deploying alongside the Foundry module and game system packages

Chronicle, the Foundry module and each system package ship separately. In this
order:

1. **Back up**, set `BACKUP_REQUIRED=1`, and set `BASE_URL` to the public
   `https://` address. The Foundry connect line (Campaign > Manage > Foundry)
   is built from `BASE_URL`; make a new line after fixing it. Older keys are not
   revoked.
2. **Deploy Chronicle** (steps 1-5). New tables and columns are added at boot,
   each guarded so a re-run is harmless; the pre-migration snapshot comes first.
3. **Release the Foundry module** (module repo, Actions > Release, with a
   version), then update it inside Foundry. Chronicle shows the module version a
   key last reported.
4. **Release the system packages** (Draw Steel 0.13.14 or later, and 5e), then
   Admin > Packages > check for updates and update each. An update adds only the
   sheet fields it introduced to existing campaigns; it never restores a field a
   GM deleted and never touches pages.
5. Match each Foundry user to their Chronicle member, then check the status dot
   on the campaign's Foundry row.

### Which image is actually running?

Ask the process:

```sh
curl -s localhost:8080/api/version        # commit SHA (or the tag on a release build)
# Admin > Diagnostics > host.build        # binary path, size, mtime, uptime, VCS stamp
```

For the image identity:

```sh
docker inspect --format '{{.Image}}' chronicle                  # the container's real image ID
docker image inspect --format '{{.Id}}' ghcr.io/keyxmakerx/chronicle:latest   # what the tag points at now
```

If the IDs differ, the tag has moved and labels read off it
(`org.opencontainers.image.revision`, build date) describe a different artifact
than the running one. A label is a claim by whoever last wrote the tag, not
about the process.

## 7. Rollback

**`migrate-down` safety.** Run `make backup` and read the down migration
before any `make migrate-down` on live data. `000001_baseline.down.sql` wipes
every core table. `000028_public_grant_subject.down.sql` deletes every
`subject_type='public'` row from `entity_permissions`. Backups in the
`chronicle-data` volume do not survive `docker compose down -v`; keep an
offsite copy (§8) before any `migrate-down` or volume operation.

### Downgrade behavior (ADR-045)

An older image does not crash-loop. When the database's recorded version is
ahead of the build, `MigrateWithBackup` logs `database is AHEAD of this build —
skipping migrations and starting anyway` and starts. Migrations are additive, so
the older binary runs on the newer schema; features from newer migrations are
unavailable until you redeploy. The state shows at `/admin/database` ("DB ahead
of build" banner) and `GET /admin/database/status`.

A DB rollback (restore a pre-migration backup) is needed only if the newer
version dropped or renamed a column the older build reads; the startup health
check then fails with a "critical column" error instead of serving a broken app.

- **Dirty database** (a migration failed partway): the runner fails fast with
  restore guidance rather than forcing and retrying, which could loop on a
  non-idempotent migration. Restore the latest pre-migration backup or repair
  `schema_migrations` by hand, then redeploy.
- **Boot failure:** the process sleeps `BOOT_FAIL_BACKOFF` (§5) before exiting.

### Scenario A: health checks failed at boot (most common)

The container exited without serving traffic; the schema may or may not have
advanced.

```sh
docker compose logs chronicle | grep -E 'health check|migration|critical column'
```

For a migration version mismatch, roll back the schema. Pre-migration captures
use `scripts/backup.sh`'s manifest format
(`chronicle_pre_migrate_manifest_<TS>.txt` plus db/media/redis files with
sha256), so `scripts/restore.sh --manifest <path>` restores from one, as does
the admin restore UI at `/admin/restore`, which lists them with operator
backups (`chronicle_pre_migrate=1` in the manifest body):

```sh
# Most recent pre-migration manifest (one per boot that ran migrations):
docker compose exec -T chronicle ls -lt /app/data/backups/chronicle_pre_migrate_manifest_*.txt | head -3

docker compose stop chronicle   # leave DB + Redis up

# restore.sh verifies each artifact's sha256 before touching the live DB
docker compose run --rm chronicle sh /app/scripts/restore.sh \
  --manifest /app/data/backups/chronicle_pre_migrate_manifest_<TS>.txt \
  --yes --force

# Pin the previous image tag in docker-compose.yml or your registry, then:
docker compose up -d chronicle
```

For a legacy `chronicle_pre_migrate_<TS>.sql.gz` with no manifest, restore the
DB by hand:

```sh
docker compose exec -T chronicle sh -c \
  'gunzip -c /app/data/backups/chronicle_pre_migrate_<TS>.sql.gz \
     | MYSQL_PWD="$DB_PASSWORD" mysql -h "$DB_HOST" -u "$DB_USER" "$DB_NAME"'
```

Rolling a tag back past a migration boundary needs the snapshot taken just
before that migration, then the older tag pinned. Data added by that migration
is dropped.

### Scenario B: server up, a feature is broken

No DB restore unless the release ran a destructive migration (then use A). Roll
the image tag back:

```sh
# Edit docker-compose.yml: image: ghcr.io/.../chronicle:<previous-tag>
docker compose up -d --no-deps chronicle
```

### Scenario C: data corruption or accidental destructive admin action

```sh
docker compose stop chronicle
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"
# Type RESTORE when prompted.
docker compose start chronicle
```

`RunStartupHealthChecks` validates the restored schema against the image's
`ExpectedCoreMigrationVersion`; if the manifest needs a different code
revision, chronicle refuses to start. Pin a matching image tag.

## 8. Backup procedure

Prove backups restore: run `./tools/restore-drill.sh` monthly and before
upgrades (`docs/RESTORE-DRILL.md`).

`scripts/backup.sh` snapshots DB, media and (optionally) Redis:

```sh
make backup                                  # full snapshot
make backup BACKUP_ARGS="--no-media"          # DB only
make backup BACKUP_ARGS="--no-redis --retention 30"
make backup-check                             # validate without writing
make backup-list                              # list the volume
```

Output goes to `$BACKUP_DIR`. Each run writes:

- `chronicle_db_<TS>.sql.gz`: `mysqldump --single-transaction --routines --triggers`, gzipped.
- `chronicle_media_<TS>.tar.gz`: `tar -czf` of `MEDIA_PATH`.
- `chronicle_redis_<TS>.rdb`: `redis-cli --rdb`, when `redis-cli` is on PATH (best effort).
- `chronicle_manifest_<TS>.txt`: sha256s, chronicle version, migration version. Drives `restore.sh`.

The script deletes artifacts older than `BACKUP_RETENTION_DAYS` and never
touches `chronicle_pre_migrate_*`, which a separate Go rotator manages.

### Daily backup from the admin page

Site admin > Backups & restore > Daily backup runs `backup.sh` in the server
once a day at the chosen hour (server time) and keeps the chosen number of days.
A failed run shows on that page and emails active site admins when email is set
up. It writes to the same `$BACKUP_DIR`, so an offsite copy is still needed. Use
it or the host cron job, not both.

### Cron (daily at 03:00)

```cron
0 3 * * * cd /opt/chronicle && /usr/bin/make backup >> /var/log/chronicle-backup.log 2>&1
```

`make backup` uses `docker compose exec -T`, so it needs no TTY.

### Offsite copy

- `rsync`:
  ```sh
  rsync -avz /var/lib/docker/volumes/chronicle_chronicle-data/_data/backups/ backups@host:/srv/chronicle/
  ```
- `rclone` to S3/B2/etc. after `make backup` in cron.
- Bind-mount `/app/data/backups` on a replicated filesystem.

Backups survive a container rebuild but not `docker compose down -v`. Keep at
least one offsite copy before a release or a risky migration.

## 9. Restore procedure

`scripts/restore.sh` refuses to run unless its preconditions hold.

```sh
make backup-list   # 1. pick the newest chronicle_manifest_<TS>.txt you trust

# 2. The script refuses if /healthz answers.
docker compose stop chronicle

# 3. RESTORE_ARGS is required; type "RESTORE" at the prompt.
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"

# 4. RunStartupHealthChecks validates the schema; an incompatible image refuses to boot.
docker compose start chronicle
docker compose logs -f chronicle
```

### Verifying a restore

Claims made before the backup must reappear. Missing ones mean the claim
postdates the dump:

```sh
docker compose exec -T chronicle-db sh -c \
  'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u "$MYSQL_USER" "$MYSQL_DATABASE" \
     -e "SELECT COUNT(*) AS claimed_characters FROM entities WHERE owner_user_id IS NOT NULL;"'
```

End to end: log in as a player whose character predates the backup, open
`GET /campaigns/:id/characters`, and confirm the card appears under Yours.

### Common restore arguments

- `--db-only`: skip media extraction.
- `--media-only`: skip the DB restore.
- `--yes`: skip the RESTORE prompt; required unattended.
- `--force`: restore over a non-empty target. The default refuses if the DB has tables or `MEDIA_PATH` is non-empty.

### Unhappy paths

| Symptom | Cause | Fix |
|---|---|---|
| `RESTORE=failed reason=server_running` | Chronicle still answers `/healthz`. | `docker compose stop chronicle`, retry. |
| `RESTORE=failed reason=sha_mismatch` | Manifest paired with a different artifact than on disk. | Use another manifest, or re-run `backup.sh`. |
| `RESTORE=failed reason=target_not_empty` | Target DB or media dir has data. | Confirm intent, re-run with `--force`. |
| `RESTORE=failed reason=tarball_unsafe` | Media tarball has absolute paths or `..`. | Do not use it; it is tampered or corrupt. |

### Redis restore

`restore.sh` does not replace the Redis dump (the container has no Docker
control). To restore sessions:

```sh
docker compose stop chronicle-redis
docker run --rm -v chronicle_chronicle-redisdata:/data \
  -v "$PWD":/restore alpine sh -c 'cp /restore/chronicle_redis_<TS>.rdb /data/dump.rdb'
docker compose start chronicle-redis
```

Most operators skip this; sessions regenerate on next login.

## 10. Troubleshooting common boot failures

Headings are the log strings the code emits.

### `migration <N> in DIRTY state` / `forcing migration version <N>`

A migration failed partway. Chronicle fails fast because a partly applied
migration can be unsafe to re-run. Restore the latest `chronicle_pre_migrate_*`
backup (§7 Scenario A) or repair `schema_migrations` by hand, then redeploy.

### `database at migration <N> but code requires <M>`

The recorded version differs from the image's. DB older after an upgrade: wait
for migrations (over 30s, check `docker compose logs chronicle-db`). DB newer
than the code (rolled-back image): restore the matching pre-migration backup
(§7 Scenario A).

### `<K> critical column(s) missing`

Migrations have not run, or a column was dropped by hand. Run `make migrate-up`
from a host shell; if the migration itself fails, read `docker compose logs
chronicle` and roll back per §7.

### `pre-migration backup skipped: mysqldump not found`

The image lacks `mariadb-client`. Pull a current image (`docker compose pull
chronicle`) or rebuild:
`docker compose -f docker-compose.yml -f docker-compose.build.yml build --no-cache chronicle`.
Until then upgrades have no automatic safety net.

### `pre-migration backup failed (non-fatal)`

Usually `BACKUP_DIR` is not writable or the disk is full. Check
`docker compose exec chronicle df -h /app/data` and the directory's ownership.
Boot continues without a backup; fix it before the next migration.

### `WARNING: Cannot create /app/data/media or /app/data/backups`

From `docker-entrypoint.sh`: the host bind-mount is not owned by the container
UID.

```sh
sudo chown -R 1000:1000 /path/to/host/chronicle-data
```

### `SECRET_KEY must be set`

From `docker-compose.yml`'s `${SECRET_KEY:?...}`. Set it:
`openssl rand -base64 32 > /tmp/k && grep -v ^SECRET_KEY .env > .env.new && echo "SECRET_KEY=$(cat /tmp/k)" >> .env.new && mv .env.new .env`.

### `depends_on` waits forever for `chronicle-db`

MariaDB is unhealthy. Most likely `DB_PASSWORD` changed after the volume was
created: MariaDB sets the user password only on first initialisation, so
Chronicle now sends one the database does not have. Change the password inside
MariaDB or restore the old value. Otherwise check host disk space.

### `health check summary passed=K warnings=N failures=M` with `failures>0`

Boot fails. The `slog.Error` lines above the summary name the failed check; the
summary is the last line before `os.Exit(1)`.

## 11. Security checklist

Before exposing Chronicle:

- [ ] `SECRET_KEY` is 32+ bytes from `openssl rand -base64 32` and not committed.
- [ ] `DB_PASSWORD` is not `chronicle`, `password`, `secret`, `changeme`, `root` or `admin` (rejected by the production audit).
- [ ] `MYSQL_ROOT_PASSWORD` is not `rootsecret`.
- [ ] `BASE_URL` starts with `https://` (the audit warns on `http://` because CSRF cookies are then ineffective).
- [ ] `ENV=production`, or the audit warnings stay warnings.
- [ ] `DB_TLS_MODE=required` if the DB is on another host.
- [ ] Daily backup (admin page or `make backup` cron) plus an offsite copy of `$BACKUP_DIR`, tested.
- [ ] The first registered user is the real site admin, not a leftover test account.
- [ ] The reverse proxy / Cosmos enforces TLS and port 8080 is not reachable from the internet.

## 12. Out of scope

- Point-in-time recovery and binlog replay; backups are snapshots.
- Replication and standby; single instance only.
- Encrypted-at-rest backups; encrypt the offsite copy (`gpg`, age).
- Direct S3/B2/GCS shipping from `backup.sh`; use rclone in cron.
- Scheduled DR drills; `tools/restore-drill.sh` proves a backup restores, scheduling it is yours.

File an issue if any of these block you.
