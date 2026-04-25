-- Restore the previous 'drawsteel' default on bestiary_publications.system_id.
ALTER TABLE bestiary_publications
    MODIFY COLUMN system_id VARCHAR(100) NOT NULL DEFAULT 'drawsteel';
