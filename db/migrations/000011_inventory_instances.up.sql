-- Inventory instances: standalone named item collections per campaign.
-- Examples: "Party Loot", "Blacksmith Shop", "Enemy Cache".
-- Items are linked to instances via the inventory_items junction table.

CREATE TABLE inventory_instances (
    id INT AUTO_INCREMENT PRIMARY KEY,
    campaign_id VARCHAR(36) NOT NULL,
    name VARCHAR(100) NOT NULL,
    slug VARCHAR(100) NOT NULL,
    description TEXT DEFAULT NULL,
    icon VARCHAR(50) DEFAULT 'fa-box',
    color VARCHAR(7) DEFAULT '#6b7280',
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    UNIQUE KEY uq_inventory_instances_campaign_slug (campaign_id, slug),
    INDEX idx_inventory_instances_campaign (campaign_id, sort_order),
    CONSTRAINT fk_inventory_instances_campaign FOREIGN KEY (campaign_id)
        REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE inventory_items (
    id INT AUTO_INCREMENT PRIMARY KEY,
    instance_id INT NOT NULL,
    entity_id VARCHAR(36) NOT NULL,
    quantity INT DEFAULT 1,
    notes TEXT DEFAULT NULL,
    added_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_inventory_items_instance_entity (instance_id, entity_id),
    INDEX idx_inventory_items_instance (instance_id),
    CONSTRAINT fk_inventory_items_instance FOREIGN KEY (instance_id)
        REFERENCES inventory_instances(id) ON DELETE CASCADE,
    CONSTRAINT fk_inventory_items_entity FOREIGN KEY (entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE shop_transactions
    ADD COLUMN instance_id INT DEFAULT NULL AFTER campaign_id,
    ADD INDEX idx_shop_tx_instance (instance_id);
