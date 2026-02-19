package smtp

import (
	"context"
	"database/sql"
	"fmt"
)

// SMTPRepository handles database operations for SMTP settings.
// This is a singleton table (id=1) so all operations target that row.
type SMTPRepository interface {
	// Get returns the SMTP settings row including encrypted password bytes.
	Get(ctx context.Context) (*smtpRow, error)

	// Upsert updates the SMTP settings. Uses INSERT ... ON DUPLICATE KEY UPDATE
	// to handle the singleton pattern.
	Upsert(ctx context.Context, row *smtpRow) error
}

// smtpRepository implements SMTPRepository with MariaDB.
type smtpRepository struct {
	db *sql.DB
}

// NewSMTPRepository creates a new SMTP repository.
func NewSMTPRepository(db *sql.DB) SMTPRepository {
	return &smtpRepository{db: db}
}

// Get retrieves the singleton SMTP settings row.
func (r *smtpRepository) Get(ctx context.Context) (*smtpRow, error) {
	row := &smtpRow{}
	err := r.db.QueryRowContext(ctx,
		`SELECT host, port, username, password_encrypted, from_address,
		        from_name, encryption, enabled, updated_at
		 FROM smtp_settings WHERE id = 1`,
	).Scan(
		&row.Host, &row.Port, &row.Username, &row.PasswordEncrypted,
		&row.FromAddress, &row.FromName, &row.Encryption, &row.Enabled,
		&row.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying smtp settings: %w", err)
	}
	return row, nil
}

// Upsert writes the SMTP settings to the singleton row.
func (r *smtpRepository) Upsert(ctx context.Context, row *smtpRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO smtp_settings (id, host, port, username, password_encrypted,
		                            from_address, from_name, encryption, enabled)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE
		     host = VALUES(host),
		     port = VALUES(port),
		     username = VALUES(username),
		     password_encrypted = VALUES(password_encrypted),
		     from_address = VALUES(from_address),
		     from_name = VALUES(from_name),
		     encryption = VALUES(encryption),
		     enabled = VALUES(enabled)`,
		row.Host, row.Port, row.Username, row.PasswordEncrypted,
		row.FromAddress, row.FromName, row.Encryption, row.Enabled,
	)
	if err != nil {
		return fmt.Errorf("upserting smtp settings: %w", err)
	}
	return nil
}
