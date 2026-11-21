-- Revert 012_event_night_link. Every night-to-event link and every "Not this
-- one" is lost; the nights, the events and their pages stay. Nights made from
-- an event are then matched to it by name or day again, and the strip offers
-- events whose "Not this one" was dropped.
DROP TABLE IF EXISTS session_event_dismissals;
ALTER TABLE sessions DROP FOREIGN KEY IF EXISTS fk_sessions_calendar_event;
DROP INDEX IF EXISTS uq_sessions_calendar_event ON sessions;
ALTER TABLE sessions DROP COLUMN IF EXISTS calendar_event_id;
