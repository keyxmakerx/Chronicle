-- C-PERM-ANON-IDENTITY: add 'public' as a grant subject on both permission
-- tables so an owner can DELIBERATELY reveal an entity (or a tagged set of
-- entities) to everyone, including logged-out visitors.
--
-- Anonymous/public viewers now resolve to RoleNone (0), strictly below
-- RolePlayer (1), so they no longer match Player-role grants. The 'public'
-- subject is the explicit, first-class "everyone incl. the public" target that
-- replaces the (previously impossible) notion of a Player grant leaking to
-- anonymous viewers.
--
-- Additive ENUM extension only — no data reshape (mirrors 000027's additive
-- note). The visibility filter matches subject_type='public' unconditionally,
-- so existing rows are unaffected.
ALTER TABLE entity_permissions
    MODIFY COLUMN subject_type ENUM('role','user','group','public') NOT NULL;

ALTER TABLE tag_permissions
    MODIFY COLUMN subject_type ENUM('role','user','group','public') NOT NULL;
