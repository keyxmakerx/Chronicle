-- Limits on hero choices. A Director can mark an entry of a pick list (the
-- game package's own or the campaign's) as not offered, ask first or
-- required, for everyone or for one player, and can mark a whole group of
-- entries (entry_key '*', one per source) the same way. A hero's pick that
-- needs a Director's yes waits in campaign_choice_requests; it is never kept
-- in the hero's own fields, which Foundry and the sheet read as real.
--
-- target_user_id is '' for everyone so the unique key holds for that case
-- too (a NULL would let duplicates in). entry_key is a package entry's slug
-- or a campaign entry's id, never a campaign entry's slug: a deleted entry's
-- slug can be handed to a new entry, its id never repeats. entry_name is a
-- snapshot so a rule on a package entry that later leaves the package can
-- still be shown. Only campaigns is referenced: rules and requests outlive
-- the accounts they name and are inert once an account leaves.
-- Core tables, core migration. No data to backfill: no row means "open".

CREATE TABLE IF NOT EXISTS campaign_choice_rules (
    id             BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id    CHAR(36)     NOT NULL,
    system_id      VARCHAR(64)  NOT NULL,
    field_key      VARCHAR(64)  NOT NULL,
    entry_source   ENUM('package','campaign') NOT NULL,
    entry_key      VARCHAR(100) NOT NULL,
    entry_name     VARCHAR(100) NOT NULL DEFAULT '',
    target_user_id CHAR(36)     NOT NULL DEFAULT '',
    state          ENUM('open','ask_first','not_offered','required') NOT NULL,
    reason         VARCHAR(300) NOT NULL DEFAULT '',
    created_by     CHAR(36)     NOT NULL DEFAULT '',
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uq_campaign_choice_rules (campaign_id, system_id, field_key, entry_source, entry_key, target_user_id),
    KEY idx_campaign_choice_rules_target (campaign_id, system_id, target_user_id),
    CONSTRAINT fk_campaign_choice_rules_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS campaign_choice_requests (
    id           BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id  CHAR(36)     NOT NULL,
    system_id    VARCHAR(64)  NOT NULL,
    field_key    VARCHAR(64)  NOT NULL,
    entry_source ENUM('package','campaign') NOT NULL,
    entry_key    VARCHAR(100) NOT NULL,
    entry_name   VARCHAR(100) NOT NULL DEFAULT '',
    user_id      CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL DEFAULT '',
    entity_name  VARCHAR(200) NOT NULL DEFAULT '',
    status       ENUM('waiting','allowed','suggested','withdrawn') NOT NULL DEFAULT 'waiting',
    message      VARCHAR(300) NOT NULL DEFAULT '',
    decided_by   CHAR(36)     NOT NULL DEFAULT '',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at   DATETIME     NULL DEFAULT NULL,

    KEY idx_campaign_choice_requests_status (campaign_id, status),
    KEY idx_campaign_choice_requests_user (campaign_id, user_id, field_key),
    CONSTRAINT fk_campaign_choice_requests_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
