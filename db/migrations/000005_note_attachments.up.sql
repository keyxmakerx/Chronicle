-- Note attachments for audio files and transcripts (Sprint V-5).
CREATE TABLE IF NOT EXISTS note_attachments (
    id            CHAR(26) NOT NULL,
    note_id       CHAR(26) NOT NULL,
    campaign_id   CHAR(26) NOT NULL,
    file_path     VARCHAR(512) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    mime_type     VARCHAR(100) NOT NULL,
    file_size     BIGINT NOT NULL DEFAULT 0,
    duration_secs INT DEFAULT NULL,
    transcript    LONGTEXT DEFAULT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (id),
    INDEX idx_note_attachments_note (note_id),
    INDEX idx_note_attachments_campaign (campaign_id),

    CONSTRAINT fk_note_attachments_note FOREIGN KEY (note_id) REFERENCES notes(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
