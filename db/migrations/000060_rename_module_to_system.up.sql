-- Rename addon category ENUM value from 'module' to 'system'.
-- This aligns the database with the codebase rename of modules → systems.
-- Uses a 3-step approach: add new value, update rows, remove old value.

-- Step 1: Add 'system' to the ENUM (keep 'module' temporarily).
ALTER TABLE addons
    MODIFY COLUMN category ENUM('module', 'system', 'widget', 'integration', 'plugin') NOT NULL;

-- Step 2: Update existing module rows to system.
UPDATE addons SET category = 'system' WHERE category = 'module';

-- Step 3: Remove 'module' from the ENUM.
ALTER TABLE addons
    MODIFY COLUMN category ENUM('system', 'widget', 'integration', 'plugin') NOT NULL;
