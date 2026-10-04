ALTER TABLE map_drawings
    DROP COLUMN IF EXISTS sort_order,
    DROP COLUMN IF EXISTS crop,
    DROP COLUMN IF EXISTS image_id;
