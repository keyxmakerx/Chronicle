-- Rename the floating notebook addon from "player-notes" to "notes".
-- "player-notes" will be re-added as a separate addon for entity-page
-- collaborative notes (a future template-editor block).
UPDATE addons
SET slug = 'notes',
    name = 'Notes',
    description = 'Floating notebook panel (bottom-right) for personal and shared campaign notes. Includes checklists, color coding, version history, and edit locking.',
    icon = 'fa-book'
WHERE slug = 'player-notes';

-- Re-add "player-notes" as a planned addon for entity-page collaborative notes.
INSERT INTO addons (slug, name, description, version, category, status, icon, author)
VALUES ('player-notes', 'Player Notes', 'Collaborative note-taking block for entity pages. Players can write real-time notes about specific entities, visible to all campaign members.', '0.1.0', 'widget', 'planned', 'fa-sticky-note', 'Chronicle');
