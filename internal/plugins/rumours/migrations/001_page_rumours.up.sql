-- 001_page_rumours: the Rumours page block. A rumour is something the party
-- heard about a page (a person, a place, a guild): what was said, who said it,
-- how sure they were, what the Director knows to be true, and what the party
-- has decided to believe.
--
-- truth is the Director's alone ('open' until they settle it). belief is the
-- party's, set by a Scribe or the Director ('' until someone marks it). They
-- are separate columns on purpose: a read for anyone below the Director never
-- selects truth at all (see the repository), so there is nothing to filter
-- out afterwards.
--
-- by_dm and belief_by_dm record that the Director's side wrote the rumour or
-- marked the belief, so the block can say "the DM" and leave "added by" off
-- Director-written rumours without comparing roles that may have changed since. created_by / belief_by carry no foreign key on
-- purpose: they only record who acted and must not block removing a user;
-- the block shows a name when the user still exists.
--
-- Core tables run before plugin ones and the collation matches them, so the
-- foreign keys to campaigns and entities are safe; deleting a page deletes
-- its rumours.
CREATE TABLE IF NOT EXISTS page_rumours (
    id          CHAR(36)         NOT NULL,
    campaign_id CHAR(36)         NOT NULL,
    entity_id   CHAR(36)         NOT NULL,
    quote       VARCHAR(280)     NOT NULL,
    source      VARCHAR(80)      NOT NULL DEFAULT '',
    sure        TINYINT UNSIGNED NOT NULL DEFAULT 2,
    truth       VARCHAR(5)       NOT NULL DEFAULT 'open',
    belief      VARCHAR(6)       NOT NULL DEFAULT '',
    belief_by   CHAR(36)         NULL,
    belief_by_dm TINYINT(1)      NOT NULL DEFAULT 0,
    by_dm       TINYINT(1)       NOT NULL DEFAULT 0,
    created_by  CHAR(36)         NULL,
    created_at  DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)      NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_page_rumours_page (entity_id, created_at),
    KEY idx_page_rumours_campaign (campaign_id),
    CONSTRAINT fk_page_rumours_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_page_rumours_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
