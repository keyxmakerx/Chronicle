-- 006_crafting_study: finds that need study before anyone may take them.
--
-- A drop line marked for study is copied into the tray with the mark, the
-- characteristic its study adds, and whether one failed study loses it.
-- A lost line keeps who lost it and the roll, as a taken line does.
ALTER TABLE crafting_spoil_lines
    ADD COLUMN IF NOT EXISTS study      TINYINT(1)  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS study_stat VARCHAR(40) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS study_once TINYINT(1)  NOT NULL DEFAULT 0;

-- What the party has identified. Once a hero's study lands or the Director
-- reveals an item, every player reads its name and kinds in the tray, now
-- and whenever it drops again. character_id is the hero who identified it,
-- NULL for a reveal; recipes says the study also showed which of the
-- party's recipes use it.
CREATE TABLE IF NOT EXISTS crafting_known_items (
    campaign_id  CHAR(36)   NOT NULL,
    item_id      CHAR(36)   NOT NULL,
    character_id CHAR(36)   NULL,
    recipes      TINYINT(1) NOT NULL DEFAULT 0,
    learned_by   CHAR(36)   NULL,
    learned_at   DATETIME   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, item_id),
    CONSTRAINT fk_crafting_known_items_campaign  FOREIGN KEY (campaign_id)  REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_known_items_item      FOREIGN KEY (item_id)      REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_crafting_known_items_character FOREIGN KEY (character_id) REFERENCES entities(id)  ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Every study and reveal, for the Director's history and the retry rule: a
-- hero who missed an item studies it again only after a respite logged
-- since. respite_id is the campaign's last respite when the row went in, so
-- "since the last respite" never hangs on two clocks agreeing. A study's
-- row goes in before its dice are rolled, so one hero can't roll twice at
-- once; its outcome is filled in after.
CREATE TABLE IF NOT EXISTS crafting_studies (
    id           BIGINT      NOT NULL AUTO_INCREMENT,
    campaign_id  CHAR(36)    NOT NULL,
    item_id      CHAR(36)    NOT NULL,
    line_id      BIGINT      NULL,
    character_id CHAR(36)    NULL,
    outcome      VARCHAR(12) NOT NULL DEFAULT '',
    rolled       INT         NULL,
    respite_id   BIGINT      NOT NULL DEFAULT 0,
    created_by   CHAR(36)    NULL,
    created_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_crafting_studies_tries (campaign_id, item_id, character_id, respite_id),
    KEY idx_crafting_studies_recent (campaign_id, created_at),
    CONSTRAINT fk_crafting_studies_campaign  FOREIGN KEY (campaign_id)  REFERENCES campaigns(id)            ON DELETE CASCADE,
    CONSTRAINT fk_crafting_studies_item      FOREIGN KEY (item_id)      REFERENCES entities(id)             ON DELETE CASCADE,
    CONSTRAINT fk_crafting_studies_line      FOREIGN KEY (line_id)      REFERENCES crafting_spoil_lines(id) ON DELETE SET NULL,
    CONSTRAINT fk_crafting_studies_character FOREIGN KEY (character_id) REFERENCES entities(id)             ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
