-- 007_ai_request_undo: a kept "before" for what an applied request changed,
-- so the owner can put it back from "What it did".
--
-- ai_request_befores: one row per change an applied request made that can be
-- put back (a page's title and text, a home page, a tracker's settings).
-- before_json is what the thing was (NULL when it did not exist, so undoing
-- takes it away); after_hash is a fingerprint of the state the AI's change
-- left, which Undo compares with the live state so it never overwrites work
-- done since. Rows go with their request (ON DELETE CASCADE) and a purge
-- removes them once keep_until has passed.
--
-- ai_connection_requests.undone_at: when the owner pressed Undo. It is set by
-- one conditional update, so a double press cannot run the undo twice.
CREATE TABLE IF NOT EXISTS ai_request_befores (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    request_id  BIGINT       NOT NULL,
    campaign_id VARCHAR(36)  NOT NULL,
    kind        VARCHAR(32)  NOT NULL,
    target_id   VARCHAR(64)  NOT NULL DEFAULT '',
    target_name VARCHAR(255) NOT NULL DEFAULT '',
    action      VARCHAR(16)  NOT NULL,
    before_json MEDIUMTEXT   NULL,
    after_hash  CHAR(64)     NOT NULL DEFAULT '',
    created_at  DATETIME     NOT NULL,
    keep_until  DATETIME     NOT NULL,
    undone_at   DATETIME     NULL DEFAULT NULL,
    undone_by   VARCHAR(36)  NULL DEFAULT NULL,
    PRIMARY KEY (id),
    INDEX idx_ai_request_befores_request (request_id),
    INDEX idx_ai_request_befores_keep (keep_until),
    CONSTRAINT fk_ai_request_befores_request FOREIGN KEY (request_id)
        REFERENCES ai_connection_requests(id) ON DELETE CASCADE,
    CONSTRAINT fk_ai_request_befores_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE ai_connection_requests ADD COLUMN IF NOT EXISTS undone_at DATETIME NULL DEFAULT NULL;
