-- Add description and pinned_entity_ids columns to entity_types for category
-- dashboard pages. Description holds rich text content shown at the top of
-- the category listing; pinned_entity_ids stores a JSON array of entity IDs
-- pinned to the top of the listing.
ALTER TABLE entity_types
    ADD COLUMN description TEXT DEFAULT NULL AFTER color,
    ADD COLUMN pinned_entity_ids JSON DEFAULT NULL AFTER description;
