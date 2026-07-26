-- Revert C-CAL-RSVP-P1 event RSVP storage.
-- Tokens first: they FK the rsvp table's event, and dropping in reverse
-- dependency order keeps the rollback clean on engines that check.
DROP TABLE IF EXISTS calendar_event_rsvp_tokens;
DROP TABLE IF EXISTS calendar_event_rsvps;

ALTER TABLE calendar_events
  DROP COLUMN IF EXISTS collect_rsvps;
