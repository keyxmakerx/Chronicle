-- 003_encounter_shown: what scenes showed to players, newest last. A
-- player's Table lists tonight's rows meant for everyone or for them; the
-- DM team sees every row. Rows copy the label and note, so a renamed or
-- deleted page doesn't change what was shown. The foreign key points at a
-- core table, which migrates before any plugin, and the collation matches.
CREATE TABLE IF NOT EXISTS encounter_shown (
    id           CHAR(36)      NOT NULL,
    campaign_id  CHAR(36)      NOT NULL,
    kind         VARCHAR(16)   NOT NULL,
    label        VARCHAR(255)  NOT NULL DEFAULT '',
    text         TEXT          NOT NULL,
    entity_id    CHAR(36)      NULL,
    to_user      CHAR(36)      NULL,
    scene        VARCHAR(255)  NOT NULL DEFAULT '',
    shown_by     CHAR(36)      NULL,
    shown_at     DATETIME      NOT NULL,
    PRIMARY KEY (id),
    KEY idx_encounter_shown_campaign_time (campaign_id, shown_at),
    CONSTRAINT fk_encounter_shown_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
