-- Revert submission/approval workflow columns.
DROP INDEX idx_packages_submitted_by ON packages;
DROP INDEX idx_packages_status ON packages;

ALTER TABLE packages
    DROP COLUMN deprecation_msg,
    DROP COLUMN deprecated_at,
    DROP COLUMN review_note,
    DROP COLUMN reviewed_at,
    DROP COLUMN reviewed_by,
    DROP COLUMN status,
    DROP COLUMN submitted_by;
