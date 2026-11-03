-- 001_hero_layers: layers on a hero's page (a vampire's bite, rising as a
-- revenant). A layer lists the fields it changes as one JSON document the
-- service validates; the page's own fields are never written, which is
-- what lets a layer come off cleanly. Layers apply oldest first. Foreign
-- keys point at core tables only, which migrate before any plugin, and the
-- collation matches them; deleting a page deletes its layers.
CREATE TABLE IF NOT EXISTS hero_layers (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL,
    name         VARCHAR(80)  NOT NULL,
    source       VARCHAR(120) NOT NULL DEFAULT '',
    data         LONGTEXT     NOT NULL,
    created_by   CHAR(36)     NULL,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_hero_layers_page (entity_id, created_at),
    KEY idx_hero_layers_campaign (campaign_id),
    CONSTRAINT fk_hero_layers_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_hero_layers_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
