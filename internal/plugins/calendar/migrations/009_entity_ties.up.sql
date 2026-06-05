-- C-CAL-ENTITY-TIES-DATA-MODEL (build-order step 3): the real persistence
-- behind Phase 1.5's mock attach-entity picker. Today a calendar_event has a
-- single optional entity_id; there is no M:N, no era ties, no participation
-- semantics. These two link tables add the optional many-to-many both ways
-- with a participation role.
--
-- Migration-safety / placement decision = OPTION (a) (hard cross-plugin FKs),
-- and it is trivially safe here:
--   * `entities` is a CORE table (db/migrations/000001_baseline) — core
--     migrations always run BEFORE any plugin migration, so it exists.
--   * `calendar_events` / `calendar_eras` are this plugin's own tables
--     (migration 001), so they exist by migration 009.
-- Precedent: calendar_events.entity_id already hard-FKs entities(id)
-- (migration 001), so the calendar plugin already couples to the core
-- entities table at the DB layer. Hard FKs with ON DELETE CASCADE give us
-- DB-enforced cascade (deleting an entity, event, or era removes its links)
-- for free — no service-layer cascade needed. Go-level cross-plugin access
-- still goes through service interfaces per CLAUDE.md rule 8.
--
-- participation_role is a VARCHAR validated in the service layer (matching
-- the existing calendar_events.visibility pattern) rather than a SQL ENUM —
-- the enum is easier to extend without a migration and the vocabulary is
-- pinned to Phase 1.5's picker: involved | present | affected | mentioned.

-- Entity <-> event ties. participation_role is required (an event tie always
-- carries a role). Unique on the (entity, event) pair; indexed both ways for
-- the entity-side and event-side queries.
CREATE TABLE IF NOT EXISTS entity_event_links (
    id                 INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    entity_id          VARCHAR(36)  NOT NULL,
    event_id           VARCHAR(36)  NOT NULL,
    participation_role VARCHAR(20)  NOT NULL DEFAULT 'involved',
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_entity_event_links_pair (entity_id, event_id),
    INDEX idx_entity_event_links_entity (entity_id),
    INDEX idx_entity_event_links_event (event_id),
    CONSTRAINT fk_entity_event_links_entity
      FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_event_links_event
      FOREIGN KEY (event_id) REFERENCES calendar_events(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Entity <-> era ties. Era ties are coarser, so participation_role is
-- NULLABLE (an entity may simply "belong to" an era with no finer semantics).
CREATE TABLE IF NOT EXISTS entity_era_links (
    id                 INT          NOT NULL AUTO_INCREMENT PRIMARY KEY,
    entity_id          VARCHAR(36)  NOT NULL,
    era_id             INT          NOT NULL,
    participation_role VARCHAR(20)  NULL,
    created_at         DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uq_entity_era_links_pair (entity_id, era_id),
    INDEX idx_entity_era_links_entity (entity_id),
    INDEX idx_entity_era_links_era (era_id),
    CONSTRAINT fk_entity_era_links_entity
      FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_era_links_era
      FOREIGN KEY (era_id) REFERENCES calendar_eras(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
