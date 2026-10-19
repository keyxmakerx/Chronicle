ALTER TABLE tracker_kinds DROP INDEX IF EXISTS uq_tracker_kinds_owner;
ALTER TABLE tracker_kinds DROP COLUMN IF EXISTS owner;
