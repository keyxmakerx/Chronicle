-- Per-user flag tracking: prevents one user from flagging the same publication multiple times.
-- The composite primary key (user_id, publication_id) enforces uniqueness at the DB level.
CREATE TABLE IF NOT EXISTS bestiary_flags (
    user_id        CHAR(36)  NOT NULL,
    publication_id CHAR(36)  NOT NULL,
    reason         TEXT      DEFAULT NULL,
    created_at     DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (user_id, publication_id),

    CONSTRAINT fk_bflag_publication FOREIGN KEY (publication_id)
        REFERENCES bestiary_publications(id) ON DELETE CASCADE,
    CONSTRAINT fk_bflag_user FOREIGN KEY (user_id)
        REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
