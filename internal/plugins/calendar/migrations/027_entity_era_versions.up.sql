-- 027_entity_era_versions — a page's version for one calendar era.
--
-- A row changes only what it sets; a NULL column means "as the usual page":
--   name        the page's name in this era.
--   image_path  the page's picture in this era.
--   entry       the page's text in this era (editor JSON), with entry_html
--               rendered from it. entry_rev counts text saves, so a save on
--               top of someone else's newer text is caught.
--   fields_data only the field values that differ from the usual page.
-- Deleting the page or the era deletes its versions.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS entity_era_versions (
    entity_id   VARCHAR(36)  NOT NULL,
    era_id      INT          NOT NULL,
    name        VARCHAR(200) DEFAULT NULL,
    image_path  VARCHAR(500) DEFAULT NULL,
    entry       JSON         DEFAULT NULL,
    entry_html  LONGTEXT     DEFAULT NULL,
    entry_rev   INT          NOT NULL DEFAULT 0,
    fields_data JSON         DEFAULT NULL,
    updated_by  VARCHAR(36)  DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (entity_id, era_id),
    INDEX idx_entity_era_versions_era (era_id),
    CONSTRAINT fk_entity_era_versions_entity
      FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_era_versions_era
      FOREIGN KEY (era_id) REFERENCES calendar_eras(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
