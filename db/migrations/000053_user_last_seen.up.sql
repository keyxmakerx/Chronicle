-- When each person last used the site at all, not just signed in with a
-- password: sessions last weeks, so last_login_at says little about whether
-- someone is still around. Written at most once a day per person. An admin
-- may hand over a campaign only once its owner has been away longer than the
-- owner allowed.
-- Core tables, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen_at DATETIME NULL;
