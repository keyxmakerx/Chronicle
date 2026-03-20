-- Add player_notes fields for player-facing content separate from the main entry.
-- Used by Foundry VTT sync to populate a player-visible journal page.
ALTER TABLE entities
    ADD COLUMN player_notes JSON DEFAULT NULL AFTER entry_html,
    ADD COLUMN player_notes_html TEXT DEFAULT NULL AFTER player_notes;
