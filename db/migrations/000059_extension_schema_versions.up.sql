-- Sprint S-2: Schema version tracking for per-extension migrations (ADR-024).
-- Each extension can declare its own numbered SQL migrations. This table tracks
-- which versions have been applied, keyed by extension ID.

CREATE TABLE extension_schema_versions (
    extension_id CHAR(36) NOT NULL,
    version      INT NOT NULL,
    applied_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (extension_id, version),
    CONSTRAINT fk_esv_extension
      FOREIGN KEY (extension_id) REFERENCES extensions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
