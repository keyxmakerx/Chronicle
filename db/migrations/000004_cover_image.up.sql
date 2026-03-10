-- Add cover_image_path column to entities for the cover image layout block.
-- Stored separately from image_path (profile thumbnail) to allow both.
ALTER TABLE entities ADD COLUMN cover_image_path VARCHAR(500) DEFAULT NULL AFTER image_path;
