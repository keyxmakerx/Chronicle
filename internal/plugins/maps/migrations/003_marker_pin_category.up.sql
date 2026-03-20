-- Add pin_category for Foundry VTT pin type mapping.
-- Valid categories: location, danger, treasure, quest, note.
ALTER TABLE map_markers
    ADD COLUMN pin_category VARCHAR(50) DEFAULT NULL AFTER color;
