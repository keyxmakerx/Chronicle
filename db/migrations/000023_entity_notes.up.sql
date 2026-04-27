-- entity_notes: per-user, per-entity notes with a 5-tier audience ACL.
-- Distinct from `notes` (campaign-scoped floating panel) and from
-- `entity_posts` (entity-scoped sub-content shared by all members).
-- This table is the backing store for the "Player Notes" entity-page
-- block: each player can author private/dm-only/dm-scribe/everyone/
-- custom-share notes attached to an entity.
--
-- Audience semantics (enforced in repo + service, mirrored client-side):
--   private    — author only. Even DMs cannot read another user's
--                private notes; this is intentional and matches what
--                Roll20/Foundry/Kanka do.
--   dm_only    — RoleOwner + any user with campaign_members.is_dm_granted=1.
--   dm_scribe  — RoleOwner + RoleScribe + is_dm_granted users.
--                Maps to the codebase's "Co-DM" concept (Scribe role's
--                doc string at internal/plugins/campaigns/model.go:33-34
--                already calls Scribe "the TTRPG note-taker / co-author").
--   everyone   — all members of the campaign.
--   custom     — explicit user_ids in shared_with JSON array.
--
-- entity_id is NOT NULL: this table is *always* entity-scoped. The
-- existing campaign-wide `notes` table covers the floating-panel use
-- case; we don't conflate the two. If we ever want campaign-scoped
-- player notes, that's a separate column or a separate table.

CREATE TABLE entity_notes (
    id           CHAR(36)     NOT NULL,
    entity_id    CHAR(36)     NOT NULL,
    campaign_id  CHAR(36)     NOT NULL,
    -- author_user_id, not "user_id", because *who can see the note* is
    -- determined by the audience field + shared_with list, not by the
    -- author. The author always sees their own note regardless of audience.
    author_user_id CHAR(36)   NOT NULL,
    audience     ENUM('private','dm_only','dm_scribe','everyone','custom')
                 NOT NULL DEFAULT 'private',
    -- shared_with is a JSON array of user_ids; only meaningful when
    -- audience='custom'. NULL otherwise. Validated server-side that
    -- entries are valid UUIDs and the user is a campaign member.
    shared_with  JSON         NULL,
    title        VARCHAR(200) NULL,
    -- TipTap ProseMirror JSON for the rich-text body. Same shape used
    -- by entity_posts.entry and entities.entry — clients reuse the
    -- same TipTap setup (window.TipTap globals from the vendor bundle).
    body         JSON         NULL     COMMENT 'TipTap/ProseMirror JSON',
    -- Pre-sanitized HTML for display. Sanitization runs on every write
    -- via internal/sanitize.HTML; this column is always safe to inject
    -- into a Templ render via templ.Raw.
    body_html    LONGTEXT     NULL     COMMENT 'Pre-rendered & sanitized HTML',
    pinned       BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    -- Dominant query: "list all notes on this entity that this user can
    -- see, newest first." Index on (entity_id, audience) accelerates
    -- the audience filter; author lookup goes through PK + author index
    -- below for "show me MY notes."
    INDEX idx_entity_notes_entity_audience (entity_id, audience),
    INDEX idx_entity_notes_author (author_user_id),
    INDEX idx_entity_notes_campaign (campaign_id),

    CONSTRAINT fk_entity_notes_entity
        FOREIGN KEY (entity_id) REFERENCES entities(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_notes_campaign
        FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
    CONSTRAINT fk_entity_notes_author
        FOREIGN KEY (author_user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
