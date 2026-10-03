-- 023_weather_settings (down) — drop the per-calendar weather settings.
-- Destroys every stored climate choice; idempotent.
DROP TABLE IF EXISTS calendar_weather_settings;
