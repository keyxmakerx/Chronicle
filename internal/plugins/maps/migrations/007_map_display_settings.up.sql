-- Per-map display settings (frame override, pin style, kinds of pin, grid,
-- opening view, who can draw) as one nullable JSON document. NULL means every
-- default. The service validates and normalises it before it is written, so
-- the column never holds a key the renderer does not know.
ALTER TABLE maps
    ADD COLUMN IF NOT EXISTS display_settings JSON DEFAULT NULL AFTER background_color;

-- Campaign-wide default frame for the map page. A campaign with no row here
-- uses the application default (atlas). Owned by this plugin, not by the
-- campaigns table, so the campaigns plugin never has to know map looks exist.
CREATE TABLE IF NOT EXISTS map_campaign_settings (
    campaign_id VARCHAR(36) NOT NULL PRIMARY KEY,
    frame_style VARCHAR(20) NOT NULL DEFAULT 'atlas',
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    CONSTRAINT fk_map_campaign_settings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
