DROP INDEX idx_campaigns_join_code ON campaigns;
ALTER TABLE campaigns DROP COLUMN join_code;
DROP INDEX idx_campaigns_archived_at ON campaigns;
ALTER TABLE campaigns DROP COLUMN archived_at;
