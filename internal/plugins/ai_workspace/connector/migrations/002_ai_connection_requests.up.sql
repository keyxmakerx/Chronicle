-- 002_ai_connection_requests: an AI app may ask for changes, and the owner
-- says yes or no to each one in Chronicle.
--
-- ai_connections.can_ask is the connection's Change setting, chosen by the
-- owner when the key is made: FALSE ("Only look") or TRUE ("Ask for
-- changes"). Keys made before this column read as Only look.
--
-- ai_connection_requests: what an AI asked for, waiting for the owner. body
-- is the blocks it sent, in the same format as a paste; nothing in it is
-- applied until the owner reviews it and presses Apply. summary is the AI's
-- own one line (shown in quotes), counts is Chronicle's ("2 to add, 1 to
-- change"). status is waiting, answered (reviewed and applied) or
-- turned_down; outcome says what was applied.
ALTER TABLE ai_connections ADD COLUMN IF NOT EXISTS can_ask BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS ai_connection_requests (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    campaign_id   VARCHAR(36)  NOT NULL,
    connection_id BIGINT       NOT NULL,
    app_name      VARCHAR(64)  NOT NULL DEFAULT '',
    summary       VARCHAR(200) NOT NULL DEFAULT '',
    counts        VARCHAR(200) NOT NULL DEFAULT '',
    blocks        INT          NOT NULL,
    body          MEDIUMTEXT   NOT NULL,
    status        VARCHAR(16)  NOT NULL DEFAULT 'waiting',
    outcome       VARCHAR(120) NOT NULL DEFAULT '',
    created_at    DATETIME     NOT NULL,
    decided_at    DATETIME     NULL DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_ai_connection_requests_campaign (campaign_id, status, id),
    CONSTRAINT fk_ai_connection_requests_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
