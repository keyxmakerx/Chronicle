-- Per-campaign change feed for external sync clients (GET /sync/changes).
-- A client that missed live WebSocket frames replays ids from here instead
-- of rescanning every resource. Rows carry ids only, never content, so
-- visibility is applied when the client refetches through the normal reads.

CREATE TABLE IF NOT EXISTS sync_changes (
    seq           BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id   VARCHAR(36)  NOT NULL,
    resource_type VARCHAR(40)  NOT NULL,
    resource_id   VARCHAR(64)  NOT NULL DEFAULT '',
    op            ENUM('created','updated','deleted') NOT NULL,
    created_at    DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    KEY idx_sync_changes_campaign_seq (campaign_id, seq),
    KEY idx_sync_changes_created (created_at),
    CONSTRAINT fk_sync_changes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Highest seq the retention job has deleted for a campaign. A client whose
-- cursor is below it has a gap in the feed and must do a full resync.
CREATE TABLE IF NOT EXISTS sync_change_watermarks (
    campaign_id    VARCHAR(36) NOT NULL PRIMARY KEY,
    pruned_through BIGINT      NOT NULL DEFAULT 0,

    CONSTRAINT fk_sync_change_watermarks_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
