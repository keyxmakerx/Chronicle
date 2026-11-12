-- 002_design_gallery: the server gallery, where an Owner publishes a snapshot of
-- a library design for every other Owner on this instance.
--
-- An entry is its own copy of the design (name, note, data), so later edits to
-- the library design change nothing here until the publisher updates it.
-- publisher_user_id and source_design_id carry no foreign key on purpose: an
-- entry must outlive a deleted library design, and the moderation record must
-- outlive an account.
--
-- status is visible, pending, rejected or removed; only visible entries are
-- shown to anyone but their publisher and the instance admins. reasons holds
-- machine-readable codes as a JSON array.
CREATE TABLE IF NOT EXISTS design_gallery (
    id                CHAR(36)     NOT NULL,
    publisher_user_id CHAR(36)     NOT NULL,
    source_design_id  CHAR(36)     NULL,
    kind              VARCHAR(40)  NOT NULL,
    system_id         VARCHAR(255) NOT NULL DEFAULT '',
    name              VARCHAR(60)  NOT NULL,
    note              VARCHAR(140) NOT NULL DEFAULT '',
    maker_name        VARCHAR(100) NOT NULL DEFAULT '',
    data              LONGTEXT     NOT NULL,
    data_hash         CHAR(64)     NOT NULL,
    status            VARCHAR(10)  NOT NULL DEFAULT 'pending',
    reasons           VARCHAR(255) NOT NULL DEFAULT '[]',
    reviewed_by       CHAR(36)     NULL,
    reviewed_at       DATETIME     NULL,
    published_at      DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_design_gallery_source (publisher_user_id, source_design_id),
    INDEX idx_design_gallery_browse (status, system_id, published_at),
    INDEX idx_design_gallery_hash (publisher_user_id, data_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- One report per reporter per entry; three distinct reports send a visible
-- entry back to review.
CREATE TABLE IF NOT EXISTS design_gallery_reports (
    entry_id         CHAR(36)     NOT NULL,
    reporter_user_id CHAR(36)     NOT NULL,
    reason           VARCHAR(10)  NOT NULL,
    note             VARCHAR(140) NOT NULL DEFAULT '',
    created_at       DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entry_id, reporter_user_id),
    CONSTRAINT fk_design_gallery_reports_entry
        FOREIGN KEY (entry_id) REFERENCES design_gallery(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Every publish and update attempt, for the 24-hour rate rule. It is separate
-- from design_gallery because unpublishing deletes the entry but must not
-- reset the count.
CREATE TABLE IF NOT EXISTS design_gallery_events (
    id         BIGINT      NOT NULL AUTO_INCREMENT,
    user_id    CHAR(36)    NOT NULL,
    created_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    INDEX idx_design_gallery_events_user (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
