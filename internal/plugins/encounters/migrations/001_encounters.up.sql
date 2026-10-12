-- 001_encounters: fights a DM plans on a page and loads in Foundry.
--
-- name and ready are columns because the Foundry list sorts and searches on
-- them; the heroes, creature rows and notes are one JSON document validated
-- by the service, so the builder can grow without a schema change. version
-- guards concurrent edits. Foreign keys point at core tables only, which
-- migrate before any plugin, and the collation matches them.
CREATE TABLE IF NOT EXISTS encounters (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL,
    name         VARCHAR(120) NOT NULL,
    ready        TINYINT(1)   NOT NULL DEFAULT 0,
    data         LONGTEXT     NOT NULL,
    version      INT          NOT NULL DEFAULT 1,
    created_by   CHAR(36)     NULL,
    updated_by   CHAR(36)     NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_encounters_page (entity_id, created_at),
    KEY idx_encounters_campaign (campaign_id, ready, updated_at),
    CONSTRAINT fk_encounters_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_encounters_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
