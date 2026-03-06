-- Add character assignment to campaign members.
-- Links a member to their character entity in the campaign.
ALTER TABLE campaign_members ADD COLUMN character_entity_id VARCHAR(36) DEFAULT NULL;
ALTER TABLE campaign_members ADD CONSTRAINT fk_cm_character_entity
  FOREIGN KEY (character_entity_id) REFERENCES entities(id) ON DELETE SET NULL;
