ALTER TABLE bestiary_publications
    DROP COLUMN IF EXISTS prior_visibility,
    DROP COLUMN IF EXISTS legacy_escaped_text;
