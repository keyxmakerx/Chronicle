-- Both are display aids for the call flow; dropping them loses no
-- source-of-truth data.
DROP INDEX IF EXISTS idx_sync_events_time ON sync_events;
ALTER TABLE sync_events DROP COLUMN IF EXISTS was_value;
