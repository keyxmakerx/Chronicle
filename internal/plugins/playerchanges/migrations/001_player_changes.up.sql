-- 001_player_changes: what players changed on pages, for the owner's
-- "What players changed" page, and the packed months behind it.
--
-- player_changes holds the last month of changes one row each: a sheet
-- field, a page's title or text, or a page removed. Values are kept twice:
-- old_text/new_text as the page shows them, old_json/new_json as stored, so
-- Undo writes back exactly what was there. entity_id, user_id and undone_by
-- carry no foreign key: the history must outlive the page and the account,
-- and page_name/user_name keep what the row says after either is gone.
--
-- player_change_packs holds older rows, one gzip'd JSON array per campaign
-- and month. keep_until is set when a month is packed, from the site's
-- Long-term trash length at that moment, so shortening the setting never
-- removes anything early.
--
-- player_change_kept_pages names the pages a player removed that the
-- campaign Trash must keep past its days (Long-term trash), until
-- keep_until, so the owner can still bring them back whole.
--
-- Core tables run before plugin ones; the only foreign keys point at
-- campaigns, so removing a campaign removes its history.
CREATE TABLE IF NOT EXISTS player_changes (
    id          BIGINT UNSIGNED  NOT NULL AUTO_INCREMENT,
    campaign_id CHAR(36)         NOT NULL,
    entity_id   CHAR(36)         NOT NULL,
    page_name   VARCHAR(200)     NOT NULL,
    user_id     CHAR(36)         NOT NULL,
    user_name   VARCHAR(200)     NOT NULL DEFAULT '',
    kind        VARCHAR(12)      NOT NULL,
    field_key   VARCHAR(100)     NOT NULL DEFAULT '',
    field_label VARCHAR(200)     NOT NULL DEFAULT '',
    old_text    TEXT             NULL,
    new_text    TEXT             NULL,
    old_json    MEDIUMTEXT       NULL,
    new_json    MEDIUMTEXT       NULL,
    source      VARCHAR(16)      NOT NULL DEFAULT '',
    app         VARCHAR(80)      NOT NULL DEFAULT '',
    created_at  DATETIME(6)      NOT NULL,
    undone_at   DATETIME         NULL DEFAULT NULL,
    undone_by   CHAR(36)         NULL DEFAULT NULL,
    PRIMARY KEY (id),
    KEY idx_player_changes_campaign (campaign_id, created_at),
    KEY idx_player_changes_page (campaign_id, entity_id, created_at),
    CONSTRAINT fk_player_changes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS player_change_packs (
    campaign_id CHAR(36)         NOT NULL,
    month       DATE             NOT NULL,
    row_count   INT UNSIGNED     NOT NULL DEFAULT 0,
    body        MEDIUMBLOB       NOT NULL,
    keep_until  DATE             NOT NULL,
    packed_at   DATETIME         NOT NULL,
    PRIMARY KEY (campaign_id, month),
    KEY idx_player_change_packs_keep (keep_until),
    CONSTRAINT fk_player_change_packs_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS player_change_kept_pages (
    campaign_id CHAR(36)         NOT NULL,
    entity_id   CHAR(36)         NOT NULL,
    keep_until  DATE             NOT NULL,
    PRIMARY KEY (campaign_id, entity_id),
    KEY idx_player_change_kept_until (keep_until),
    CONSTRAINT fk_player_change_kept_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
