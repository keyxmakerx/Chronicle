-- Migration 000045: Map expansion for Foundry VTT sync.
-- Adds layers, drawings, tokens, and fog of war to the maps system.
-- Also adds grid and sync settings to the maps table.

-- Map layers for organizing content (background, drawing, token, gm, fog).
CREATE TABLE map_layers (
    id CHAR(36) NOT NULL PRIMARY KEY,
    map_id CHAR(36) NOT NULL,
    name VARCHAR(200) NOT NULL,
    layer_type VARCHAR(50) NOT NULL COMMENT 'background, drawing, token, gm, fog',
    sort_order INT NOT NULL DEFAULT 0,
    is_visible BOOLEAN NOT NULL DEFAULT TRUE,
    opacity DECIMAL(3,2) NOT NULL DEFAULT 1.00,
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_layer_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE,
    INDEX idx_layer_map (map_id, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Freehand drawings, shapes, and text annotations on maps.
CREATE TABLE map_drawings (
    id CHAR(36) NOT NULL PRIMARY KEY,
    map_id CHAR(36) NOT NULL,
    layer_id CHAR(36),
    drawing_type VARCHAR(50) NOT NULL COMMENT 'freehand, rectangle, ellipse, polygon, text',
    points JSON NOT NULL COMMENT 'Array of {x, y} pairs in percentage coords (0-100)',
    stroke_color VARCHAR(7) NOT NULL DEFAULT '#000000',
    stroke_width DECIMAL(5,2) NOT NULL DEFAULT 2.0,
    fill_color VARCHAR(7) COMMENT 'NULL = no fill',
    fill_alpha DECIMAL(3,2) NOT NULL DEFAULT 0.5,
    text_content TEXT COMMENT 'Text content for text drawings',
    font_size INT,
    rotation DECIMAL(6,2) NOT NULL DEFAULT 0,
    visibility VARCHAR(20) NOT NULL DEFAULT 'everyone',
    created_by CHAR(36),
    foundry_id VARCHAR(255) COMMENT 'Foundry Drawing document ID',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_drawing_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE,
    CONSTRAINT fk_drawing_layer FOREIGN KEY (layer_id) REFERENCES map_layers(id) ON DELETE SET NULL,
    INDEX idx_drawing_map (map_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Tokens placed on maps (characters, NPCs, objects).
CREATE TABLE map_tokens (
    id CHAR(36) NOT NULL PRIMARY KEY,
    map_id CHAR(36) NOT NULL,
    layer_id CHAR(36),
    entity_id CHAR(36) COMMENT 'Optional link to Chronicle entity',
    name VARCHAR(200) NOT NULL,
    image_path VARCHAR(500) COMMENT 'Token image (media path or URL)',
    x DECIMAL(8,4) NOT NULL COMMENT 'Percentage position 0-100',
    y DECIMAL(8,4) NOT NULL,
    width DECIMAL(8,4) NOT NULL DEFAULT 1.0 COMMENT 'Grid units',
    height DECIMAL(8,4) NOT NULL DEFAULT 1.0,
    rotation DECIMAL(6,2) NOT NULL DEFAULT 0,
    scale DECIMAL(5,3) NOT NULL DEFAULT 1.0,
    is_hidden BOOLEAN NOT NULL DEFAULT FALSE COMMENT 'GM-only visibility',
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    bar1_value INT,
    bar1_max INT,
    bar2_value INT,
    bar2_max INT,
    aura_radius DECIMAL(8,4),
    aura_color VARCHAR(7),
    light_radius DECIMAL(8,4),
    light_dim_radius DECIMAL(8,4),
    light_color VARCHAR(7),
    vision_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    vision_range DECIMAL(8,4),
    elevation INT NOT NULL DEFAULT 0,
    sort_order INT NOT NULL DEFAULT 0,
    status_effects JSON COMMENT 'Array of status effect strings',
    flags JSON COMMENT 'Extensible metadata',
    foundry_id VARCHAR(255) COMMENT 'Foundry Token document ID',
    created_by CHAR(36),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT fk_token_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE,
    CONSTRAINT fk_token_layer FOREIGN KEY (layer_id) REFERENCES map_layers(id) ON DELETE SET NULL,
    CONSTRAINT fk_token_entity FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE SET NULL,
    INDEX idx_token_map (map_id),
    INDEX idx_token_entity (entity_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Fog of war regions that the GM can reveal/hide.
CREATE TABLE map_fog (
    id CHAR(36) NOT NULL PRIMARY KEY,
    map_id CHAR(36) NOT NULL,
    points JSON NOT NULL COMMENT 'Polygon vertices as {x, y} pairs',
    is_explored BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_fog_map FOREIGN KEY (map_id) REFERENCES maps(id) ON DELETE CASCADE,
    INDEX idx_fog_map (map_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Add grid, viewport, and Foundry sync columns to the existing maps table.
ALTER TABLE maps
    ADD COLUMN grid_type VARCHAR(20) DEFAULT 'none' COMMENT 'none, square, hex',
    ADD COLUMN grid_size DECIMAL(8,4) DEFAULT 50.0 COMMENT 'Grid cell size in pixels',
    ADD COLUMN grid_color VARCHAR(7) DEFAULT '#000000',
    ADD COLUMN grid_opacity DECIMAL(3,2) DEFAULT 0.2,
    ADD COLUMN background_color VARCHAR(7) DEFAULT '#222222',
    ADD COLUMN fog_exploration BOOLEAN DEFAULT FALSE COMMENT 'Enable fog of war',
    ADD COLUMN initial_view_x DECIMAL(8,4) DEFAULT 50.0 COMMENT 'Default viewport center X',
    ADD COLUMN initial_view_y DECIMAL(8,4) DEFAULT 50.0 COMMENT 'Default viewport center Y',
    ADD COLUMN initial_zoom DECIMAL(5,3) DEFAULT 1.0,
    ADD COLUMN foundry_scene_id VARCHAR(255) COMMENT 'Linked Foundry Scene ID';
