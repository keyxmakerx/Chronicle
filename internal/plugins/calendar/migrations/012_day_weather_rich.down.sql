-- Rollback of the day-weather rich columns (cordinator#53 seam). Drops only
-- the migration-012 additions; the migration-008 base columns are untouched.
ALTER TABLE calendar_day_weather
    DROP COLUMN IF EXISTS preset_label,
    DROP COLUMN IF EXISTS icon,
    DROP COLUMN IF EXISTS color,
    DROP COLUMN IF EXISTS temperature_celsius,
    DROP COLUMN IF EXISTS wind_speed_kph,
    DROP COLUMN IF EXISTS wind_speed_tier,
    DROP COLUMN IF EXISTS wind_direction,
    DROP COLUMN IF EXISTS wind_direction_degrees,
    DROP COLUMN IF EXISTS precipitation_type,
    DROP COLUMN IF EXISTS precipitation_intensity,
    DROP COLUMN IF EXISTS description;
