-- Reverse C-CAL-WEATHER-ZONES: drop the calendar_weather_zones table.
-- The active-zone reference (calendar_weather.zone_id + zone_name)
-- was added in migration 003 and stays untouched.

DROP TABLE IF EXISTS calendar_weather_zones;
