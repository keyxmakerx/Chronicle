-- 008_crafting_packs: recipe packs, a set of crafting recipes saved with
-- their materials' names so another campaign can bring them in.
--
-- body is the pack as JSON: recipe and material names, kinds, quantities,
-- steps and project numbers, never a page's content, a rule, who knows a
-- recipe or any id from the campaign it came from. shared lets any Director
-- on the server bring it in; otherwise only Directors of campaign_id can.
-- source is 'saved' (from the Workshop) or 'file' (uploaded).
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS crafting_packs (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    created_by   CHAR(36)     NULL,
    name         VARCHAR(120) NOT NULL,
    source       VARCHAR(8)   NOT NULL DEFAULT 'saved',
    shared       TINYINT(1)   NOT NULL DEFAULT 0,
    recipe_count INT          NOT NULL DEFAULT 0,
    body         MEDIUMTEXT   NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    KEY idx_crafting_packs_campaign (campaign_id),
    KEY idx_crafting_packs_shared (shared),
    CONSTRAINT fk_crafting_packs_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
