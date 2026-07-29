-- Revert C-CALV4-RSVP-P8B's schedule-ask send bookkeeping.
--
-- Dropping the table drops both persisted limits with it, so a rolled-back
-- instance is back to "nobody has been asked yet" — which is the honest reading
-- of an absent log and is the safe direction (a send becomes possible again; no
-- send becomes impossible).
DROP TABLE IF EXISTS calendar_schedule_asks;
