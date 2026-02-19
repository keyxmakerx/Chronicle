-- Migration: 000006_sidebar_config
-- Description: Adds sidebar_config JSON column to campaigns for per-campaign
--              sidebar customization (entity type ordering, visibility).

ALTER TABLE campaigns
    ADD COLUMN sidebar_config JSON NOT NULL DEFAULT ('{}') AFTER backdrop_path;
