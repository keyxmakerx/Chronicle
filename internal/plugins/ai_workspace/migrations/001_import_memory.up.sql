-- 001_import_memory: what AI Import has applied in a campaign, so a later
-- paste can be checked against it.
--
-- ai_import_parts: when an AI splits its answer over several pastes it labels
-- each one (`kind: part`, a name shared by every part, the part's number, how
-- many there are and the earlier parts it needs). A pasted part waits in the
-- tray (markdown set) until the whole answer is in, then the parts are
-- reviewed together; applied_at is set once a part is applied, and its
-- markdown is cleared.
--
-- ai_import_blocks: a fingerprint (SHA-256 of the block's text) of every
-- block applied, so pasting the same block again is pointed out before it
-- makes a duplicate. Only the fingerprint is kept, never the text.
--
-- Core migrations run before plugin ones, so the foreign keys to campaigns
-- are safe; the collation matches that table.
CREATE TABLE IF NOT EXISTS ai_import_parts (
    campaign_id VARCHAR(36)  NOT NULL,
    answer_key  VARCHAR(191) NOT NULL,
    part_no     INT          NOT NULL,
    answer_name VARCHAR(255) NOT NULL,
    part_of     INT          NOT NULL DEFAULT 0,
    needs       VARCHAR(255) NOT NULL DEFAULT '',
    markdown    MEDIUMTEXT   NULL,
    added_by    VARCHAR(36)  NOT NULL,
    added_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    applied_at  TIMESTAMP    NULL DEFAULT NULL,
    PRIMARY KEY (campaign_id, answer_key, part_no),
    INDEX idx_ai_import_parts_tray (campaign_id, applied_at),
    CONSTRAINT fk_ai_import_parts_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ai_import_blocks (
    campaign_id VARCHAR(36) NOT NULL,
    block_hash  CHAR(64)    NOT NULL,
    applied_at  TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, block_hash),
    CONSTRAINT fk_ai_import_blocks_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
