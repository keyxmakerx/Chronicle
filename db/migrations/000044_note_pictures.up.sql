-- Which notes hold which note pictures, as written by the picture's uploader.
-- A note picture (media_files.usage_type 'note_image') is opened only by
-- someone who can read a note bound to it here, so the right to open it comes
-- from where its uploader put it, never from an id someone else pastes into
-- a note of their own. A row is written only when the uploader saves the note
-- with the picture in its text, and goes when a save no longer contains it.
-- Both parents are core tables, so this is a core migration; deleting either
-- the note or the file removes the row.

CREATE TABLE IF NOT EXISTS note_pictures (
    media_id   CHAR(36)  NOT NULL,
    note_id    CHAR(36)  NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (media_id, note_id),
    INDEX idx_note_pictures_note (note_id),

    CONSTRAINT fk_note_pictures_media FOREIGN KEY (media_id) REFERENCES media_files(id) ON DELETE CASCADE,
    CONSTRAINT fk_note_pictures_note  FOREIGN KEY (note_id)  REFERENCES notes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
