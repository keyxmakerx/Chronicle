-- Add updated_at to map_layers and map_fog so they can participate in
-- the optimistic-concurrency pattern used by the rest of the maps plugin
-- (markers, drawings, tokens). The DEFAULT CURRENT_TIMESTAMP / ON UPDATE
-- CURRENT_TIMESTAMP triple matches the convention used on the other
-- map tables in migration 001.
ALTER TABLE map_layers
    ADD COLUMN updated_at TIMESTAMP NOT NULL
        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
        AFTER created_at;

ALTER TABLE map_fog
    ADD COLUMN updated_at TIMESTAMP NOT NULL
        DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
        AFTER created_at;
