-- The call flow on the Sync history page shows what a change replaced, in
-- words (the calendar's date before a date push), next to what was asked
-- for. Empty when not known.
ALTER TABLE sync_events ADD COLUMN IF NOT EXISTS was_value VARCHAR(200) NOT NULL DEFAULT '' AFTER message;

-- The call flow reads a few minutes of one campaign's history at a time.
CREATE INDEX IF NOT EXISTS idx_sync_events_time ON sync_events (campaign_id, occurred_at);
