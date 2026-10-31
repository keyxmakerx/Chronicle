-- 003_crafting_pictures: which picture a material wears.
--
-- Pictures are drawings with their own names (a leaf, a flask), never tied
-- to a material. Every kind ships with a default picture and tint in the
-- plugin's library; a row in crafting_kind_pictures is a campaign's own
-- choice for one kind, and no row means the default. A row in
-- crafting_material_pictures gives one Armory item its own picture and tint
-- instead of following its first kind's. Keys are checked against the
-- library on write and dropped on read when the library no longer has them.
CREATE TABLE IF NOT EXISTS crafting_kind_pictures (
    campaign_id CHAR(36)    NOT NULL,
    kind        VARCHAR(24) NOT NULL,
    picture     VARCHAR(24) NOT NULL,
    tint        CHAR(7)     NOT NULL,
    PRIMARY KEY (campaign_id, kind),
    CONSTRAINT fk_crafting_kind_pictures_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS crafting_material_pictures (
    item_id     CHAR(36)    NOT NULL,
    campaign_id CHAR(36)    NOT NULL,
    picture     VARCHAR(24) NOT NULL,
    tint        CHAR(7)     NOT NULL,
    PRIMARY KEY (item_id),
    KEY idx_crafting_material_pictures_campaign (campaign_id),
    CONSTRAINT fk_crafting_material_pictures_item     FOREIGN KEY (item_id)     REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_crafting_material_pictures_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
