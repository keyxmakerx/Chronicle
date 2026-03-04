ALTER TABLE tags ADD COLUMN dm_only BOOLEAN NOT NULL DEFAULT FALSE AFTER color;

-- Index to support filtered queries: list visible tags for players.
CREATE INDEX idx_tags_campaign_visible ON tags (campaign_id, dm_only);
