-- Remove maps addon registration.
DELETE FROM addons WHERE slug = 'maps';

-- Drop maps tables in dependency order.
DROP TABLE IF EXISTS map_markers;
DROP TABLE IF EXISTS maps;
