-- 001_crafting: a campaign's crafting.
--
-- Recipes are the Director's own: materials (Armory items, or "any herb" by
-- tag), steps from the plugin's fixed library, the item made, the project
-- fields the game system names, a coin cost, the kind of bench it is made at,
-- and who knows it. Materials, steps, rules and looks are JSON the service
-- cleans on every write, so nothing stored can break the bench that plays
-- it. A material's item id is not a foreign key: a deleted item leaves the
-- recipe saying so rather than silently changing what it asks for. Core
-- tables run before plugin ones, and the collation matches them, so the
-- foreign keys are safe.
CREATE TABLE IF NOT EXISTS crafting_settings (
    campaign_id  CHAR(36)    NOT NULL,
    skin         VARCHAR(12) NOT NULL DEFAULT 'desk',
    difficulty   VARCHAR(8)  NOT NULL DEFAULT 'std',
    bane         TINYINT(1)  NOT NULL DEFAULT 1,
    skip_steps   TINYINT(1)  NOT NULL DEFAULT 1,
    updated_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id),
    CONSTRAINT fk_crafting_settings_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS crafting_recipes (
    id             CHAR(36)     NOT NULL,
    campaign_id    CHAR(36)     NOT NULL,
    name           VARCHAR(120) NOT NULL,
    template       VARCHAR(24)  NOT NULL DEFAULT '',
    bench_kind     VARCHAR(24)  NOT NULL DEFAULT 'apothecary',
    result_item_id CHAR(36)     NULL,
    result_qty     INT          NOT NULL DEFAULT 1,
    goal           INT          NOT NULL DEFAULT 30,
    stat           VARCHAR(40)  NOT NULL DEFAULT '',
    prerequisite   VARCHAR(200) NOT NULL DEFAULT '',
    source         VARCHAR(200) NOT NULL DEFAULT '',
    cost_cents     BIGINT       NOT NULL DEFAULT 0,
    cost_currency  VARCHAR(12)  NOT NULL DEFAULT '',
    who            VARCHAR(8)   NOT NULL DEFAULT 'all',
    materials      TEXT         NOT NULL,
    steps          TEXT         NOT NULL,
    colors         VARCHAR(255) NOT NULL DEFAULT '',
    rules          MEDIUMTEXT   NOT NULL,
    looks          TEXT         NOT NULL,
    draft          TINYINT(1)   NOT NULL DEFAULT 0,
    draft_prompt   VARCHAR(500) NOT NULL DEFAULT '',
    sort_order     INT          NOT NULL DEFAULT 0,
    version        INT          NOT NULL DEFAULT 1,
    created_by     CHAR(36)     NULL,
    created_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_crafting_recipes_name (campaign_id, name),
    KEY idx_crafting_recipes_order (campaign_id, sort_order),
    CONSTRAINT fk_crafting_recipes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_recipes_result FOREIGN KEY (result_item_id) REFERENCES entities(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Who knows a "must be learned" recipe: one row per character.
CREATE TABLE IF NOT EXISTS crafting_known (
    recipe_id    CHAR(36) NOT NULL,
    character_id CHAR(36) NOT NULL,
    campaign_id  CHAR(36) NOT NULL,
    added_by     CHAR(36) NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (recipe_id, character_id),
    KEY idx_crafting_known_character (campaign_id, character_id),
    CONSTRAINT fk_crafting_known_recipe    FOREIGN KEY (recipe_id)    REFERENCES crafting_recipes(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_known_character FOREIGN KEY (character_id) REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Crafting tags on Armory items: what kind of material each one is.
CREATE TABLE IF NOT EXISTS crafting_material_tags (
    item_id     CHAR(36)    NOT NULL,
    tag         VARCHAR(24) NOT NULL,
    campaign_id CHAR(36)    NOT NULL,
    PRIMARY KEY (item_id, tag),
    KEY idx_crafting_material_tags_campaign (campaign_id),
    CONSTRAINT fk_crafting_material_tags_item     FOREIGN KEY (item_id)     REFERENCES entities(id)  ON DELETE CASCADE,
    CONSTRAINT fk_crafting_material_tags_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- A bench is a named place of one bench kind. Each look (the desk, the
-- journal) keeps its own arrangement from build mode, NULL until arranged.
CREATE TABLE IF NOT EXISTS crafting_benches (
    id          CHAR(36)    NOT NULL,
    campaign_id CHAR(36)    NOT NULL,
    name        VARCHAR(80) NOT NULL,
    kind        VARCHAR(24) NOT NULL,
    desk        MEDIUMTEXT  NULL,
    journal     MEDIUMTEXT  NULL,
    sort_order  INT         NOT NULL DEFAULT 0,
    version     INT         NOT NULL DEFAULT 1,
    created_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_crafting_benches_name (campaign_id, name),
    CONSTRAINT fk_crafting_benches_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
