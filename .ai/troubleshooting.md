# Troubleshooting

Non-obvious problems and their fixes.

## Templ Files Not Updating

**Symptom:** A `.templ` change does not show in the browser.

**Cause:** `_templ.go` files are generated and are stale.

**Fix:** `make templ`, or run `make dev` (air watches `.templ`).

---

## HTMX Requests Returning Full Page

**Symptom:** Clicking an HTMX element replaces the whole page instead of just
the target element.

**Cause:** The handler does not check `middleware.IsHTMX(c)` before rendering.

**Fix:** Branch on it and render the fragment; pattern in `.ai/conventions.md`.

---

## MariaDB "parseTime" Error

**Symptom:** `sql: Scan error on column 'created_at', converting driver.Value
type []uint8 ("2026-01-15 10:30:00") to a time.Time`

**Cause:** The DSN lacks `parseTime=true`. Chronicle adds it when it builds the
DSN from `DB_HOST`/`DB_USER`/etc; a `DATABASE_URL` is used as-is.

**Fix:** Add it to `DATABASE_URL`:
```
user:pass@tcp(localhost:3306)/chronicle?parseTime=true
```

---

## Migration "dirty database" Error

**Symptom:** `make migrate-up` fails with "dirty database version N".

**Cause:** A migration partially applied.

**Fix:**
1. Find the dirty version: `SELECT * FROM schema_migrations;`
2. Repair the schema by hand to match what that migration intended. Do not edit
   an applied migration file; a correction is a new idempotent migration.
3. `migrate -path db/migrations -database "$DATABASE_URL" force N`
4. `make migrate-up`

---

## UUID Ordering in MariaDB

**Symptom:** Slow inserts or poor index use with CHAR(36) UUID primary keys.

**Cause:** Random v4 UUIDs fragment B-tree indexes.

**Fix:** Time-ordered v7 (`uuid.Must(uuid.NewV7())`) has better locality.

---

## Redis Connection Refused in Dev

**Symptom:** App fails to start with "redis: connection refused".

**Cause:** Redis is not running.

**Fix:** `make docker-up` (MariaDB + Redis).

---

## "There is no database here" (there usually is)

**Symptom:** `make docker-up` fails because there is no Docker daemon, and
integration tests silently SKIP.

**Cause:** MariaDB runs fine without Docker via `mariadbd`. Two things make it
look broken:

- `mariadbd` refuses to start as root without `--user=root`; the error ("how to
  run mysqld as root") reads like a permissions wall.
- A Unix socket path over about 107 characters fails with a truncated path, so
  a socket in a long scratch directory looks like the server never started.

**Fix:** `make test-db-up` (or `tools/start-test-db.sh`) starts a disposable
server on port 13306, then `make test-int-local`. Integration tests SKIP, not
fail, when no server answers: a green run without a database proves nothing
about them.

---

## A plugin's CSS or JS "isn't in the container"

**Symptom:** `grep` or `ls` under `/app/static` finds no trace of a plugin's
front-end asset, which looks like missing code.

**Cause:** Front-end files are served from the on-disk static root
(`/app/static`) or, for some plugins, `//go:embed`ded into the binary, where no
shell command sees them.

**Fix:** An empty grep of `/app/static` is expected for embedded assets. Check
the plugin's `embed.go` or run the `host.embedded` admin diagnostic
(`docs/operator-diagnostics.md`).
