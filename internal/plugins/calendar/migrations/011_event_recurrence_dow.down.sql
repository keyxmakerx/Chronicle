-- Revert C-CAL-EDITOR-EXPANSION PR2 recurrence_day_of_week.
ALTER TABLE calendar_events
  DROP COLUMN recurrence_day_of_week;
