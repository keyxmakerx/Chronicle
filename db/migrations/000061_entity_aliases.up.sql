-- Add entity_aliases table for multiple canonical names per entity.
-- Aliases appear in auto-linking, search, and @mention results.
CREATE TABLE entity_aliases (
    id         INT UNSIGNED NOT NULL AUTO_INCREMENT,
    entity_id  CHAR(36)     NOT NULL,
    alias      VARCHAR(200) NOT NULL,
    created_at DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_alias_entity (entity_id, alias),
    FULLTEXT INDEX ft_alias (alias),
    CONSTRAINT fk_alias_entity FOREIGN KEY (entity_id)
        REFERENCES entities(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
