-- Add per-entity popup preview configuration.
-- Controls what appears in the hover tooltip when an @mention is hovered.
-- Default NULL = show all available sections (image, attributes, entry excerpt).
ALTER TABLE entities ADD COLUMN popup_config JSON DEFAULT NULL AFTER field_overrides;
