-- Fix addon status mismatches: sync-api is fully built, game modules and
-- dice-roller/media-gallery are not yet implemented.

-- sync-api was built in Phase B — mark it active.
UPDATE addons SET status = 'active' WHERE slug = 'sync-api';

-- Game modules are "coming soon" — no content packs exist yet.
UPDATE addons SET status = 'planned' WHERE slug IN ('dnd5e', 'pathfinder2e', 'drawsteel');

-- dice-roller has no implementation — should be planned.
UPDATE addons SET status = 'planned' WHERE slug = 'dice-roller';

-- media-gallery has no dedicated gallery widget — should be planned.
UPDATE addons SET status = 'planned' WHERE slug = 'media-gallery';
