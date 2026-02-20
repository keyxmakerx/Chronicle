-- Revert addon status fixes back to original seed values.
UPDATE addons SET status = 'planned' WHERE slug = 'sync-api';
UPDATE addons SET status = 'active' WHERE slug IN ('dnd5e', 'pathfinder2e', 'drawsteel');
UPDATE addons SET status = 'active' WHERE slug = 'dice-roller';
UPDATE addons SET status = 'active' WHERE slug = 'media-gallery';
