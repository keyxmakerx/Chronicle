-- Addon registry: all installable addons (modules, widgets, integrations).
-- Admin controls global enable/disable; campaign owners toggle per-campaign.
CREATE TABLE IF NOT EXISTS addons (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    slug        VARCHAR(100) NOT NULL UNIQUE,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    version     VARCHAR(50) NOT NULL DEFAULT '0.1.0',
    category    ENUM('module', 'widget', 'integration') NOT NULL DEFAULT 'module',
    status      ENUM('active', 'planned', 'deprecated') NOT NULL DEFAULT 'active',
    icon        VARCHAR(100) DEFAULT 'fa-puzzle-piece',
    author      VARCHAR(200),
    config_schema JSON DEFAULT NULL,
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Per-campaign addon settings: which addons are enabled for a campaign.
CREATE TABLE IF NOT EXISTS campaign_addons (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id CHAR(36) NOT NULL,
    addon_id    INT NOT NULL,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    config_json JSON DEFAULT NULL,
    enabled_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enabled_by  CHAR(36),

    UNIQUE KEY uq_campaign_addon (campaign_id, addon_id),
    FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    FOREIGN KEY (addon_id) REFERENCES addons(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Seed default addons from the existing module/widget registries.
INSERT INTO addons (slug, name, description, version, category, status, icon, author) VALUES
    ('dnd5e', 'D&D 5th Edition', 'Reference data, stat blocks, and tooltips for Dungeons & Dragons 5th Edition', '0.1.0', 'module', 'active', 'fa-dragon', 'Chronicle'),
    ('pathfinder2e', 'Pathfinder 2nd Edition', 'Reference data and tooltips for Pathfinder 2nd Edition', '0.1.0', 'module', 'active', 'fa-shield-halved', 'Chronicle'),
    ('drawsteel', 'Draw Steel', 'Reference data for the Draw Steel RPG system', '0.1.0', 'module', 'active', 'fa-swords', 'Chronicle'),
    ('player-notes', 'Player Notes', 'Personal note-taking blocks for players on entity pages and standalone pages', '0.1.0', 'widget', 'planned', 'fa-sticky-note', 'Chronicle'),
    ('calendar', 'Calendar', 'Campaign calendar with custom date systems and event tracking', '0.1.0', 'widget', 'planned', 'fa-calendar-days', 'Chronicle'),
    ('maps', 'Interactive Maps', 'Leaflet.js map viewer with entity pins and layer support', '0.1.0', 'widget', 'planned', 'fa-map', 'Chronicle'),
    ('sync-api', 'Sync API', 'Secure REST API for external tool integration (Foundry VTT, Roll20, etc.)', '0.1.0', 'integration', 'planned', 'fa-arrows-rotate', 'Chronicle'),
    ('media-gallery', 'Media Gallery', 'Advanced media management with albums, tagging, and lightbox', '0.1.0', 'widget', 'active', 'fa-images', 'Chronicle'),
    ('timeline', 'Timeline', 'Visual timeline widget for campaign events and entity histories', '0.1.0', 'widget', 'planned', 'fa-timeline', 'Chronicle'),
    ('family-tree', 'Family Tree', 'Visual family/org tree diagram from entity relations', '0.1.0', 'widget', 'planned', 'fa-sitemap', 'Chronicle'),
    ('dice-roller', 'Dice Roller', 'In-app dice rolling with formula support and history', '0.1.0', 'widget', 'active', 'fa-dice-d20', 'Chronicle');
