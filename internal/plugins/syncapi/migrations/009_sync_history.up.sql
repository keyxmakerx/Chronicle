-- Sync history: one row per thing that synced between Chronicle and an
-- external client, in either direction, so the owner and the module read the
-- same list. Chronicle writes the rows for calls it receives; the module
-- reports what only it can see (applied in Foundry, set aside, skipped).
-- Rows carry names and ids, never page text. parent_id groups the children
-- of one catch-up run under the row that started it.

CREATE TABLE IF NOT EXISTS sync_events (
    id            BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id   VARCHAR(36)  NOT NULL,
    parent_id     BIGINT       NULL DEFAULT NULL,
    occurred_at   DATETIME(3)  NOT NULL,
    direction     ENUM('to_chronicle','to_foundry','link') NOT NULL,
    reported_by   ENUM('chronicle','client') NOT NULL,
    kind          VARCHAR(20)  NOT NULL DEFAULT '',
    resource_id   VARCHAR(64)  NOT NULL DEFAULT '',
    resource_name VARCHAR(200) NOT NULL DEFAULT '',
    action        VARCHAR(80)  NOT NULL DEFAULT '',
    call_desc     VARCHAR(200) NOT NULL DEFAULT '',
    status        VARCHAR(16)  NOT NULL DEFAULT '',
    ok            TINYINT(1)   NOT NULL DEFAULT 1,
    duration_ms   INT          NOT NULL DEFAULT 0,
    message       VARCHAR(500) NOT NULL DEFAULT '',
    user_id       VARCHAR(36)  NULL DEFAULT NULL,
    api_key_id    INT          NULL DEFAULT NULL,
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_sync_events_campaign (campaign_id, parent_id, id),
    KEY idx_sync_events_parent (parent_id),
    KEY idx_sync_events_created (created_at),
    CONSTRAINT fk_sync_events_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
