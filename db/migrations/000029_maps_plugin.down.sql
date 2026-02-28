-- Revert maps addon to its original seed values from migration 000015
-- (category='widget', status='planned') rather than deleting it.
UPDATE addons
SET category    = 'widget',
    status      = 'planned',
    description = 'Leaflet.js map viewer with entity pins and layer support'
WHERE slug = 'maps';

-- Drop maps tables in dependency order.
DROP TABLE IF EXISTS map_markers;
DROP TABLE IF EXISTS maps;
