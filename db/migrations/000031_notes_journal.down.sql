-- Reverse 000031: drop the three Journal columns. No index or FK depends on them.
ALTER TABLE notes DROP COLUMN IF EXISTS linked_note_id;
ALTER TABLE notes DROP COLUMN IF EXISTS archived_at;
ALTER TABLE notes DROP COLUMN IF EXISTS shared_with_gm;
