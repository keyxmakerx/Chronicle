-- Make media-gallery addon active and update its description to reflect
-- that it IS the existing media system (upload, browse, manage), with
-- future expansion planned for albums, tagging, and lightbox.
UPDATE addons
SET status      = 'active',
    description = 'Campaign media management — upload, browse, and organize images. Future: albums, tagging, and lightbox.',
    category    = 'plugin'
WHERE slug = 'media-gallery';
