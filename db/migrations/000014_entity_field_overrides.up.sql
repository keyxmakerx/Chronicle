-- Per-entity field overrides allowing individual entities to customize
-- their attribute fields independently from the category template.
ALTER TABLE entities
    ADD COLUMN field_overrides JSON DEFAULT NULL AFTER fields_data;
