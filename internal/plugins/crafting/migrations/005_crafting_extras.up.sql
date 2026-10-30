-- 005_crafting_extras: a recipe's "If it goes wrong" and "Extras" settings,
-- what a project's item came out as, and how each roll's work went.
--
-- crafting_recipes.extras is JSON the service cleans on every write (an
-- empty string reads as nothing turned on). crafting_projects.details is
-- JSON too: what the item came out as (fine, marked, a quirk, made as
-- something else) and the standing costs of the substitutes it started
-- with. crafting_rolls.work is how the steps went on that roll ('' for rolls
-- made before it was kept), which mastery counts.
--
-- Idempotent: applies on a fresh database and a second time on top of itself.
ALTER TABLE crafting_recipes
    ADD COLUMN IF NOT EXISTS extras TEXT NULL;

ALTER TABLE crafting_projects
    ADD COLUMN IF NOT EXISTS details TEXT NULL;

ALTER TABLE crafting_rolls
    ADD COLUMN IF NOT EXISTS work VARCHAR(12) NOT NULL DEFAULT '';
