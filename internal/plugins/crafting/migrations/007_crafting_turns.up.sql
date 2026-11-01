-- 007_crafting_turns: the once-a-respite turns a hero spends outside their
-- own projects: an experiment ('try') and a hand on a party member's project
-- ('help', with the project it went to).
--
-- respite_id is the campaign's latest crafting_respites row when the turn
-- was spent (0 before the first respite), so a new respite gives every turn
-- back without touching this table. It carries no foreign key for that
-- reason. project_id is '' for a try.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
CREATE TABLE IF NOT EXISTS crafting_turns (
    campaign_id  CHAR(36)    NOT NULL,
    character_id CHAR(36)    NOT NULL,
    respite_id   BIGINT      NOT NULL DEFAULT 0,
    kind         VARCHAR(8)  NOT NULL,
    project_id   VARCHAR(36) NOT NULL DEFAULT '',
    created_at   DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (campaign_id, character_id, respite_id, kind, project_id),
    CONSTRAINT fk_crafting_turns_campaign  FOREIGN KEY (campaign_id)  REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_crafting_turns_character FOREIGN KEY (character_id) REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
