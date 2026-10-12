-- Extra places a page is listed in the page tree. A page keeps exactly one
-- real parent (entities.parent_id, which the breadcrumb follows); each row
-- here adds one more spot where the same page shows up, so it is edited once
-- instead of cloned and left to drift.
--
-- Both ends cascade: purging a page (or its parent) removes the listing, while
-- a page that is only in the Trash keeps its rows so Restore brings the
-- listings back. Reads hide a listing whose page or parent is trashed.
-- campaign_id is stored so a cross-campaign row can be told apart from a good
-- one by the repository's checks and by the integration tests.
-- created_by is deliberately not a foreign key: the listing must outlive the
-- account that made it. Core tables only (entities); core migration.

CREATE TABLE IF NOT EXISTS entity_places (
    entity_id        CHAR(36)     NOT NULL,
    parent_entity_id CHAR(36)     NOT NULL,
    campaign_id      CHAR(36)     NOT NULL,
    sort_order       INT          NOT NULL DEFAULT 0,
    created_by       CHAR(36)     NULL DEFAULT NULL,
    created_at       TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    PRIMARY KEY (entity_id, parent_entity_id),
    INDEX idx_entity_places_parent (parent_entity_id, sort_order),
    INDEX idx_entity_places_campaign (campaign_id),

    CONSTRAINT fk_entity_places_entity FOREIGN KEY (entity_id)        REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_places_parent FOREIGN KEY (parent_entity_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
