-- Reverse C-SWEEP-R4 stage 25: put calendar_active back on the destructive
-- cascade.
--
-- THIS DOWN IS LOSSY IN A WAY THE UP IS NOT, and that asymmetry is the point of
-- the migration. Restoring ON DELETE CASCADE means the next calendar deletion
-- again destroys every viewer's sidebar pin, Block layer set and Bench sections
-- for the whole campaign.
--
-- It also cannot run while any row holds a NULL calendar_id: NOT NULL rejects
-- them. Rows are NULLed only by a delete that this migration was written to
-- survive, so a campaign that has deleted a calendar since the upgrade must
-- have those pointers re-seated (or the rows removed) before rolling back.
-- That is deliberately NOT done here — a down migration must not decide on an
-- operator's behalf which calendar a viewer meant.

ALTER TABLE calendar_active
  DROP FOREIGN KEY IF EXISTS fk_calendar_active_cal;

ALTER TABLE calendar_active
  MODIFY COLUMN calendar_id VARCHAR(36) NOT NULL;

ALTER TABLE calendar_active
  ADD CONSTRAINT fk_calendar_active_cal
    FOREIGN KEY (calendar_id) REFERENCES calendars(id) ON DELETE CASCADE;
