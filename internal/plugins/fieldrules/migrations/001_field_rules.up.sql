-- 001_field_rules: a campaign's worked-out rules and local words.
--
-- A rule maps the value of one field (follows) to another through a table of
-- rows, kept as JSON because a rule is always read and written whole and has
-- at most a few dozen rows.
--
-- A local word is what one place (a page) or one era calls a word. scope_key
-- is 'p:<page id>' or 'e:<era id>' so one unique key can say a word has one
-- local name per place or era; place_id carries the foreign key so removing
-- the place removes its words. era_id has no foreign key because eras belong
-- to the calendar plugin's tables; a word whose era is gone simply never
-- applies, and the service drops it from lists.
CREATE TABLE IF NOT EXISTS field_rules (
    id          INT          NOT NULL AUTO_INCREMENT,
    campaign_id CHAR(36)     NOT NULL,
    name        VARCHAR(100) NOT NULL,
    follows     VARCHAR(100) NOT NULL,
    rows_json   JSON         NOT NULL,
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_field_rules_name (campaign_id, name),
    CONSTRAINT fk_field_rules_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS local_words (
    id          INT          NOT NULL AUTO_INCREMENT,
    campaign_id CHAR(36)     NOT NULL,
    word        VARCHAR(100) NOT NULL,
    local_word  VARCHAR(100) NOT NULL,
    place_id    CHAR(36)     NULL,
    era_id      INT          NULL,
    scope_key   VARCHAR(40)  NOT NULL,
    dm_only     TINYINT(1)   NOT NULL DEFAULT 0,
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_local_words_scope (campaign_id, word, scope_key),
    CONSTRAINT fk_local_words_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_local_words_place    FOREIGN KEY (place_id)    REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
