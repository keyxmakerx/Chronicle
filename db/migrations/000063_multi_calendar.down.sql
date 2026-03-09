-- Revert Multi-Calendar Support: restore single-calendar-per-campaign constraint.

-- 1. Drop sessions calendar FK and column.
ALTER TABLE sessions DROP FOREIGN KEY fk_sessions_calendar;
ALTER TABLE sessions DROP COLUMN calendar_id;

-- 2. Drop sort_order and is_default columns.
ALTER TABLE calendars DROP COLUMN is_default;
ALTER TABLE calendars DROP COLUMN sort_order;

-- 3. Drop the regular index and restore the UNIQUE constraint.
ALTER TABLE calendars DROP INDEX idx_calendars_campaign;
ALTER TABLE calendars ADD UNIQUE KEY idx_calendars_campaign (campaign_id);
