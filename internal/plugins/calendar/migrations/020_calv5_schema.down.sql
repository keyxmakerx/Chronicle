-- 020_calv5_schema (down) — reverse the V5 schema back to 019's exit state.
--
-- Schema-only in both directions: 019 already emptied calendars/calendar_events
-- and every table this migration creates starts with zero rows, so there is no
-- data to preserve or lose here, unlike 019's own down (which really can't
-- undo a data wipe). Idempotent throughout.

-- Children before parents, so a re-run mid-drop never trips a live FK.
DROP TABLE IF EXISTS entity_era_links;
DROP TABLE IF EXISTS entity_event_links;
DROP TABLE IF EXISTS calendar_weather;
DROP TABLE IF EXISTS calendar_cycle_entries;
DROP TABLE IF EXISTS calendar_cycles;
DROP TABLE IF EXISTS calendar_festivals;
DROP TABLE IF EXISTS calendar_eras;
DROP TABLE IF EXISTS calendar_seasons;
DROP TABLE IF EXISTS calendar_moons;
DROP TABLE IF EXISTS calendar_weekdays;
DROP TABLE IF EXISTS calendar_months;

-- calendar_events: drop the FK + the columns this migration added, restore
-- the two it dropped so the table's shape matches what 019 left behind.
ALTER TABLE calendar_events
  DROP FOREIGN KEY IF EXISTS fk_calendar_events_kind;

ALTER TABLE calendar_events
  DROP INDEX IF EXISTS idx_calendar_events_kind,
  DROP COLUMN IF EXISTS kind_id,
  DROP COLUMN IF EXISTS announced,
  DROP COLUMN IF EXISTS payload,
  ADD COLUMN IF NOT EXISTS category VARCHAR(50) DEFAULT NULL,
  ADD COLUMN IF NOT EXISTS collect_rsvps TINYINT(1) NOT NULL DEFAULT 0;

-- calendar_event_kinds can now be dropped: nothing in calendar_events
-- references it any more.
DROP TABLE IF EXISTS calendar_event_kinds;

-- calendars: restore the mood-tint columns, drop the three new switches.
ALTER TABLE calendars
  DROP COLUMN IF EXISTS hemisphere,
  DROP COLUMN IF EXISTS forecasts_enabled,
  DROP COLUMN IF EXISTS month_starts_new_week,
  ADD COLUMN IF NOT EXISTS mood_tint_color VARCHAR(32) NULL,
  ADD COLUMN IF NOT EXISTS mood_tint_intensity FLOAT NULL;
