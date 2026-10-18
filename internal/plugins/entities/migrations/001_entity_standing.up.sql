-- 001_entity_standing: how a group page (a faction, a guild, a person) feels
-- about the party or about one character, and the reasons the Director gives.
--
-- entity_standings holds one row per (page, subject): the word the Director
-- set by hand (level 0 Hostile .. 4 Trusted). subject_key is '' for the whole
-- party and the character's page id otherwise; it is a plain key rather than a
-- nullable foreign key so the party can share the unique key below (NULLs never
-- collide in a unique key). A deleted or trashed character page leaves its row
-- behind unseen: reads join the live page, so restoring the page brings it back.
--
-- entity_standing_reasons holds the explanations, newest first. A reason marked
-- dm_only is for the Director; every read for a player filters it out in SQL.
-- direction is 'up', 'down' or 'none' (checked by the service, not an ENUM).
--
-- entity_standing_settings is the page-wide switch for whether players see the
-- standing at all; a page with no row here lets them (the Director's default).
--
-- created_by / updated_by have no foreign key on purpose: they only record who
-- acted and must not block removing a user. Core tables run before plugin
-- ones and the collation matches them, so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS entity_standings (
    id           CHAR(36)         NOT NULL,
    campaign_id  CHAR(36)         NOT NULL,
    entity_id    CHAR(36)         NOT NULL,
    subject_key  CHAR(36)         NOT NULL DEFAULT '',
    level        TINYINT UNSIGNED NOT NULL DEFAULT 2,
    updated_by   CHAR(36)         NULL,
    created_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at   DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uq_entity_standing_subject (entity_id, subject_key),
    KEY idx_entity_standing_campaign (campaign_id),
    CONSTRAINT fk_entity_standing_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_standing_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS entity_standing_reasons (
    id           CHAR(36)     NOT NULL,
    standing_id  CHAR(36)     NOT NULL,
    direction    VARCHAR(4)   NOT NULL,
    reason       VARCHAR(90)  NOT NULL,
    when_label   VARCHAR(30)  NOT NULL DEFAULT '',
    dm_only      TINYINT(1)   NOT NULL DEFAULT 0,
    created_by   CHAR(36)     NULL,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_entity_standing_reason_order (standing_id, created_at),
    CONSTRAINT fk_entity_standing_reason_standing FOREIGN KEY (standing_id) REFERENCES entity_standings(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS entity_standing_settings (
    entity_id    CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    players_see  TINYINT(1)   NOT NULL DEFAULT 1,
    updated_by   CHAR(36)     NULL,
    updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (entity_id),
    KEY idx_entity_standing_settings_campaign (campaign_id),
    CONSTRAINT fk_entity_standing_settings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_standing_settings_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
