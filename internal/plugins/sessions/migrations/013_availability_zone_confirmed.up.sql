-- member_availability_status.zone_confirmed_at records that a member answered
-- the one-time "which time zone are your hours in?" question, whichever way
-- they answered, so it is never asked again. NULL for everyone until then;
-- the question itself only appears when the zone saved with their hours
-- differs from their account's (or browser's), so most members never see it.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.

ALTER TABLE member_availability_status
  ADD COLUMN IF NOT EXISTS zone_confirmed_at DATETIME NULL DEFAULT NULL;
