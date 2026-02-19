# Troubleshooting

<!-- ====================================================================== -->
<!-- Category: Semi-static                                                    -->
<!-- Purpose: Known gotchas and their solutions. Prevents re-debugging the    -->
<!--          same non-obvious issues across sessions.                        -->
<!-- Update: Whenever a non-obvious bug is encountered and solved.            -->
<!-- ====================================================================== -->

> This file will grow as the project progresses. Add entries whenever you
> encounter and solve a non-obvious issue.

---

## Templ Files Not Updating

**Symptom:** Changed a `.templ` file but browser shows old content.

**Cause:** Templ generates `_templ.go` files that must be regenerated.

**Fix:** Run `make templ` or ensure `air` is configured to watch `.templ` files.

---

## HTMX Requests Returning Full Page

**Symptom:** Clicking an HTMX element replaces the whole page instead of just
the target element.

**Cause:** Handler not checking the `HX-Request` header.

**Fix:** Add `isHTMX(c)` check in handler before rendering. See
`.ai/conventions.md` for the pattern.

---

## MariaDB "parseTime" Error

**Symptom:** `sql: Scan error on column 'created_at', converting driver.Value
type []uint8 ("2026-01-15 10:30:00") to a time.Time`

**Cause:** Missing `parseTime=true` in the MariaDB DSN.

**Fix:** Ensure DATABASE_URL includes `?parseTime=true`:
```
user:pass@tcp(localhost:3306)/chronicle?parseTime=true
```

---

## Migration "dirty database" Error

**Symptom:** `make migrate-up` fails with "dirty database version N".

**Cause:** A previous migration partially applied.

**Fix:**
1. Check what version is dirty: `SELECT * FROM schema_migrations;`
2. Fix the migration SQL
3. Force version: `migrate -path db/migrations -database "$DATABASE_URL" force N`
4. Re-run: `make migrate-up`

---

## UUID Ordering in MariaDB

**Symptom:** Queries with `ORDER BY created_at` are slow or indexes not used
well with CHAR(36) UUID primary keys.

**Cause:** Random UUIDs (v4) cause index fragmentation in B-trees.

**Fix:** Consider UUID v7 (time-ordered) for better index locality. The
`google/uuid` package supports this with `uuid.Must(uuid.NewV7())`. Decision
to switch should be an ADR.

---

## Redis Connection Refused in Dev

**Symptom:** App fails to start with "redis: connection refused".

**Cause:** Redis container not running.

**Fix:** `make docker-up` to start MariaDB + Redis containers.
