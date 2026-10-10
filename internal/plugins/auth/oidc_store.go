package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// oidcSettingsRow is the stored provider setting, secret still sealed.
type oidcSettingsRow struct {
	Enabled      bool
	ButtonName   string
	Issuer       string
	ClientID     string
	SealedSecret *string
	AllowSignup  bool
	HidePassword bool
	LastTestAt   *time.Time
	LastTestOK   bool
	LastTestNote string
}

// Identity is one provider account linked to a Chronicle account.
type Identity struct {
	Issuer    string
	Subject   string
	Email     string
	CreatedAt time.Time
}

// OIDCStore keeps the provider setting and the account links. It is a
// separate interface from UserRepository so only the provider code needs it.
type OIDCStore interface {
	GetOIDCSettings(ctx context.Context) (oidcSettingsRow, error)
	SaveOIDCSettings(ctx context.Context, row oidcSettingsRow) error
	RecordOIDCTest(ctx context.Context, ok bool, note string) error
	FindIdentity(ctx context.Context, issuer, subject string) (userID string, err error)
	LinkIdentity(ctx context.Context, userID, issuer, subject, email string) error
	TouchIdentity(ctx context.Context, issuer, subject string) error
	ListIdentities(ctx context.Context, userID string) ([]Identity, error)
	UnlinkIdentities(ctx context.Context, userID, issuer string) error
}

// NewOIDCStore returns the MariaDB store.
func NewOIDCStore(db *sql.DB) OIDCStore { return &oidcStore{db: db} }

type oidcStore struct{ db *sql.DB }

// GetOIDCSettings returns the setting, or a zero row when none was saved.
func (r *oidcStore) GetOIDCSettings(ctx context.Context) (oidcSettingsRow, error) {
	var row oidcSettingsRow
	var secret sql.NullString
	var tested sql.NullTime
	err := r.db.QueryRowContext(ctx,
		`SELECT enabled, button_name, issuer, client_id, client_secret_encrypted,
		        allow_signup, hide_password, last_test_at, last_test_ok, last_test_note
		   FROM oidc_settings WHERE id = 1`).Scan(
		&row.Enabled, &row.ButtonName, &row.Issuer, &row.ClientID, &secret,
		&row.AllowSignup, &row.HidePassword, &tested, &row.LastTestOK, &row.LastTestNote)
	if errors.Is(err, sql.ErrNoRows) {
		return oidcSettingsRow{}, nil
	}
	if err != nil {
		return oidcSettingsRow{}, fmt.Errorf("reading sign-in provider: %w", err)
	}
	if secret.Valid {
		row.SealedSecret = &secret.String
	}
	if tested.Valid {
		row.LastTestAt = &tested.Time
	}
	return row, nil
}

// SaveOIDCSettings writes the whole setting and forgets the last test,
// because a test of the old setting says nothing about the new one.
func (r *oidcStore) SaveOIDCSettings(ctx context.Context, row oidcSettingsRow) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO oidc_settings (id, enabled, button_name, issuer, client_id, client_secret_encrypted,
		                            allow_signup, hide_password, last_test_at, last_test_ok, last_test_note)
		 VALUES (1, ?, ?, ?, ?, ?, ?, ?, NULL, FALSE, '')
		 ON DUPLICATE KEY UPDATE enabled = VALUES(enabled), button_name = VALUES(button_name),
		        issuer = VALUES(issuer), client_id = VALUES(client_id),
		        client_secret_encrypted = VALUES(client_secret_encrypted),
		        allow_signup = VALUES(allow_signup), hide_password = VALUES(hide_password),
		        last_test_at = NULL, last_test_ok = FALSE, last_test_note = ''`,
		row.Enabled, row.ButtonName, row.Issuer, row.ClientID, row.SealedSecret, row.AllowSignup, row.HidePassword)
	if err != nil {
		return fmt.Errorf("saving sign-in provider: %w", err)
	}
	return nil
}

// RecordOIDCTest stores the outcome of the admin's last test sign-in.
func (r *oidcStore) RecordOIDCTest(ctx context.Context, ok bool, note string) error {
	if len(note) > 255 {
		note = note[:255]
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE oidc_settings SET last_test_at = UTC_TIMESTAMP(), last_test_ok = ?, last_test_note = ? WHERE id = 1`,
		ok, note)
	if err != nil {
		return fmt.Errorf("recording provider test: %w", err)
	}
	return nil
}

// FindIdentity returns the Chronicle account a provider account is linked
// to, or a NotFound error.
func (r *oidcStore) FindIdentity(ctx context.Context, issuer, subject string) (string, error) {
	var userID string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id FROM user_identities WHERE issuer = ? AND subject = ?`, issuer, subject).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", apperror.NewNotFound("no linked account")
	}
	if err != nil {
		return "", fmt.Errorf("finding linked account: %w", err)
	}
	return userID, nil
}

// LinkIdentity links a provider account. A provider account already linked
// to someone else is a Conflict, never a silent move.
func (r *oidcStore) LinkIdentity(ctx context.Context, userID, issuer, subject, email string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, issuer, subject, email, last_used_at)
		 VALUES (?, ?, ?, ?, UTC_TIMESTAMP())`, userID, issuer, subject, email)
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) && myErr.Number == 1062 {
		return apperror.NewConflict("that account is already linked to another Chronicle account")
	}
	if err != nil {
		return fmt.Errorf("linking account: %w", err)
	}
	return nil
}

// TouchIdentity records a sign-in through a link.
func (r *oidcStore) TouchIdentity(ctx context.Context, issuer, subject string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE user_identities SET last_used_at = UTC_TIMESTAMP() WHERE issuer = ? AND subject = ?`, issuer, subject)
	return err
}

// ListIdentities lists a person's links, oldest first.
func (r *oidcStore) ListIdentities(ctx context.Context, userID string) ([]Identity, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT issuer, subject, email, created_at FROM user_identities WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("listing linked accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Identity
	for rows.Next() {
		var id Identity
		if err := rows.Scan(&id.Issuer, &id.Subject, &id.Email, &id.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning linked account: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UnlinkIdentities removes a person's links to one provider.
func (r *oidcStore) UnlinkIdentities(ctx context.Context, userID, issuer string) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM user_identities WHERE user_id = ? AND issuer = ?`, userID, issuer); err != nil {
		return fmt.Errorf("unlinking account: %w", err)
	}
	return nil
}
