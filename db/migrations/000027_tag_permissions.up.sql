-- ============================================================================
-- Tag-based visibility grants (C-PERM-W1-TAG-GRANTS).
-- ============================================================================
-- A tag may carry visibility grants. An entity bearing a granted tag becomes
-- visible to the grant's subjects EVEN IF it would otherwise be hidden
-- (dm_only / custom-without-you). Grants are ADDITIVE ONLY -- a tag can widen
-- visibility, never narrow it. Untag the entity or revoke the grant and the
-- entity re-hides.
--
-- Lives in core (not a plugin migrations dir) because the tags / entity_tags
-- tables it builds on are themselves core tables (000001_baseline): the tags
-- surface is a Chronicle widget, not a plugin with its own migrations dir, so
-- "the plugin that owns the tags tables" is core. tag_id therefore FKs a core
-- table and this migration is safe on a fresh DB.
--
-- subject_type/subject_id deliberately mirror entity_permissions so the
-- visibility filter's subject-match fragment is identical across the two grant
-- tables. subject_id is VARCHAR(36): a role int as text, a campaign-group int
-- as text, or a user UUID -- exactly as entity_permissions stores it.
-- 'custom_role' is intentionally omitted from the ENUM here (W2) -- adding it is
-- an additive ALTER then, no reshape.
--
-- created_by is an attribution field with no FK on purpose: deleting the
-- granting user must NOT cascade-delete grants (that would silently re-hide
-- content -- the exact failure mode this feature's "never silently expose"
-- discipline guards against).

CREATE TABLE IF NOT EXISTS tag_permissions (
    id           INT          AUTO_INCREMENT PRIMARY KEY,
    tag_id       INT          NOT NULL,
    subject_type ENUM('role','user','group') NOT NULL,
    subject_id   VARCHAR(36)  NOT NULL,
    created_by   CHAR(36)     NOT NULL,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY uq_tag_perm (tag_id, subject_type, subject_id),
    INDEX idx_tag_perm_tag (tag_id),
    INDEX idx_tag_perm_subject (subject_type, subject_id),

    CONSTRAINT fk_tag_perm_tag
        FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
