-- 030_sun_look (down) — drop the sun's kind and colour. Every sun then shows
-- as the ordinary yellow one. Idempotent.
ALTER TABLE calendars
  DROP COLUMN IF EXISTS sun_color,
  DROP COLUMN IF EXISTS sun_kind;
