-- Who the party saw on which game night. One row per night and page: the
-- night is the session plus its occurrence date (a one-off night uses its own
-- scheduled date), so a weekly series records each night separately, which
-- session_entities (one link per series) cannot. A "not_seen" row is kept so
-- a dismissed suggestion stays dismissed. Rows come from the after-night
-- check-in or are added by hand on a page's Last seen.
CREATE TABLE IF NOT EXISTS session_night_sightings (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    session_id      CHAR(36)    NOT NULL,
    occurrence_date DATE        NOT NULL,
    entity_id       CHAR(36)    NOT NULL,
    state           VARCHAR(10) NOT NULL,
    source          VARCHAR(10) NOT NULL,
    added_by        CHAR(36)    DEFAULT NULL,
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_session_sighting_session FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_sighting_entity  FOREIGN KEY (entity_id)  REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_session_sighting_user    FOREIGN KEY (added_by)   REFERENCES users(id)    ON DELETE SET NULL,
    UNIQUE KEY uq_session_night_sighting (session_id, occurrence_date, entity_id),
    INDEX idx_session_sighting_entity (entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
