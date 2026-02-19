-- Rollback: 000006_sidebar_config

ALTER TABLE campaigns DROP COLUMN IF EXISTS sidebar_config;
