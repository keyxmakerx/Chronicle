-- Maps plugin: interactive maps with image overlays and pin markers.
-- Each campaign can have multiple maps (world, region, city, dungeon, etc.).
-- Markers can link to entities and have visibility controls.

-- Map definitions. Each map has an uploaded background image.
CREATE TABLE IF NOT EXISTS maps (
    id           VARCHAR(36)  NOT NULL PRIMARY KEY,
    campaign_id  VARCHAR(36)  NOT NULL,
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    image_id     VARCHAR(36)  DEFAULT NULL,
    image_width  INT          NOT NULL DEFAULT 0,
    image_height INT          NOT NULL DEFAULT 0,
    sort_order   INT          NOT NULL DEFAULT 0,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_maps_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_maps_image FOREIGN KEY (image_id) REFERENCES media_files(id) ON DELETE SET NULL,
    INDEX idx_maps_campaign (campaign_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Map markers (pins). Positioned by percentage coordinates (0-100) on the map image.
-- Optional entity linking and DM-only visibility.
CREATE TABLE IF NOT EXISTS map_markers (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    map_id      VARCHAR(36)  NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    x           DOUBLE       NOT NULL DEFAULT 50,
    y           DOUBLE       NOT NULL DEFAULT 50,
    icon        VARCHAR(100) NOT NULL DEFAULT 'fa-map-pin',
    color       VARCHAR(7)   NOT NULL DEFAULT '#3b82f6',
    entity_id   VARCHAR(36)  DEFAULT NULL,
    visibility  VARCHAR(20)  NOT NULL DEFAULT 'everyone',
    created_by  VARCHAR(36)  DEFAULT NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_markers_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE,
    CONSTRAINT fk_markers_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE SET NULL,
    INDEX idx_markers_map (map_id),
    INDEX idx_markers_entity (entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Update the maps addon from planned/widget to active/plugin now that
-- the backing code is implemented. Row was seeded in migration 000015.
-- The 'plugin' ENUM value was added in migration 000027.
UPDATE addons
SET category    = 'plugin',
    status      = 'active',
    description = 'Interactive maps with pin markers, entity linking, and DM-only visibility. Upload world, region, city, or dungeon maps.'
WHERE slug = 'maps';
