-- 002_encounter_runs: the fight the DM team is running on the DM Screen's
-- Fight tab, one per campaign.
--
-- The run copies the encounter's heroes and creatures when it starts, so it
-- has no foreign key to encounters: a fight in progress keeps going if its
-- encounter is changed or deleted. Turn order, hit points and conditions are
-- one JSON document validated by the service; version guards two DMs editing
-- at once. The foreign key points at a core table, which migrates before any
-- plugin, and the collation matches it.
CREATE TABLE IF NOT EXISTS encounter_runs (
    campaign_id  CHAR(36)  NOT NULL,
    data         LONGTEXT  NOT NULL,
    version      INT       NOT NULL DEFAULT 1,
    updated_by   CHAR(36)  NULL,
    updated_at   DATETIME  NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_encounter_runs_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
