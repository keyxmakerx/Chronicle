-- Reverse 000032. Narrowing truncates any 36-character id saved since the
-- fix, so under strict mode this fails loudly instead of corrupting rows.
ALTER TABLE note_attachments DROP FOREIGN KEY IF EXISTS fk_note_attachments_note;

ALTER TABLE note_attachments
    MODIFY COLUMN id          CHAR(26) NOT NULL,
    MODIFY COLUMN note_id     CHAR(26) NOT NULL,
    MODIFY COLUMN campaign_id CHAR(26) NOT NULL;

ALTER TABLE note_attachments
    ADD CONSTRAINT fk_note_attachments_note
    FOREIGN KEY IF NOT EXISTS (note_id) REFERENCES notes(id) ON DELETE CASCADE;
