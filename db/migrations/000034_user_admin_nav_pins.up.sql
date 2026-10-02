-- A site admin's own pinned admin pages: a JSON array of admin sidebar item
-- links (e.g. "/admin/users") in the order pinned; NULL for none. It lives on
-- the user row so pins belong to that admin alone and follow them across
-- devices. Stored values that no longer name an admin page are ignored on read.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS admin_nav_pins JSON NULL;
