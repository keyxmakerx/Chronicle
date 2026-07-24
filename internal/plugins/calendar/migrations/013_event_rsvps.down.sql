-- Reverse C-CAL-RSVP-P1: drop the calendar-event RSVP tables and the per-event
-- collect_rsvps opt-in column. The token/response FKs are ON DELETE CASCADE, so
-- dropping the tables is sufficient; the referenced calendar_events/users tables
-- are untouched. DROP ... IF EXISTS keeps the rollback idempotent.
ALTER TABLE calendar_events DROP COLUMN IF EXISTS collect_rsvps;
DROP TABLE IF EXISTS calendar_event_rsvp_tokens;
DROP TABLE IF EXISTS calendar_event_rsvps;
