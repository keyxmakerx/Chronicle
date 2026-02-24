-- Remove the planned player-notes addon and rename notes back.
DELETE FROM addons WHERE slug = 'player-notes';
UPDATE addons
SET slug = 'player-notes',
    name = 'Player Notes',
    description = 'Personal note-taking blocks for players on entity pages and standalone pages',
    icon = 'fa-sticky-note'
WHERE slug = 'notes';
