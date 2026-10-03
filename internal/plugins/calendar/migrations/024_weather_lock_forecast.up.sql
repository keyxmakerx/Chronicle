-- 024_weather_lock_forecast — a lock on a day's weather, and how far ahead
-- players are shown a forecast.
--
-- calendar_weather_days.locked: a locked day keeps its reading when weather
-- is generated over it, the same protection a painted (manual) day already
-- has, but without being a different reading. A painted write clears it,
-- since painting is the owner saying what the day is.
--
-- calendar_weather_settings.forecast_days: how many days ahead of today a
-- player's forecast reaches (the service keeps it within 1..10). Whether
-- players see a forecast at all is calendars.forecasts_enabled, which
-- already exists.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE calendar_weather_days
    ADD COLUMN IF NOT EXISTS locked BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE calendar_weather_settings
    ADD COLUMN IF NOT EXISTS forecast_days INT NOT NULL DEFAULT 5;
