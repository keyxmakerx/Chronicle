-- 030_sun_look — the kind of star a calendar's sun is.
--
-- calendars.sun_kind: 'red', 'orange', 'yellow', 'white' or 'blue'. The sky
-- colours the sun, its daylight and its glow from it; 'yellow' is the sun
-- every calendar had before. Validated in Go.
--
-- calendars.sun_color: an exact #rrggbb colour that overrides the kind's own,
-- or NULL to use the kind's.
--
-- A moon's colour strength needs no column: calendar_moons.tint already holds
-- it as a number from 0 to 1.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendars
  ADD COLUMN IF NOT EXISTS sun_kind  VARCHAR(8) NOT NULL DEFAULT 'yellow',
  ADD COLUMN IF NOT EXISTS sun_color VARCHAR(7) DEFAULT NULL;
