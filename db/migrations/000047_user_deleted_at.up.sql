-- When a person deleted their own account. The row stays, emptied and
-- disabled, because content other members rely on references it; this
-- column tells a deleted account from one an admin merely disabled.
-- Core table, core migration: no plugin tables referenced.

ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at DATETIME NULL;
