# Chronicle Deployment Runbook

The operator's reference for installing, upgrading, backing up, restoring,
and troubleshooting a Chronicle instance.

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
./tools/restore-drill.sh          # prove your first backup actually restores (see docs/RESTORE-DRILL.md)
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
player-to-character claim relationships (`entities.owner_user_id`, migration
22). `mysqldump --single-transaction` captures it transparently and
`restore.sh` brings it back identically — no separate handling.

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

Requires Go 1.27+, MariaDB 10.11+, Redis 7+, and a build of Tailwind +
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
| `MEDIA_SERVE_RATE_LIMIT` | `300` | Requests/min/IP for `GET /media/:id`. Per-IP only works if the row below is right. |
| `TRUSTED_PROXY_CIDRS` | loopback + private ranges | Comma-separated CIDR blocks or bare addresses whose `X-Real-IP` / `X-Forwarded-For` headers are believed. **Replaces** the default, does not extend it. See "Client IP behind a reverse proxy" below. |
| `BACKUP_DIR` | `/app/data/backups` | Where backups land. Defaults to the persistent `/app/data` volume so a fresh deploy works without operator setup. Override only if you mount backups on a different path. Setting it explicitly to empty is unsupported (the admin UI will surface a "not configured" error and the in-process pre-migration backup will be skipped). |
| `BACKUP_RETENTION_DAYS` | `7` | Used by `scripts/backup.sh`. The in-process rotator uses a separate hardcoded 7d for `chronicle_pre_migrate_*` artifacts. |
| `BACKUP_REQUIRED` | `0` | `1`/`true` makes the in-process pre-migration capture mandatory: any failure (mysqldump missing, dump zero bytes, manifest write fails) aborts startup before migrations apply. Covers both gates — pending core migrations (`MigrateWithBackup`) and pending plugin migrations (`main.go`'s `PendingPluginMigrations` gate). Use in production; the default fail-open (warn + proceed) suits dev setups without `mariadb-client`. |
| `BACKUP_SCRIPT_PATH` | `/app/scripts/backup.sh` | Used by the admin "Run backup" button. |
| `RESTORE_SCRIPT_PATH` | `/app/scripts/restore.sh` | Used by the admin restore page. |
| `CHRONICLE_VERSION` | (empty, except on tag builds) | Read by `GET /api/version` (highest precedence, then the compiled-in VCS revision, then the main module version, then `unknown`), by the `host.build` diagnostic, and stamped into the pre-migration manifest's `chronicle_version=` line. CI sets it as a Docker build arg only for `v*` tag builds; a `main`-branch push leaves it empty since the binary already carries its own commit SHA (`vcs.revision`) that `/api/version` falls through to. Set it yourself only for a human-chosen name. |
| `MYSQL_ROOT_PASSWORD` | (compose, **required**) | Compose-only; sets the bundled MariaDB's root password on first initialisation only — compose refuses to start without it. An install predating this requirement may still have the old default `rootsecret` unless rotated; see "Rotating the root password" below. |
| `MYSQL_PASSWORD` | (compose) | Compose-only; must match `DB_PASSWORD`. |

### Rotating the root password

`MYSQL_ROOT_PASSWORD` is read by the MariaDB image only when the data volume
is empty, so changing it in `.env` does nothing to an existing database. To
rotate on a running install (the DB port isn't published, so this is
defence-in-depth, not an open door):

```
docker compose exec -T mariadb mariadb -uroot -p"$OLD_ROOT_PASSWORD" \
  -e "ALTER USER 'root'@'localhost' IDENTIFIED BY '<new>'; ALTER USER 'root'@'%' IDENTIFIED BY '<new>'; FLUSH PRIVILEGES;"
```

Then set the new value in `.env` so the healthcheck and backup tooling agree.

### Client IP behind a reverse proxy

Chronicle resolves the client address with Echo's `IPExtractor`, believing
`X-Real-IP`/`X-Forwarded-For` **only** from a peer inside
`TRUSTED_PROXY_CIDRS` (otherwise it records the peer itself). Per-IP rate
limiting, the media serve limit, and the address in every audit/security row
depend on getting this right.

The default covers loopback and private ranges — correct for a proxy on the
same Docker network, wrong for one reaching Chronicle over a **mesh VPN**
(peer address in `100.64.0.0/10`, matching nothing in the default): every
public visitor then gets recorded as the proxy, sharing one rate-limit bucket
and one audit-log address.

Check the running service rather than guessing:

```bash
docker compose logs --tail 500 chronicle \
  | grep -oE '"?remote_ip"?[=:]"?[^ ",}]+' | sort | uniq -c | sort -rn | head
```

A single private address on nearly every row means the headers aren't
believed. Set the variable to the proxy's own address — **name the proxy, not
its range**: every host inside a trusted range can dictate the client IP of
any request it relays, so a mesh-wide entry hands that power to every peer
that ever joins.

```
TRUSTED_PROXY_CIDRS=127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fd00::/8,<proxy-address>
```

Then verify — a wrong answer here is worse than the problem. Forge a header
from outside the network:

```bash
curl -s -o /dev/null -H 'X-Forwarded-For: 203.0.113.99' https://your-instance/
```

Re-read the logs. `203.0.113.99` appearing means the proxy is **appending**
to `X-Forwarded-For` instead of replacing it, so any visitor can choose their
own recorded address — revert the setting and fix the proxy to set
`X-Real-IP` or overwrite `X-Forwarded-For` first. If the real client address
appears instead, the configuration is sound.

An unparseable entry stops the server at startup naming the offending value,
rather than silently disabling client-IP resolution.

## 6. Upgrade / redeploy

```sh
make backup                                            # 1. operator snapshot
docker compose pull                                    # 2. fetch new images
docker compose up -d --no-deps chronicle               # 3. swap chronicle only
docker compose logs -f chronicle                       # 4. watch the boot
curl -s localhost:8080/api/version                     # 5. confirm what is RUNNING
```

Step 5 is not ceremony: it is the only step that reports the software you are
actually running — steps 1–4 can all succeed while the running container
never changes.

**`docker compose up -d` alone is not an upgrade.** It will not rebuild and
will not re-pull when an image with that tag already exists locally — it
starts what is already on the host. The compose file sets `pull_policy:
always` on the `chronicle` service so `up` does fetch the published image,
but keep the explicit `docker compose pull` in the sequence: it is the step
whose output tells you whether anything new arrived.

**Never build a local image onto the published tag.** The `chronicle` service
deliberately has no `build:` section, so `docker compose build` cannot tag a
local build onto the published `ghcr.io/<org>/chronicle:latest` name — a
second producer for that tag would make it impossible to tell a local build
apart from the published one. To run from source, use the override, which
tags the result `chronicle:local` instead:

```sh
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
# or: make docker-all-local
```

### One-way migrations: the calendar clean slate

Three plugin migrations (calendar rebuild) delete the old calendar's data:
`calendar/019_calv5_clean_slate`, `timeline/002_calv5_clear_calendar_links`,
`sessions/006_calv5_clear_availability`. **Their down files are deliberately
empty** — once run, that data returns only from a backup. Still-pending on an
install: back up yourself first (`make backup`), set `BACKUP_REQUIRED=1` so
the pre-migration snapshot is mandatory (pending plugin migrations trigger it
too, not only core ones), and deploy only a build at or after commit
`1bda7d6` — an earlier build (`bfcaf24`) shipped a version of `019` that
dropped tables other plugins still point at, and a DB that ever booted it
recorded version 19 permanently. Watch the boot log for `019`, `002`, `006`,
then confirm the version with step 5 above.

Rolling back to an older image afterwards still boots (ADR-045, §7), but the
calendar data stays gone; restore the `chronicle_pre_migrate_db_<timestamp>.sql.gz`
snapshot taken on that boot to get it back (§9).

### Deploying alongside the Foundry module and game system packages

Chronicle, the Foundry module and each game system package ship separately,
and a Chronicle deploy changes none of the others. In this order:

1. **Back up**, set `BACKUP_REQUIRED=1`, and make sure `BASE_URL` is the public
   `https://` address. The Foundry connect line (Campaign > Manage >
   Foundry) is built from `BASE_URL`; a line made while it still says
   `localhost` points Foundry at the wrong place. Make a new connect line after
   fixing it. Older keys are not revoked.
2. **Deploy Chronicle** (steps 1–5 above). New tables and columns are added at
   boot, each guarded so a re-run is harmless; the pre-migration snapshot is
   taken first. Confirm the running version with step 5.
3. **Release the Foundry module** (the module repo's Actions > Release, with a
   version), then update it inside Foundry. Chronicle shows the module version a key last reported.
4. **Release the game system packages** (Draw Steel 0.13.14 or later, and 5e),
   then in Admin > Packages check for updates and update each one. Updating a
   package adds only the sheet fields that update introduced to existing
   campaigns, never restores a field a GM deleted, and never touches pages.
5. Match each Foundry user to their Chronicle member, then use the campaign's
   Foundry row to check the status dot.

### Which image is actually running?

Ask the process first — it's the only thing that can testify about itself:

```sh
curl -s localhost:8080/api/version        # commit SHA (or the tag on a release build)
# Admin > Diagnostics > host.build        # binary path, size, mtime, uptime, VCS stamp
```

Only if you need the image identity too:

```sh
docker inspect --format '{{.Image}}' chronicle                  # the container's REAL image ID
docker image inspect --format '{{.Id}}' ghcr.io/keyxmakerx/chronicle:latest   # what the TAG points at now
```

**If those two IDs differ, the tag has moved** and any label read off it
describes a different artifact than the one running — a local `:latest`
image can carry consistent, truthful labels
(`org.opencontainers.image.revision`, build date) while the running
container was created from an entirely different image. A label is a claim
made by whoever last wrote the tag, never a claim about a process.

In step 4 you should see, in order (backup lines appear only when the release
ships pending migrations; none logs `no pending migrations` instead; a
release shipping only PLUGIN migrations logs `pending plugin migration(s)
detected — backing up before applying` between the health checks and the
plugin migrations):

```
creating pre-migration backup file=/app/data/backups/chronicle_pre_migrate_<TS>.sql.gz
pre-migration backup completed file=...
migrations applied
migration version validated version=N
health check summary passed=K warnings=0 failures=0
```

`pre-migration backup skipped: mysqldump not found` means the image is
missing `mariadb-client` — pull a current one (`docker compose pull
chronicle`) or rebuild from source via the override:
`docker compose -f docker-compose.yml -f docker-compose.build.yml build --no-cache chronicle`.

If `failures=0` doesn't appear and the container exits, the release is
broken — go to §7.

The `--no-deps` flag keeps MariaDB and Redis up across the swap; they restart
only when their own image changes.

## 7. Rollback

**`migrate-down` safety rule.** Never run `make migrate-down` on live data
without first running `make backup` and reviewing what the down migration
does. `000001_baseline.down.sql` wipes all 34 core tables (total data loss).
`000028_public_grant_subject.down.sql` deletes every `subject_type='public'`
row from `entity_permissions` (unrecoverable without a restore). Backups
inside the `chronicle-data` volume also do not survive `docker compose down
-v` — a volume-delete wipes your only local safety net. Keep an offsite copy
(§8) before any `migrate-down` or volume operation you're not fully
confident about. The three scenarios below all use the existing health-check
gate plus the pre-migration backup — no new server code is needed for a rollback.

### Downgrade / rollback behavior (ADR-045)

An **image downgrade does not crash-loop**. When you pull an OLDER image whose
migration set is behind the database's recorded version, the boot runner
(`MigrateWithBackup`) detects "DB ahead of build", logs `database is AHEAD of
this build — skipping migrations and starting anyway`, and **starts
normally**. Migrations are additive, so the older binary runs fine on the
newer schema; features added after the build's highest migration are simply
unavailable until you redeploy a newer image. Confirm the state at
**`/admin/database`** (a "DB ahead of build" banner) or `GET
/admin/database/status`.

You only need a **DB rollback** (restore a pre-migration backup) if the newer
version *dropped or renamed* a column the older build reads — then the
startup health checks fail fast with a "critical column" error instead of
serving a broken app.

- **Dirty database** (a migration failed partway): the runner **fails fast**
  with restore guidance instead of auto-forcing-and-retrying, which could
  loop forever on a non-idempotent migration. Restore the most recent
  pre-migration backup or repair `schema_migrations` manually, then redeploy.
- **Boot-failure backoff** (`BOOT_FAIL_BACKOFF`, default `45s`): on any
  unrecoverable boot error the process sleeps before `os.Exit(1)`, so
  `restart: unless-stopped` retries ~1/min instead of hot-looping ~60/min and
  flooding logs/disk with repeated pre-migration backups. Lower it in dev
  (e.g. `BOOT_FAIL_BACKOFF=2s`).

### Scenario A — server failed health checks at boot (most common)

The chronicle container `os.Exit(1)`'d cleanly without serving any traffic.
The DB schema may or may not have advanced.

```sh
docker compose logs chronicle | grep -E 'health check|migration|critical column'
```

Read which check failed. If it's a migration version mismatch, roll back the
schema. Pre-migration captures share `scripts/backup.sh`'s manifest format
(`chronicle_pre_migrate_manifest_<TS>.txt` plus per-artifact db/media/redis
files with sha256 verification), so `scripts/restore.sh --manifest <path>`
rolls back from one directly — or use the **admin restore UI** at
`/admin/restore`, where pre-migration manifests are listed alongside
operator-triggered backups (`chronicle_pre_migrate=1` line in the body):

```sh
# Most recent pre-migration manifest (one per boot that ran migrations):
docker compose exec -T chronicle ls -lt /app/data/backups/chronicle_pre_migrate_manifest_*.txt | head -3

docker compose stop chronicle   # leave DB + Redis up

# restore.sh verifies sha256 of each artifact before touching the live DB
docker compose run --rm chronicle sh /app/scripts/restore.sh \
  --manifest /app/data/backups/chronicle_pre_migrate_manifest_<TS>.txt \
  --yes --force

# Pin the previous image tag in docker-compose.yml or your registry, then:
docker compose up -d chronicle
```

For DB-only rollbacks against a legacy `chronicle_pre_migrate_<TS>.sql.gz`
file (no manifest), the manual `gunzip | mysql` approach still works:

```sh
docker compose exec -T chronicle sh -c \
  'gunzip -c /app/data/backups/chronicle_pre_migrate_<TS>.sql.gz \
     | MYSQL_PWD="$DB_PASSWORD" mysql -h "$DB_HOST" -u "$DB_USER" "$DB_NAME"'
```

There is **no forward-compat fallback**: rolling an image tag back past a
migration boundary needs the pre-migration snapshot taken just before that
migration ran (above), then the older tag pinned and restarted. Any data
added by that migration is dropped by the rollback — intrinsic to undoing
it, not a bug in the procedure.

### Scenario B — server is up but a feature is broken

No DB restore needed unless the release introduced a destructive migration
(then use Scenario A). Roll the image tag back:

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

A backup you've never restored is a hope, not a backup — run
`./tools/restore-drill.sh` monthly and before upgrades to prove the newest one
actually restores, in a disposable container that never touches your live DB
(see `docs/RESTORE-DRILL.md`).

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
make backup-list   # 1. pick chronicle_manifest_<TS>.txt, most recent you trust

# 2. Restore over a running server is never safe — the script refuses
# if /healthz answers.
docker compose stop chronicle

# 3. RESTORE_ARGS is required; type "RESTORE" at the prompt.
make restore RESTORE_ARGS="--manifest=/app/data/backups/chronicle_manifest_<TS>.txt"

# 4. RunStartupHealthChecks validates the schema; an incompatible image
# refuses to boot and says so in the logs.
docker compose start chronicle
docker compose logs -f chronicle
```

### Verifying a 0.0.2+ restore

After restore, confirm the player-character claim data round-tripped:
pre-restore claims must reappear post-restore. If they don't, either the
dump predates the claim (expected) or it's below schema version 22 (the
binary would have refused to boot already):

```sh
docker compose exec -T chronicle-db sh -c \
  'MYSQL_PWD="$MARIADB_PASSWORD" mysql -u "$MARIADB_USER" "$MARIADB_DATABASE" \
     -e "SELECT COUNT(*) AS claimed_characters FROM entities WHERE owner_user_id IS NOT NULL;"'
```

End-to-end: log in as a player whose owned character predates the backup,
open `My Characters` (`GET /campaigns/:id/me`), and confirm the character
card appears. A missing card for a claim made on 0.0.2 means the claim
postdates the dump — not a restore bug.

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

Source: `internal/database/healthcheck.go` `checkMigrationVersion`. The image
is too new for the DB or the DB is too new for the image. Just upgraded and
DB older: expected, wait for migrations to finish (>30s hung → check `docker
compose logs chronicle-db`). DB newer than code (rolled-back image): restore
the matching pre-migration backup (§7 Scenario A).

### `<K> critical column(s) missing`

Source: `internal/database/healthcheck.go` `checkCriticalColumns`. Migrations
haven't run, or a column was dropped manually. Run `make migrate-up` from a
host shell; if the migration itself is failing, check `docker compose logs
chronicle` for the error and roll back per §7.

### `pre-migration backup skipped: mysqldump not found`

Source: `internal/database/healthcheck.go` `PreMigrationBackup`. The image is
missing `mariadb-client`. Pull a current image (`docker compose pull
chronicle`), or rebuild from source via the override —
`docker compose -f docker-compose.yml -f docker-compose.build.yml build --no-cache chronicle`.
Until you do, upgrades have no automatic safety net.

### `pre-migration backup failed (non-fatal)`

Same source. The backup attempted but failed — usually `BACKUP_DIR` isn't
writable or the disk is full. Check `docker compose exec chronicle df -h
/app/data` and the directory's ownership. Boot continues without a backup;
**fix this before the next migration window.**

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
