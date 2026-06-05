-- Reverse C-CAL-WORLDSTATE-SERVER-MODEL: drop the worldstate tables and the
-- moon-render-param / mood-tint columns. Order: child tables and ALTERs
-- first is irrelevant (FKs are ON DELETE CASCADE and the ALTERs are
-- independent), but we drop in reverse creation order for clarity.

ALTER TABLE calendars
    DROP COLUMN mood_tint_color,
    DROP COLUMN mood_tint_intensity;

ALTER TABLE calendar_moons
    DROP COLUMN base_design,
    DROP COLUMN tint,
    DROP COLUMN phase_source,
    DROP COLUMN size,
    DROP COLUMN orbit_speed;

DROP TABLE IF EXISTS calendar_special_days;
DROP TABLE IF EXISTS calendar_moon_phases;
DROP TABLE IF EXISTS calendar_celestial_events;
DROP TABLE IF EXISTS calendar_day_weather;
