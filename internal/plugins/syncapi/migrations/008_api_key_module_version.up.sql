-- The Chronicle Sync module reports its own version in the
-- X-Chronicle-Module-Version header on every REST request; the middleware
-- stores the latest valid value here so the owner's "Apps & game system" page
-- can show which module version last talked to Chronicle. Nullable: keys
-- that have never carried the header (older modules) simply have no version.
ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS module_version VARCHAR(32) DEFAULT NULL AFTER device_bound_at;
