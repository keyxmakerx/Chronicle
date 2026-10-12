-- 001_page_blocks: the text a DM types into a page's Read-aloud, Table and
-- Steps blocks.
--
-- One JSON document per (page, layout block), validated by the service, so a
-- block's shape can grow without a schema change; version guards concurrent
-- edits. block_id is the layout block's id, which is unique within a page
-- layout but not across pages, hence the composite key. kind is kept beside
-- the document so a block whose type was swapped in the layout cannot be read
-- as the wrong shape. Core tables run before plugin ones, and the collation
-- matches them, so the foreign keys are safe.
CREATE TABLE IF NOT EXISTS page_block_content (
    entity_id    CHAR(36)    NOT NULL,
    block_id     VARCHAR(64) NOT NULL,
    campaign_id  CHAR(36)    NOT NULL,
    kind         VARCHAR(24) NOT NULL,
    data         LONGTEXT    NOT NULL,
    version      INT         NOT NULL DEFAULT 1,
    updated_by   CHAR(36)    NULL,
    updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (entity_id, block_id),
    CONSTRAINT fk_page_block_content_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_page_block_content_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
