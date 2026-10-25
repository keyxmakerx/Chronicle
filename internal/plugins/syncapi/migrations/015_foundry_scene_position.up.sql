-- Scenes keep the order Foundry's Scenes sidebar shows them in (the owner's
-- folders and their sorting), so Chronicle lists them the same way rather
-- than A to Z. Rows stored before this read in id order until Foundry next
-- reports, which it does on connect.
ALTER TABLE foundry_scenes ADD COLUMN IF NOT EXISTS position INT NOT NULL DEFAULT 0 AFTER in_navigation;
ALTER TABLE foundry_scenes DROP INDEX IF EXISTS idx_foundry_scenes_name;
CREATE INDEX IF NOT EXISTS idx_foundry_scenes_position ON foundry_scenes (campaign_id, position);
