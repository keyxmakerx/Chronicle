-- 032_place_weather (down) — drop each place's weather and the party's
-- marks. The calendar goes back to one weather for the world; climates set
-- on pages stay as page fields and are simply not read. Idempotent.
DROP TABLE IF EXISTS calendar_party_places;
DROP TABLE IF EXISTS calendar_place_weather_days;
