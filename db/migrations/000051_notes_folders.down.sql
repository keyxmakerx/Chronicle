ALTER TABLE notes
    DROP FOREIGN KEY fk_notes_parent,
    DROP INDEX idx_notes_parent,
    DROP COLUMN is_folder,
    DROP COLUMN parent_id;
