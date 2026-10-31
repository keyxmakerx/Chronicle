-- Reverses 005_crafting_extras. IF EXISTS so a second run is harmless.
ALTER TABLE crafting_benches DROP COLUMN IF EXISTS upgrades;
ALTER TABLE crafting_rolls DROP COLUMN IF EXISTS work;
ALTER TABLE crafting_projects DROP COLUMN IF EXISTS details;
ALTER TABLE crafting_recipes DROP COLUMN IF EXISTS extras;
