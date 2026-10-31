-- 004_ai_watch: one AI key per person, switches for who may use AI, and
-- warnings when a key is used in a way that looks wrong.
--
-- ai_connections.seen_apps is every name the key's AI app has given itself,
-- comma-separated, so a new one can be noticed. The index lets each person's
-- own key be found without reading every key of the campaign.
--
-- ai_campaign_access: one row per campaign that has changed a switch.
-- players_allowed lets the campaign's players connect their own AI (off when
-- there is no row); cut_off_at set means the owner stopped every AI in the
-- campaign until they turn it back on.
--
-- ai_server_access: the site admin's switches, one row (id 1). ai_off stops
-- every key on the server; players_allowed FALSE hides the players switch in
-- every campaign. No row reads as AI on and players allowed.
--
-- ai_connection_alerts: a warning about a key, shown to the campaign's owner
-- and to the site admin until someone settles it. campaign_id is NULL for a
-- warning about an address rather than a key (wrong-key floods), which only
-- the site admin sees.
ALTER TABLE ai_connections ADD COLUMN IF NOT EXISTS seen_apps VARCHAR(255) NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_ai_connections_person ON ai_connections (campaign_id, user_id, revoked_at);

CREATE TABLE IF NOT EXISTS ai_campaign_access (
    campaign_id     VARCHAR(36) NOT NULL,
    players_allowed BOOLEAN     NOT NULL DEFAULT FALSE,
    cut_off_at      DATETIME    NULL DEFAULT NULL,
    updated_at      DATETIME    NOT NULL,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_ai_campaign_access_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ai_server_access (
    id              TINYINT     NOT NULL,
    ai_off          BOOLEAN     NOT NULL DEFAULT FALSE,
    players_allowed BOOLEAN     NOT NULL DEFAULT TRUE,
    updated_at      DATETIME    NOT NULL,
    updated_by      VARCHAR(36) NOT NULL DEFAULT '',
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ai_connection_alerts (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    campaign_id   VARCHAR(36)  NULL DEFAULT NULL,
    connection_id BIGINT       NULL DEFAULT NULL,
    kind          VARCHAR(24)  NOT NULL,
    detail        VARCHAR(200) NOT NULL,
    address       VARCHAR(45)  NOT NULL DEFAULT '',
    hits          INT          NOT NULL DEFAULT 1,
    created_at    DATETIME     NOT NULL,
    updated_at    DATETIME     NOT NULL,
    settled_at    DATETIME     NULL DEFAULT NULL,
    settled_how   VARCHAR(16)  NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    INDEX idx_ai_connection_alerts_open (settled_at, id),
    INDEX idx_ai_connection_alerts_campaign (campaign_id, settled_at),
    INDEX idx_ai_connection_alerts_key (connection_id, kind, settled_at),
    CONSTRAINT fk_ai_connection_alerts_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_ai_connection_alerts_connection FOREIGN KEY (connection_id)
        REFERENCES ai_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
