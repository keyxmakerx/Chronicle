-- 006_calv5_clear_availability — CALV5-PLACEHOLDER
--
-- Empties the scheduling answers as part of the CALV5 clean slate. The
-- operator ruled on 2026-08-21 that nothing is preserved, availability and
-- RSVP answers included: players re-enter their availability once V5 ships.
--
-- TABLES ARE KEPT AND ONLY EMPTIED. The sessions plugin is untouched by the
-- calendar demolition and still reads and writes all three; dropping them
-- would take a working feature down with a rebuild it has nothing to do with.
--
-- CONVENTION DEVIATION, STATED PLAINLY: CLAUDE.md says one-time DATA fixes do
-- not belong in migrations — use an idempotent reconciler. This is a one-time
-- data wipe and it is in a migration anyway, on the operator's explicit
-- instruction to do the clean slate through the startup migration runner. It
-- is defensible here because it is a single irreversible clean-slate event
-- rather than a rule the product must keep re-applying, and because a
-- reconciler that deleted player data every boot would be far more dangerous
-- than a migration that does it once. It is NOT a precedent.
--
-- TRUNCATE rather than DELETE: it resets AUTO_INCREMENT, so V5 does not
-- inherit id sequences from data nobody can see any more. Both are idempotent
-- against an already-empty table.
--
-- What is lost, so it is on the record rather than discovered later:
--   member_availability        — every painted free/busy block, including the
--                                week_parity tracks that made alternating
--                                weeks work for a part-time player
--   member_availability_status — the record that a member ANSWERED, which is
--                                the only thing separating "never replied"
--                                from "genuinely never free"
--   availability_exceptions    — one-off dates a member marked unavailable

TRUNCATE TABLE member_availability;
TRUNCATE TABLE member_availability_status;
TRUNCATE TABLE availability_exceptions;
