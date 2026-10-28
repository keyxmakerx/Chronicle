-- 002_crafting_projects: projects a hero is working on, the respites that
-- give them rolls, and every roll made.
--
-- A project keeps what it started with (the recipe's name, goal,
-- characteristic and the item it makes), so a recipe changed or deleted
-- mid-project never moves the goal under the player. The live recipe's rules
-- still apply while it exists. Rolls ready stack: each respite adds one to
-- every project in progress. A finished project keeps whether its item
-- reached the hero's pack, so a failed give is shown to the Director rather
-- than lost. Rolls are kept as made, dice and all, so a result can always be
-- checked.
CREATE TABLE IF NOT EXISTS crafting_projects (
    id             CHAR(36)     NOT NULL,
    campaign_id    CHAR(36)     NOT NULL,
    character_id   CHAR(36)     NOT NULL,
    recipe_id      CHAR(36)     NULL,
    recipe_name    VARCHAR(120) NOT NULL,
    goal           INT          NOT NULL,
    stat           VARCHAR(40)  NOT NULL DEFAULT '',
    result_item_id CHAR(36)     NULL,
    result_qty     INT          NOT NULL DEFAULT 1,
    points         INT          NOT NULL DEFAULT 0,
    rolls_ready    INT          NOT NULL DEFAULT 0,
    status         VARCHAR(12)  NOT NULL DEFAULT 'active',
    delivered      TINYINT(1)   NOT NULL DEFAULT 0,
    took           VARCHAR(500) NOT NULL DEFAULT '',
    started_by     CHAR(36)     NULL,
    started_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at    DATETIME     NULL,
    version        INT          NOT NULL DEFAULT 1,
    PRIMARY KEY (id),
    KEY idx_crafting_projects_status (campaign_id, status),
    KEY idx_crafting_projects_character (campaign_id, character_id),
    CONSTRAINT fk_crafting_projects_campaign  FOREIGN KEY (campaign_id)    REFERENCES campaigns(id)        ON DELETE CASCADE,
    CONSTRAINT fk_crafting_projects_character FOREIGN KEY (character_id)   REFERENCES entities(id)         ON DELETE CASCADE,
    CONSTRAINT fk_crafting_projects_recipe    FOREIGN KEY (recipe_id)      REFERENCES crafting_recipes(id) ON DELETE SET NULL,
    CONSTRAINT fk_crafting_projects_result    FOREIGN KEY (result_item_id) REFERENCES entities(id)         ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One row per respite logged, from the Crafting app or the Homebrew drawer,
-- with the calendar's date at the time when there is one.
CREATE TABLE IF NOT EXISTS crafting_respites (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    campaign_id CHAR(36)     NOT NULL,
    logged_by   CHAR(36)     NULL,
    day_label   VARCHAR(80)  NOT NULL DEFAULT '',
    projects    INT          NOT NULL DEFAULT 0,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_crafting_respites_campaign (campaign_id, id),
    CONSTRAINT fk_crafting_respites_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Every project roll: the dice as they fell, each term added, and the
-- points before and after.
CREATE TABLE IF NOT EXISTS crafting_rolls (
    id          BIGINT       NOT NULL AUTO_INCREMENT,
    project_id  CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    rolled_by   CHAR(36)     NULL,
    dice        VARCHAR(40)  NOT NULL DEFAULT '',
    terms       TEXT         NOT NULL,
    total       INT          NOT NULL DEFAULT 0,
    points_from INT          NOT NULL DEFAULT 0,
    points_to   INT          NOT NULL DEFAULT 0,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_crafting_rolls_project (project_id, id),
    CONSTRAINT fk_crafting_rolls_project  FOREIGN KEY (project_id)  REFERENCES crafting_projects(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_rolls_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)         ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
