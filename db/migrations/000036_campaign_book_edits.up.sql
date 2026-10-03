-- A campaign's own edits to a game system's Rulebook book. The package's book
-- files stay untouched on disk; a campaign's edition is the package book plus
-- the rows below, merged on read, so a package update can never overwrite
-- what a Director wrote and the Director can always see what changed.
-- Core tables, core migration: only campaigns is referenced.

-- House-rules chapters: chapters the campaign added. chapter_id starts with
-- "house_", which no package chapter id can (package ids have no underscore).
CREATE TABLE IF NOT EXISTS campaign_book_chapters (
    campaign_id CHAR(36)     NOT NULL,
    system_id   VARCHAR(64)  NOT NULL,
    chapter_id  VARCHAR(64)  NOT NULL,
    title       VARCHAR(200) NOT NULL,
    intro       TEXT         NOT NULL,
    director    BOOLEAN      NOT NULL DEFAULT FALSE,
    sort_order  INT          NOT NULL DEFAULT 0,
    created_by  CHAR(36)     DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (campaign_id, system_id, chapter_id),
    CONSTRAINT fk_campaign_book_chapters_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Pages. package_index is the 0-based position of the package page this row
-- replaces; NULL marks a page the campaign added (ordered by sort_order).
-- base_hash is the hash of the package page the campaign copied, so a later
-- package change to that page is noticed. updated_by is deliberately not a
-- foreign key: the page must outlive the account that wrote it.
-- The unique key allows many NULL package_index rows (own pages) per chapter
-- and exactly one copy per package page.
CREATE TABLE IF NOT EXISTS campaign_book_pages (
    id            BIGINT      AUTO_INCREMENT PRIMARY KEY,
    campaign_id   CHAR(36)    NOT NULL,
    system_id     VARCHAR(64) NOT NULL,
    chapter_id    VARCHAR(64) NOT NULL,
    package_index INT         DEFAULT NULL,
    sort_order    INT         NOT NULL DEFAULT 0,
    page_json     MEDIUMTEXT  NOT NULL,
    base_hash     CHAR(64)    DEFAULT NULL,
    updated_by    CHAR(36)    DEFAULT NULL,
    created_at    DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uq_campaign_book_pages_slot (campaign_id, system_id, chapter_id, package_index),
    INDEX idx_campaign_book_pages_lookup (campaign_id, system_id, chapter_id, sort_order),
    CONSTRAINT fk_campaign_book_pages_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
