-- 001_page_tests: the Tests page block. A test is a long effort the party
-- makes on a page (a montage, a chase) and the result it ended in.
--
-- name and shown are columns because the encounter builder lists tests by
-- name and players are sent shown tests only; the results, the tally and the
-- recorded result are one JSON document validated by the service. version
-- guards concurrent edits, which matter here because the DM Screen's tally
-- and the page can both write during play. Foreign keys point at core tables
-- only, which migrate before any plugin, and the collation matches them;
-- deleting a page deletes its tests.
CREATE TABLE IF NOT EXISTS page_tests (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL,
    name         VARCHAR(120) NOT NULL,
    shown        TINYINT(1)   NOT NULL DEFAULT 0,
    data         LONGTEXT     NOT NULL,
    version      INT          NOT NULL DEFAULT 1,
    created_by   CHAR(36)     NULL,
    updated_by   CHAR(36)     NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_page_tests_page (entity_id, created_at),
    KEY idx_page_tests_campaign (campaign_id, updated_at),
    CONSTRAINT fk_page_tests_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_page_tests_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
