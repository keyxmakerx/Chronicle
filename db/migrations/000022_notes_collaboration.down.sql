-- 000022_notes_collaboration.down.sql

DROP TABLE IF EXISTS note_versions;

DROP INDEX idx_notes_locked ON notes;
DROP INDEX idx_notes_shared ON notes;

ALTER TABLE notes
  DROP COLUMN is_shared,
  DROP COLUMN last_edited_by,
  DROP COLUMN locked_by,
  DROP COLUMN locked_at,
  DROP COLUMN entry,
  DROP COLUMN entry_html;
