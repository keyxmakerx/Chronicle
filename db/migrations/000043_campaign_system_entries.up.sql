-- A campaign's own entries for a game system's pick lists (ancestries, kits,
-- cultures, careers, classes, races...). They live apart from the system
-- package's data files, so installing or updating a package can never touch
-- or overwrite what a Director added; the pick list is the package's entries
-- plus these rows, merged on read.
-- created_by is deliberately not a foreign key: an entry must outlive the
-- account that wrote it. visibility 'directors' keeps an entry out of every
-- player's list.
-- Core tables, core migration: only campaigns is referenced, and the code
-- that owns the table (internal/systems) is core, like campaign_book_*.

CREATE TABLE IF NOT EXISTS campaign_system_entries (
    id          BIGINT       AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    field_key   VARCHAR(64)  NOT NULL,
    slug        VARCHAR(64)  NOT NULL,
    name        VARCHAR(100) NOT NULL,
    summary     VARCHAR(200) NOT NULL DEFAULT '',
    description MEDIUMTEXT   NOT NULL,
    properties  JSON         NOT NULL DEFAULT ('{}'),
    visibility  ENUM('everyone','directors') NOT NULL DEFAULT 'everyone',
    created_by  CHAR(36)     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uq_campaign_system_entries_slug (campaign_id, system_id, field_key, slug),
    INDEX idx_campaign_system_entries_name (campaign_id, system_id, field_key, name),
    CONSTRAINT fk_campaign_system_entries_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
