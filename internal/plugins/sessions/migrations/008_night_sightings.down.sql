-- Revert 008_night_sightings. Drops every recorded sighting and check-in
-- answer; nothing else holds them.
DROP TABLE IF EXISTS session_night_sightings;
