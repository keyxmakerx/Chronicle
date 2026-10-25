-- 002_ai_note_access: the owner's switches on the Your AI page for which
-- shared Journal entries and jots the AI may change besides the owner's own:
-- the ones co-DMs shared, and the ones Scribes shared. No row means both
-- off.
CREATE TABLE IF NOT EXISTS ai_note_access (
    campaign_id VARCHAR(36) NOT NULL,
    co_dms      BOOLEAN     NOT NULL DEFAULT FALSE,
    scribes     BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_by  VARCHAR(36) NOT NULL,
    updated_at  TIMESTAMP   NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_ai_note_access_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
