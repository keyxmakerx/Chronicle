-- 001_page_pins: pages the Director pins for everyone. The Pinned Pages
-- widget on a campaign home shows them above each viewer's own pins (their
-- starred pages, entity_favorites), leaving out any page that viewer can't
-- open.
--
-- One row per pinned page. Deleting the page or the campaign removes the pin;
-- a page in the Trash keeps its row unseen, since reads join the live page,
-- so restoring it pins it again. pinned_by has no foreign key on purpose: it
-- only records who pinned and must not block removing a user. Core tables run
-- before plugin ones and the collation matches them.
CREATE TABLE IF NOT EXISTS page_pins (
    campaign_id  CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL,
    pinned_by    CHAR(36)     NULL,
    created_at   DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (campaign_id, entity_id),
    KEY idx_page_pins_entity (entity_id),
    CONSTRAINT fk_page_pins_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_page_pins_entity   FOREIGN KEY (entity_id)   REFERENCES entities(id)  ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
