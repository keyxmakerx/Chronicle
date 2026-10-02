-- Reverse 000034: drop the admin's own pinned admin pages.
ALTER TABLE users DROP COLUMN IF EXISTS admin_nav_pins;
