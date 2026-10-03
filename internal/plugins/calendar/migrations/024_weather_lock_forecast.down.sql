-- Reverses 024_weather_lock_forecast. IF EXISTS so a second run is harmless.
ALTER TABLE calendar_weather_days DROP COLUMN IF EXISTS locked;
ALTER TABLE calendar_weather_settings DROP COLUMN IF EXISTS forecast_days;
