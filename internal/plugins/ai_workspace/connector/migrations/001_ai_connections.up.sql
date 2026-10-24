-- 001_ai_connections: an AI app connected to one campaign, and what it did.
--
-- ai_connections: one row per connection key. Only the SHA-256 of the key is
-- kept (key_hash); the key itself is shown to the owner once and never
-- stored. key_hint is its last four characters, so the owner can tell keys
-- apart. privacy is the privacy mode the owner chose when making it, which
-- the AI can never change. A connection stops working when revoked_at is set,
-- when expires_at passes, or when user_id is no longer the campaign's owner.
--
-- ai_connection_calls: every tool call a connection made (when, which tool, a
-- one-line summary), so the owner can see what their AI looked up. Never the
-- answer itself.
--
-- Core migrations run before plugin ones, so the foreign key to campaigns is
-- safe; the collation matches that table.
CREATE TABLE IF NOT EXISTS ai_connections (
    id           BIGINT       NOT NULL AUTO_INCREMENT,
    campaign_id  VARCHAR(36)  NOT NULL,
    user_id      VARCHAR(36)  NOT NULL,
    key_hash     CHAR(64)     NOT NULL,
    key_hint     VARCHAR(8)   NOT NULL,
    privacy      VARCHAR(16)  NOT NULL,
    client_name  VARCHAR(64)  NOT NULL DEFAULT '',
    created_at   DATETIME     NOT NULL,
    expires_at   DATETIME     NOT NULL,
    last_used_at DATETIME     NULL DEFAULT NULL,
    revoked_at   DATETIME     NULL DEFAULT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uq_ai_connections_key (key_hash),
    INDEX idx_ai_connections_campaign (campaign_id, revoked_at),
    CONSTRAINT fk_ai_connections_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ai_connection_calls (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    connection_id BIGINT       NOT NULL,
    campaign_id   VARCHAR(36)  NOT NULL,
    called_at     DATETIME     NOT NULL,
    tool          VARCHAR(40)  NOT NULL,
    summary       VARCHAR(200) NOT NULL,
    note          VARCHAR(120) NOT NULL DEFAULT '',
    ok            BOOLEAN      NOT NULL DEFAULT TRUE,
    PRIMARY KEY (id),
    INDEX idx_ai_connection_calls_campaign (campaign_id, id),
    CONSTRAINT fk_ai_connection_calls_connection FOREIGN KEY (connection_id)
        REFERENCES ai_connections(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
