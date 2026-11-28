-- 003_cutscene_moment_answers: what players answer while a showing holds on
-- a moment, and which moments the Director has let go.
--
-- released is a bit per moment of the showing's scene (bit i for moment i;
-- a scene has at most six), set when the Director lets that moment go, so
-- a screen that opens late lands on the right beat.
--
-- One answer per viewer per moment. status is waiting until the Director
-- adds it to the story (added), adds a changed line (changed) or turns it
-- down (declined); line is what joined the story and page_id where. text is
-- what the player sent and stays as they wrote it. drawn holds what was drawn
-- or rolled for the ways that draw or roll. page_id carries no foreign key:
-- deleting the page later must not take the record with it.
--
-- New tables and a column on this plugin's own table only.
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE cutscene_showings ADD COLUMN IF NOT EXISTS released INT NOT NULL DEFAULT 0 AFTER stopped_at;

CREATE TABLE IF NOT EXISTS cutscene_moment_answers (
    id          CHAR(36)     NOT NULL,
    showing_id  CHAR(36)     NOT NULL,
    moment      TINYINT      NOT NULL,
    user_id     CHAR(36)     NOT NULL,
    way         VARCHAR(16)  NOT NULL,
    `text`      TEXT         NOT NULL,
    drawn       TEXT         NULL,
    status      VARCHAR(16)  NOT NULL DEFAULT 'waiting',
    line        TEXT         NULL,
    page_id     CHAR(36)     NULL,
    created_at  DATETIME     NOT NULL,
    resolved_at DATETIME     NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_cutscene_answers_viewer (showing_id, moment, user_id),
    CONSTRAINT fk_cutscene_answers_showing FOREIGN KEY (showing_id) REFERENCES cutscene_showings(id) ON DELETE CASCADE,
    CONSTRAINT fk_cutscene_answers_user    FOREIGN KEY (user_id)    REFERENCES users(id)             ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
