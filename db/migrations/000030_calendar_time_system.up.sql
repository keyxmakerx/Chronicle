-- Calendar time system: add configurable time units (hours/minutes/seconds)
-- and current time tracking to calendars, plus optional event start/end times.
-- Fantasy calendars can customize time units; real-life mode uses 24/60/60.

-- Time configuration and current time on the calendar itself.
ALTER TABLE calendars
  ADD COLUMN hours_per_day      INT NOT NULL DEFAULT 24 AFTER current_day,
  ADD COLUMN minutes_per_hour   INT NOT NULL DEFAULT 60 AFTER hours_per_day,
  ADD COLUMN seconds_per_minute INT NOT NULL DEFAULT 60 AFTER minutes_per_hour,
  ADD COLUMN current_hour       INT NOT NULL DEFAULT 0  AFTER seconds_per_minute,
  ADD COLUMN current_minute     INT NOT NULL DEFAULT 0  AFTER current_hour;

-- Optional start/end time on events (NULL = all-day event).
ALTER TABLE calendar_events
  ADD COLUMN start_hour   INT DEFAULT NULL AFTER day,
  ADD COLUMN start_minute INT DEFAULT NULL AFTER start_hour,
  ADD COLUMN end_hour     INT DEFAULT NULL AFTER end_day,
  ADD COLUMN end_minute   INT DEFAULT NULL AFTER end_hour;
