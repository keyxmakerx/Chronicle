-- Each game night's Journal entries: one per member per night, made for them
-- when the night is due (see nightEntryDue). The note itself is an ordinary
-- Journal note; this row says which night it belongs to. note_id has no
-- foreign key on purpose: when the member deletes their entry the row stays,
-- so the entry is not made again.
CREATE TABLE IF NOT EXISTS session_night_entries (
    session_id      CHAR(36) NOT NULL,
    occurrence_date DATE     NOT NULL,
    user_id         CHAR(36) NOT NULL,
    note_id         CHAR(36) NOT NULL,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (session_id, occurrence_date, user_id),
    CONSTRAINT fk_session_night_entry_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_night_entry_user    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE,
    UNIQUE KEY uq_session_night_entry_note (note_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
