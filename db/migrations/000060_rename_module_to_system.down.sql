-- Revert addon category ENUM from 'system' back to 'module'.
ALTER TABLE addons
    MODIFY COLUMN category ENUM('module', 'system', 'widget', 'integration', 'plugin') NOT NULL;

UPDATE addons SET category = 'module' WHERE category = 'system';

ALTER TABLE addons
    MODIFY COLUMN category ENUM('module', 'widget', 'integration', 'plugin') NOT NULL;
