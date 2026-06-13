-- Revert the 'public' grant subject. Any rows using it must be removed first,
-- else MODIFY would coerce them to '' and corrupt the column.
DELETE FROM entity_permissions WHERE subject_type = 'public';
DELETE FROM tag_permissions WHERE subject_type = 'public';

ALTER TABLE entity_permissions
    MODIFY COLUMN subject_type ENUM('role','user','group') NOT NULL;

ALTER TABLE tag_permissions
    MODIFY COLUMN subject_type ENUM('role','user','group') NOT NULL;
