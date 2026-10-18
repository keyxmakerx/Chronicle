-- Which game-night bell items have gone out: the day-before reminder and the
-- after-the-night items. The row is claimed before the bell is written, so
-- several servers, or a restart, never send one twice.
CREATE TABLE IF NOT EXISTS session_night_bells (
    session_id      CHAR(36)    NOT NULL,
    occurrence_date DATE        NOT NULL,
    user_id         CHAR(36)    NOT NULL,
    kind            VARCHAR(16) NOT NULL,
    sent_at         DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (session_id, occurrence_date, user_id, kind),
    CONSTRAINT fk_session_night_bell_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_night_bell_user    FOREIGN KEY (user_id)    REFERENCES users(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
