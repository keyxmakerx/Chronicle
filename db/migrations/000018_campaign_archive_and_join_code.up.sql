-- Add soft-archive timestamp to campaigns. NULL means active, non-NULL means archived.
ALTER TABLE campaigns ADD COLUMN archived_at DATETIME NULL DEFAULT NULL AFTER updated_at;
CREATE INDEX idx_campaigns_archived_at ON campaigns (archived_at);

-- Add join_code for shareable invite links. A short random code (12-char alphanumeric)
-- that allows anyone with the link to join. NULL means no active join link.
ALTER TABLE campaigns ADD COLUMN join_code VARCHAR(20) NULL DEFAULT NULL AFTER archived_at;
CREATE UNIQUE INDEX idx_campaigns_join_code ON campaigns (join_code);
