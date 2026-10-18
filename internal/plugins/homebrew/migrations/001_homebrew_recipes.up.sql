-- Homebrew Workshop recipes: one JSON document per recipe. Parts and rules
-- are validated by the service and the plugins that own each kind of part,
-- so the store itself holds no schema for them.
CREATE TABLE IF NOT EXISTS homebrew_recipes (
    id          CHAR(36)     NOT NULL,
    campaign_id CHAR(36)     NOT NULL,
    name        VARCHAR(80)  NOT NULL,
    doc         MEDIUMTEXT   NOT NULL,
    version     INT          NOT NULL DEFAULT 1,
    created_by  CHAR(36)     NULL,
    created_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_homebrew_recipes_name (campaign_id, name),
    CONSTRAINT fk_homebrew_recipes_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
