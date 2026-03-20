-- Add foundry_id for stable Foundry VTT ID mapping on markers.
-- Matches the pattern used by map_drawings and map_tokens.
ALTER TABLE map_markers
    ADD COLUMN foundry_id VARCHAR(255) DEFAULT NULL AFTER created_by;
