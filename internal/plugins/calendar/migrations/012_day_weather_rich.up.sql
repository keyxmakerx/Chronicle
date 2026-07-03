-- C-CAL-PARITY seam (cordinator#53 / W0 audit): make calendar_day_weather the
-- ONE canonical weather store by adding the rich Calendaria-shaped fields the
-- legacy single-row calendar_weather carried. weather_type doubles as the
-- Calendaria preset id (the W1 vocabulary-parity wave made the two id sets
-- 1:1), so no separate preset_id column is added.
--
-- Column names/types mirror calendar_weather (migration 003) exactly so the
-- legacy row projects into a day row losslessly. zone_id/zone_name are
-- deliberately NOT added: per the W0 audit's refinement 2, per-zone weather is
-- W2 scope — the active-zone pointer stays on calendar_weather until then.
--
-- All columns nullable + IF NOT EXISTS: additive and idempotent per the
-- Migration Safety Rules (CLAUDE.md / .ai/conventions.md). Existing rows keep
-- rendering (weather_type untouched); NULL rich fields mean "unset".
ALTER TABLE calendar_day_weather
    ADD COLUMN IF NOT EXISTS preset_label            VARCHAR(100) DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS icon                    VARCHAR(50)  DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS color                   VARCHAR(20)  DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS temperature_celsius     FLOAT        DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS wind_speed_kph          FLOAT        DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS wind_speed_tier         VARCHAR(20)  DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS wind_direction          VARCHAR(5)   DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS wind_direction_degrees  INT          DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS precipitation_type      VARCHAR(20)  DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS precipitation_intensity FLOAT        DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS description             TEXT         DEFAULT NULL;
