-- Reverse migration 000042: remove map expansion tables and columns.

DROP TABLE IF EXISTS map_fog;
DROP TABLE IF EXISTS map_tokens;
DROP TABLE IF EXISTS map_drawings;
DROP TABLE IF EXISTS map_layers;

ALTER TABLE maps
    DROP COLUMN IF EXISTS grid_type,
    DROP COLUMN IF EXISTS grid_size,
    DROP COLUMN IF EXISTS grid_color,
    DROP COLUMN IF EXISTS grid_opacity,
    DROP COLUMN IF EXISTS background_color,
    DROP COLUMN IF EXISTS fog_exploration,
    DROP COLUMN IF EXISTS initial_view_x,
    DROP COLUMN IF EXISTS initial_view_y,
    DROP COLUMN IF EXISTS initial_zoom,
    DROP COLUMN IF EXISTS foundry_scene_id;
