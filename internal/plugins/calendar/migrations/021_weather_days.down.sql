-- 021_weather_days (down) — drop the per-day weather table. Destroys every
-- stored day reading; idempotent.
DROP TABLE IF EXISTS calendar_weather_days;
