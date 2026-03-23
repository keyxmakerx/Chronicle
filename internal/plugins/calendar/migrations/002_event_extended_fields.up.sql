-- Add extended event fields for Calendaria feature parity:
-- color, icon, all_day flag, and enhanced recurrence support.
-- Also adds is_rest_day to weekdays for rest day designation.

ALTER TABLE calendar_events
  ADD COLUMN color VARCHAR(20) DEFAULT NULL,
  ADD COLUMN icon VARCHAR(50) DEFAULT NULL,
  ADD COLUMN all_day TINYINT(1) NOT NULL DEFAULT 1,
  ADD COLUMN recurrence_interval INT DEFAULT NULL,
  ADD COLUMN recurrence_end_year INT DEFAULT NULL,
  ADD COLUMN recurrence_end_month INT DEFAULT NULL,
  ADD COLUMN recurrence_end_day INT DEFAULT NULL,
  ADD COLUMN recurrence_max_occurrences INT DEFAULT NULL;

ALTER TABLE calendar_weekdays
  ADD COLUMN is_rest_day TINYINT(1) NOT NULL DEFAULT 0;
