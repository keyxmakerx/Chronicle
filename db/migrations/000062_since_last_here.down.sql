-- Reverse 000062. Members' "seen up to" times are lost; the block falls back
-- to the join date until the column is added again.
DROP INDEX IF EXISTS idx_entities_campaign_created ON entities;
ALTER TABLE campaign_members DROP COLUMN IF EXISTS home_seen_at;
