-- 028_event_rolls (down) — drop event rolls and every stored night's result.
-- Destroys them; the events themselves stay. Idempotent.
DROP TABLE IF EXISTS calendar_event_roll_nights;
DROP TABLE IF EXISTS calendar_event_rolls;
