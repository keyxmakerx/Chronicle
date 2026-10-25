-- 001_page_layouts: a category's named page layouts besides its main one, and
-- which of them each page wears.
--
-- entity_type_page_layouts holds one JSON document per category: the main
-- layout's name, the layout a player's character starts on, and the extra
-- layouts with their blocks (the shape the entities service validates). The
-- main layout itself stays in entity_types.layout_json.
--
-- entity_page_layouts holds a page's choice of layout by key. A key that no
-- longer names one of its category's layouts reads as the main layout, so
-- removing a layout or moving the page to another category needs no cleanup.
CREATE TABLE IF NOT EXISTS entity_type_page_layouts (
    entity_type_id INT          NOT NULL,
    campaign_id    CHAR(36)     NOT NULL,
    doc            LONGTEXT     NOT NULL,
    updated_at     DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (entity_type_id),
    KEY idx_entity_type_page_layouts_campaign (campaign_id),
    CONSTRAINT fk_entity_type_page_layouts_type     FOREIGN KEY (entity_type_id) REFERENCES entity_types(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_type_page_layouts_campaign FOREIGN KEY (campaign_id)    REFERENCES campaigns(id)    ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS entity_page_layouts (
    entity_id    CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    layout_key   VARCHAR(32)  NOT NULL,
    updated_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (entity_id),
    KEY idx_entity_page_layouts_campaign (campaign_id),
    CONSTRAINT fk_entity_page_layouts_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_entity_page_layouts_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
