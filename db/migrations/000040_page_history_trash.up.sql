-- Undo for world pages: a Trash for deleted pages and a history of each
-- page's title and text.
--
-- Deleting a page now marks it (deleted_at) instead of removing the row, so
-- everything that hangs off it (relations, tags, pictures, map pins, sub-pages)
-- is still there when it is restored. trash_root_id is the page whose delete
-- trashed this row: a page deleted with its sub-pages shares one id, so the
-- Trash lists and restores them as one item. A purge hard-deletes rows once
-- they have waited the site's trash retention.
--
-- entry_rev counts changes to the page text. The editor sends the rev it
-- loaded, so a save based on text someone else has since changed is refused
-- instead of silently replacing it.
--
-- entity_versions times keep microseconds so saves within one second still
-- list in the order they happened.
--
-- entity_versions.user_id is deliberately not a foreign key: history must
-- outlive the account that wrote it. Core tables only (entities); core migration.

ALTER TABLE entities
    ADD COLUMN IF NOT EXISTS deleted_at    DATETIME     NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS deleted_by    CHAR(36)     NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS trash_root_id CHAR(36)     NULL DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS entry_rev     INT UNSIGNED NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_entities_trash ON entities (campaign_id, deleted_at);

CREATE TABLE IF NOT EXISTS entity_versions (
    id          CHAR(36)     NOT NULL PRIMARY KEY,
    entity_id   CHAR(36)     NOT NULL,
    user_id     CHAR(36)     NULL DEFAULT NULL,
    kind        VARCHAR(16)  NOT NULL DEFAULT 'edit',
    name        VARCHAR(200) NOT NULL,
    entry       JSON         NULL,
    entry_html  LONGTEXT     NULL,
    created_at  DATETIME(6)  NOT NULL,
    updated_at  DATETIME(6)  NOT NULL,

    INDEX idx_entity_versions_entity (entity_id, created_at DESC),
    CONSTRAINT fk_entity_versions_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
