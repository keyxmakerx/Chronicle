-- 000022_notes_collaboration.up.sql
-- Sprint 4: Player Notes Overhaul
--
-- Adds collaboration columns to notes (shared flag, edit locking, rich text)
-- and creates note_versions table for version history.

-- Collaboration and rich text columns on existing notes table.
ALTER TABLE notes
  ADD COLUMN is_shared      BOOLEAN   NOT NULL DEFAULT FALSE AFTER pinned,
  ADD COLUMN last_edited_by CHAR(36)  DEFAULT NULL           AFTER is_shared,
  ADD COLUMN locked_by      CHAR(36)  DEFAULT NULL           AFTER last_edited_by,
  ADD COLUMN locked_at      DATETIME  DEFAULT NULL           AFTER locked_by,
  ADD COLUMN entry          JSON      DEFAULT NULL           AFTER content,
  ADD COLUMN entry_html     TEXT      DEFAULT NULL           AFTER entry;

-- Index for stale lock cleanup queries.
CREATE INDEX idx_notes_locked ON notes(locked_by, locked_at);

-- Index for listing shared notes within a campaign.
CREATE INDEX idx_notes_shared ON notes(campaign_id, is_shared);

-- Version history: snapshot of note content at each save.
CREATE TABLE note_versions (
    id         CHAR(36)  NOT NULL PRIMARY KEY,
    note_id    CHAR(36)  NOT NULL,
    user_id    CHAR(36)  NOT NULL,
    title      VARCHAR(200) NOT NULL DEFAULT '',
    content    JSON      NOT NULL,
    entry      JSON      DEFAULT NULL,
    entry_html TEXT      DEFAULT NULL,
    created_at DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    INDEX idx_note_versions_note (note_id, created_at DESC),
    CONSTRAINT fk_note_versions_note
        FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
