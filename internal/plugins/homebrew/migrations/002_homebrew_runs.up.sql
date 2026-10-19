-- Rules a recipe ran or is holding for the Director. Each row keeps the rule
-- as it was when its event fired, so a held rule runs what was held even if
-- the recipe changed since. Old finished rows are pruned by the service.
CREATE TABLE IF NOT EXISTS homebrew_runs (
    id           CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    recipe_id    CHAR(36)     NOT NULL,
    recipe_name  VARCHAR(80)  NOT NULL,
    what         VARCHAR(200) NOT NULL,
    rule_json    TEXT         NOT NULL,
    scope        VARCHAR(36)  NULL,
    payload      TEXT         NULL,
    status       VARCHAR(16)  NOT NULL,
    outcome      TEXT         NULL,
    created_by   CHAR(36)     NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    done_by      CHAR(36)     NULL,
    done_at      DATETIME     NULL,
    PRIMARY KEY (id),
    KEY idx_homebrew_runs_status (campaign_id, status, created_at),
    CONSTRAINT fk_homebrew_runs_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
