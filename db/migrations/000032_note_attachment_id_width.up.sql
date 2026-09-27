-- note_attachments stored its ids in CHAR(26), but a note id and a campaign
-- id are 36-character UUIDs and the service mints 36-character attachment
-- ids, so every attachment insert failed ("Data too long" in strict mode, a
-- dangling truncated note_id otherwise). Widen the three columns to match
-- the tables they point at.
--
-- MariaDB will not retype a column that a foreign key uses, so the key is
-- dropped and re-created around the change; each step is idempotent. No
-- insert could ever succeed before this, so there are no rows for the
-- re-created key to reject.

ALTER TABLE note_attachments DROP FOREIGN KEY IF EXISTS fk_note_attachments_note;

ALTER TABLE note_attachments
    MODIFY COLUMN id          CHAR(36) NOT NULL,
    MODIFY COLUMN note_id     CHAR(36) NOT NULL,
    MODIFY COLUMN campaign_id CHAR(36) NOT NULL;

ALTER TABLE note_attachments
    ADD CONSTRAINT fk_note_attachments_note
    FOREIGN KEY IF NOT EXISTS (note_id) REFERENCES notes(id) ON DELETE CASCADE;
