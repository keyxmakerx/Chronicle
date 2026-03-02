DROP TABLE IF EXISTS timeline_events;

-- Restore calendar_id as NOT NULL with original FK constraint.
-- Timelines without a calendar must be deleted first (or assigned one).
DELETE FROM timelines WHERE calendar_id IS NULL;
ALTER TABLE timelines DROP FOREIGN KEY fk_timelines_calendar;
ALTER TABLE timelines MODIFY calendar_id VARCHAR(36) NOT NULL;
ALTER TABLE timelines ADD CONSTRAINT fk_timelines_calendar
    FOREIGN KEY (calendar_id) REFERENCES calendars(id);
