-- Revert media-gallery addon to planned/widget state.
UPDATE addons
SET status      = 'planned',
    description = 'Advanced media management with albums, tagging, and lightbox',
    category    = 'widget'
WHERE slug = 'media-gallery';
