-- Rename addon category ENUM value from 'module' to 'system'.
-- This aligns the database with the codebase rename of modules → systems.

-- MariaDB doesn't support ALTER TYPE directly, so we modify the column ENUM.
-- The original ENUM was: ENUM('module', 'widget', 'integration', 'plugin')
ALTER TABLE addons
    MODIFY COLUMN category ENUM('system', 'widget', 'integration', 'plugin') NOT NULL;

-- Update existing rows that had 'module' category.
-- (The ALTER above automatically converts 'module' → '' since it's no longer valid,
--  so we need to update first. Let's do it in the right order.)

-- Actually, MariaDB will keep the old value if the new ENUM includes it.
-- Since we removed 'module', any existing 'module' rows become ''.
-- We need a different approach: add 'system', update rows, then remove 'module'.

-- Step 1: Add 'system' to the ENUM (keep 'module' temporarily).
ALTER TABLE addons
    MODIFY COLUMN category ENUM('module', 'system', 'widget', 'integration', 'plugin') NOT NULL;

-- Step 2: Update existing module rows to system.
UPDATE addons SET category = 'system' WHERE category = 'module';

-- Step 3: Remove 'module' from the ENUM.
ALTER TABLE addons
    MODIFY COLUMN category ENUM('system', 'widget', 'integration', 'plugin') NOT NULL;
