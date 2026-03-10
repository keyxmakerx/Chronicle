-- Add visibility_rules JSON column to map_markers for per-player visibility.
-- Follows the same pattern as timelines and calendar events.
-- NULL = no per-player rules (use base visibility field).

ALTER TABLE map_markers ADD COLUMN visibility_rules JSON DEFAULT NULL
    COMMENT 'Per-player visibility overrides: {"allowed_users":[],"denied_users":[]}';

ALTER TABLE map_drawings ADD COLUMN visibility_rules JSON DEFAULT NULL
    COMMENT 'Per-player visibility overrides: {"allowed_users":[],"denied_users":[]}';
