-- prior_visibility remembers what a publication was before moderation hid it
-- (auto-flag or a moderator's archive), so approving or restoring it returns
-- it to that state instead of publishing something that was never public.
-- NULL means nothing to restore.
--
-- legacy_escaped_text marks rows whose name/description/flavor_text were
-- HTML-escaped by the old write path. Existing rows start at 1 and the boot
-- reconciler unescapes them once and sets 0; new writes store text as typed
-- with 0, so text a user wrote with a literal "&amp;" is never unescaped.
ALTER TABLE bestiary_publications
    ADD COLUMN IF NOT EXISTS prior_visibility ENUM('draft','published','unlisted','archived') DEFAULT NULL,
    ADD COLUMN IF NOT EXISTS legacy_escaped_text TINYINT(1) NOT NULL DEFAULT 1;
