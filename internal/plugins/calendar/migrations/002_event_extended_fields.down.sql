ALTER TABLE calendar_events
  DROP COLUMN color,
  DROP COLUMN icon,
  DROP COLUMN all_day,
  DROP COLUMN recurrence_interval,
  DROP COLUMN recurrence_end_year,
  DROP COLUMN recurrence_end_month,
  DROP COLUMN recurrence_end_day,
  DROP COLUMN recurrence_max_occurrences;

ALTER TABLE calendar_weekdays
  DROP COLUMN is_rest_day;
