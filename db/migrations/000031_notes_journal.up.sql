-- Journal notes: three nullable/defaulted columns, schema only.
--
-- shared_with_gm: the note is readable by the campaign's GMs (Owner and
--   co-DMs) as well as its owner. The other visibility states keep living in
--   is_shared (the whole party) and shared_with (named people), so every
--   existing row keeps exactly the audience it has today; FALSE is the only
--   value an existing row can have.
-- archived_at: set while a note is archived; NULL means active, which is
--   what every existing note is.
-- linked_note_id: a page note (a "jot") that was sent to the Journal points
--   at the Journal note it became. No FK: a dangling link reads as "not
--   linked" and is re-checked against visibility on every read.

ALTER TABLE notes ADD COLUMN IF NOT EXISTS shared_with_gm BOOLEAN NOT NULL DEFAULT FALSE AFTER is_shared;
ALTER TABLE notes ADD COLUMN IF NOT EXISTS archived_at DATETIME NULL DEFAULT NULL AFTER pinned;
ALTER TABLE notes ADD COLUMN IF NOT EXISTS linked_note_id CHAR(36) NULL DEFAULT NULL AFTER entity_id;
