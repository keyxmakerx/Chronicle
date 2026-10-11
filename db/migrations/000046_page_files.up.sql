-- Which page each attached file belongs to, and whether it is for the GM only.
-- A page file (media_files.usage_type 'page_file') is opened only through the
-- page it is bound to here: whoever can see that page can download it, unless
-- gm_only, which hides it from everyone below the GM tier. media_id is the
-- primary key because a file belongs to exactly one page. Both parents are core
-- tables. Deleting the file removes the row; purging the page removes the row
-- too, and the file left behind is swept as unbound.

CREATE TABLE IF NOT EXISTS page_files (
    media_id    CHAR(36)   NOT NULL,
    entity_id   CHAR(36)   NOT NULL,
    campaign_id CHAR(36)   NOT NULL,
    gm_only     TINYINT(1) NOT NULL DEFAULT 0,
    created_by  CHAR(36)   NOT NULL,
    created_at  TIMESTAMP  NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (media_id),
    INDEX idx_page_files_entity (entity_id, created_at),

    CONSTRAINT fk_page_files_media  FOREIGN KEY (media_id)  REFERENCES media_files(id) ON DELETE CASCADE,
    CONSTRAINT fk_page_files_entity FOREIGN KEY (entity_id) REFERENCES entities(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
