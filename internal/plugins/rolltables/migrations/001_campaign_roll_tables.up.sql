-- 001_campaign_roll_tables: one rolling-table document per campaign.
--
-- The whole set of a campaign's tables is one JSON document, replaced as a
-- unit; the service validates and normalizes it, so the shape can grow
-- without a schema change. campaign_id is the primary key because a campaign
-- has at most one document.
--
-- The collation matches the core campaigns table it references, and core
-- migrations run before plugin ones, so the foreign key is safe.
CREATE TABLE IF NOT EXISTS campaign_roll_tables (
    campaign_id  CHAR(36)    NOT NULL,
    data         LONGTEXT    NOT NULL,
    updated_by   CHAR(36)    NULL,
    updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_campaign_roll_tables_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
