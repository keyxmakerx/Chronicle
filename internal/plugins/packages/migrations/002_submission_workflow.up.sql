-- Add submission/approval workflow and lifecycle management columns.
ALTER TABLE packages
    ADD COLUMN submitted_by    CHAR(36) NULL AFTER install_path,
    ADD COLUMN status          ENUM('pending', 'approved', 'rejected', 'archived', 'deprecated') NOT NULL DEFAULT 'approved' AFTER submitted_by,
    ADD COLUMN reviewed_by     CHAR(36) NULL AFTER status,
    ADD COLUMN reviewed_at     DATETIME NULL AFTER reviewed_by,
    ADD COLUMN review_note     TEXT NULL AFTER reviewed_at,
    ADD COLUMN deprecated_at   DATETIME NULL AFTER review_note,
    ADD COLUMN deprecation_msg VARCHAR(500) NULL AFTER deprecated_at;

CREATE INDEX idx_packages_status ON packages(status);
CREATE INDEX idx_packages_submitted_by ON packages(submitted_by);
