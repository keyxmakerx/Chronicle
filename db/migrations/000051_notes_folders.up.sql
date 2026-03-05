-- Add folder support to notes: parent_id for nesting, is_folder to distinguish
-- folder containers from leaf notes. Folders can contain other folders and notes.
ALTER TABLE notes
    ADD COLUMN parent_id CHAR(36) NULL DEFAULT NULL AFTER entity_id,
    ADD COLUMN is_folder BOOLEAN NOT NULL DEFAULT FALSE AFTER parent_id,
    ADD INDEX idx_notes_parent (parent_id),
    ADD CONSTRAINT fk_notes_parent FOREIGN KEY (parent_id) REFERENCES notes(id) ON DELETE CASCADE;
