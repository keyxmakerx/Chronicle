ALTER TABLE foundry_scenes DROP INDEX IF EXISTS idx_foundry_scenes_position;
CREATE INDEX IF NOT EXISTS idx_foundry_scenes_name ON foundry_scenes (campaign_id, name);
ALTER TABLE foundry_scenes DROP COLUMN IF EXISTS position;
