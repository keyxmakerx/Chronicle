-- C-CAL-EDITOR-EXPANSION PR2: recurrence beyond yearly.
-- Adds recurrence_day_of_week to calendar_events to mirror the sessions
-- plugin's recurrence model (weekly/biweekly/monthly/custom). DEFAULT NULL so
-- existing rows are untouched — they keep their current single-occurrence
-- behavior (only the four new recurrence_type values expand at projection time;
-- any legacy/empty type still renders once, exactly as before).

ALTER TABLE calendar_events
  ADD COLUMN recurrence_day_of_week INT DEFAULT NULL;
