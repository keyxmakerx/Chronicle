-- Pictures placed on a map are drawings of type "image". The picture's media
-- file, its edge crop and its stacking position live beside the existing
-- geometry columns (points, rotation, fill_alpha as opacity, visibility).
-- image_id has no foreign key: the service checks the file belongs to the
-- map's campaign on every write, and a file removed later must leave the
-- drawing in place (the viewer shows a gap) rather than be rewritten.
ALTER TABLE map_drawings
    ADD COLUMN IF NOT EXISTS image_id CHAR(36) NULL AFTER foundry_id,
    ADD COLUMN IF NOT EXISTS crop JSON NULL AFTER image_id,
    ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0 AFTER crop;
