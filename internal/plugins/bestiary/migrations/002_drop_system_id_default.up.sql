-- Drop the hard-coded 'drawsteel' default on bestiary_publications.system_id.
-- Bestiary publications are system-dependent, and the system tag must come
-- from the publishing context (the source campaign's selected game system),
-- not from a default that silently mis-tags publications when the assumed
-- system isn't installed. Existing rows with system_id='drawsteel' are not
-- modified — that's user data and the user may legitimately have meant it.

ALTER TABLE bestiary_publications
    MODIFY COLUMN system_id VARCHAR(100) NOT NULL DEFAULT '';
