-- C-REAL-CALENDAR-P1: real-time (wall-clock) mode for `reallife` calendars.
-- Cites: 2026-05-21-core-tenets §T-B1; 2026-07-11-r13-rulings RC-1, RC-2.
--
-- Two owner-set columns on the plugin's own `calendars` table (migration 001),
-- so this plugin-scoped migration (runs after core) references only a plugin
-- table — safe on a fresh DB.
--
--   real_time_zone   — IANA anchor zone (e.g. "America/New_York"). NULL = not
--                      set. Validated at enable time via time.LoadLocation
--                      (P2 owns the enable flow; RC-2 makes the zone REQUIRED
--                      at enable — no silent default).
--   tracks_real_time — opt-in flag. 0 = today's behavior (stored-not-computed;
--                      manual advance/set allowed). 1 = the loader computes the
--                      current date from the wall clock in real_time_zone and
--                      the date-writers reject manual changes (B-R10: never
--                      silently rewrite a live campaign's date).
--
-- Schema-only, idempotent, append-only. DEFAULT 0 / NULL backfills every
-- existing calendar as NOT real-time, so nothing changes on upgrade (stop-and-
-- flag #2: provably zero-change when tracks_real_time=0). Flipping a calendar
-- to real-time is an owner action (P2), never a data migration.
ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS real_time_zone   VARCHAR(64) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS tracks_real_time TINYINT(1) NOT NULL DEFAULT 0;
