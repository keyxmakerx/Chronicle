-- The feed is derived from live events; dropping it only forces clients to
-- do one full resync.
DROP TABLE IF EXISTS sync_change_watermarks;
DROP TABLE IF EXISTS sync_changes;
