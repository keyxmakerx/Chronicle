package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// UserRepository defines the data access contract for user operations.
// All SQL lives in the concrete implementation -- no SQL leaks out.
type UserRepository interface {
	Create(ctx context.Context, user *User) error
	FindByID(ctx context.Context, id string) (*User, error)
	FindByEmail(ctx context.Context, email string) (*User, error)
	EmailExists(ctx context.Context, email string) (bool, error)
	UpdateLastLogin(ctx context.Context, id string) error

	// Password reset.
	UpdatePassword(ctx context.Context, userID, passwordHash string) error
	CreateResetToken(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error
	FindResetToken(ctx context.Context, tokenHash string) (userID, email string, expiresAt time.Time, usedAt *time.Time, err error)
	MarkResetTokenUsed(ctx context.Context, tokenHash string) error

	// User profile.
	UpdateTimezone(ctx context.Context, userID, timezone string) error
	UpdateDisplayName(ctx context.Context, userID, displayName string) error
	UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error

	// View preferences: the person's own look, as the stored JSON (nil when
	// they have chosen nothing).
	GetViewPrefs(ctx context.Context, userID string) ([]byte, error)
	SetViewPrefs(ctx context.Context, userID string, prefs []byte) error
	GetNotifyPrefs(ctx context.Context, userID string) ([]byte, error)
	SetNotifyPrefs(ctx context.Context, userID string, prefs []byte) error
	// ListNotifyPrefs returns the stored notify_prefs JSON of each listed
	// user that has any; users with none are absent from the map.
	ListNotifyPrefs(ctx context.Context, userIDs []string) (map[string][]byte, error)
	// AnonymizeUser empties a deleted account's row: personal fields
	// cleared, a placeholder email and name, an unusable password,
	// disabled and stamped deleted_at. Its reset tokens are removed.
	AnonymizeUser(ctx context.Context, userID, email, displayName, passwordHash string) error

	// ListLegacyAvatarPaths returns userID -> avatar_path for every user
	// whose avatar_path still starts with prefix. Used only by the boot
	// reconciler (reconcile_avatar_paths.go) to find rows still pointing at
	// a dead legacy web path; ordinary reads use FindByID/FindByEmail.
	ListLegacyAvatarPaths(ctx context.Context, prefix string) (map[string]string, error)

	// ClearAvatarPathIfMatches sets avatar_path to NULL only if it still
	// equals expectedPath, reporting whether it actually cleared the row.
	// Used by the boot reconciler so a legacy row a user has already
	// overwritten with a real upload between listing and this write is
	// never clobbered back to NULL.
	ClearAvatarPathIfMatches(ctx context.Context, userID, expectedPath string) (bool, error)

	// Email change verification.
	SetPendingEmail(ctx context.Context, userID, pendingEmail, tokenHash string, expiresAt time.Time) error
	FindByEmailVerifyToken(ctx context.Context, tokenHash string) (userID, pendingEmail string, expiresAt time.Time, err error)
	ConfirmEmailChange(ctx context.Context, userID, newEmail string) error

	// Admin operations.
	ListUsers(ctx context.Context, offset, limit int) ([]User, int, error)
	SearchUsers(ctx context.Context, opts UserSearchOptions) ([]User, error)
	CountUserFilters(ctx context.Context, query string) (UserFilterCounts, error)
	UpdateIsAdmin(ctx context.Context, id string, isAdmin bool) error
	UpdateIsDisabled(ctx context.Context, id string, isDisabled bool) error
	CountUsers(ctx context.Context) (int, error)
	CountAdmins(ctx context.Context) (int, error)

	// Two-factor sign-in. The secret arrives already encrypted; nil clears it.
	SetTOTP(ctx context.Context, userID string, encryptedSecret *string, enabled bool) error
	ReplaceRecoveryCodes(ctx context.Context, userID string, hashes []string) error
	UseRecoveryCode(ctx context.Context, userID, hash string) (bool, error)
	CountRecoveryCodes(ctx context.Context, userID string) (int, error)
}

// userRepository implements UserRepository with hand-written MariaDB queries.
type userRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new user repository backed by the given DB pool.
func NewUserRepository(db *sql.DB) UserRepository {
	return &userRepository{db: db}
}

// Create inserts a new user row into the users table.
func (r *userRepository) Create(ctx context.Context, user *User) error {
	query := `INSERT INTO users (id, email, display_name, password_hash, is_admin, created_at)
	          VALUES (?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		user.ID,
		user.Email,
		user.DisplayName,
		user.PasswordHash,
		user.IsAdmin,
		user.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("inserting user: %w", err)
	}

	return nil
}

// FindByID retrieves a user by their UUID.
// Returns apperror.NotFound if no user exists with this ID.
func (r *userRepository) FindByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT id, email, display_name, password_hash, avatar_path,
	                 is_admin, is_disabled, totp_secret, totp_enabled, timezone,
	                 created_at, last_login_at
	          FROM users WHERE id = ?`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.AvatarPath,
		&user.IsAdmin,
		&user.IsDisabled,
		&user.TOTPSecret,
		&user.TOTPEnabled,
		&user.Timezone,
		&user.CreatedAt,
		&user.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}

	return user, nil
}

// FindByEmail retrieves a user by their email address.
// Returns apperror.NotFound if no user exists with this email.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, display_name, password_hash, avatar_path,
	                 is_admin, is_disabled, totp_secret, totp_enabled, timezone,
	                 created_at, last_login_at
	          FROM users WHERE email = ?`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.AvatarPath,
		&user.IsAdmin,
		&user.IsDisabled,
		&user.TOTPSecret,
		&user.TOTPEnabled,
		&user.Timezone,
		&user.CreatedAt,
		&user.LastLoginAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}

	return user, nil
}

// EmailExists returns true if a user with the given email already exists.
// Used during registration to check for duplicates before hashing the password.
func (r *userRepository) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)`

	var exists bool
	err := r.db.QueryRowContext(ctx, query, email).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking email existence: %w", err)
	}

	return exists, nil
}

// UpdateLastLogin sets the last_login_at timestamp to now for the given user.
func (r *userRepository) UpdateLastLogin(ctx context.Context, id string) error {
	query := `UPDATE users SET last_login_at = NOW() WHERE id = ?`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("updating last login: %w", err)
	}

	return nil
}

// --- Admin Operations ---

// ListUsers returns a paginated list of all users ordered by creation date.
// Also returns the total count for pagination.
func (r *userRepository) ListUsers(ctx context.Context, offset, limit int) ([]User, int, error) {
	// Get total count.
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting users: %w", err)
	}

	// Deliberately exclude password_hash and totp_secret from this query.
	// Admin list views don't need sensitive credential data.
	query := `SELECT id, email, display_name, avatar_path,
	                 is_admin, is_disabled, totp_enabled, timezone,
	                 created_at, last_login_at
	          FROM users ORDER BY created_at DESC LIMIT ? OFFSET ?`

	rows, err := r.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.Email, &u.DisplayName, &u.AvatarPath,
			&u.IsAdmin, &u.IsDisabled, &u.TOTPEnabled, &u.Timezone,
			&u.CreatedAt, &u.LastLoginAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}

	return users, total, rows.Err()
}

// UpdateIsAdmin sets or clears the is_admin flag for a user.
func (r *userRepository) UpdateIsAdmin(ctx context.Context, id string, isAdmin bool) error {
	query := `UPDATE users SET is_admin = ? WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, isAdmin, id)
	if err != nil {
		return fmt.Errorf("updating is_admin: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}

	return nil
}

// UpdateIsDisabled sets or clears the is_disabled flag for a user. Disabled
// users cannot log in and their active sessions are invalidated separately.
// A deleted account stays disabled: enabling it would list an empty row as
// an ordinary user.
func (r *userRepository) UpdateIsDisabled(ctx context.Context, id string, isDisabled bool) error {
	query := `UPDATE users SET is_disabled = ? WHERE id = ? AND (? OR deleted_at IS NULL)`

	result, err := r.db.ExecContext(ctx, query, isDisabled, id, isDisabled)
	if err != nil {
		return fmt.Errorf("updating is_disabled: %w", err)
	}

	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found or the account was deleted")
	}

	return nil
}

// CountUsers returns the total number of registered users.
func (r *userRepository) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}

// CountAdmins returns the number of admins who can still sign in. Used to
// prevent removing the last admin; a disabled admin can't stand in for one.
func (r *userRepository) CountAdmins(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = true AND is_disabled = FALSE`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting admins: %w", err)
	}
	return count, nil
}

// --- User Profile ---

// UpdateTimezone sets the IANA timezone for a user. Empty string sets NULL.
func (r *userRepository) UpdateTimezone(ctx context.Context, userID, timezone string) error {
	var tz interface{} = timezone
	if timezone == "" {
		tz = nil
	}
	query := `UPDATE users SET timezone = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, tz, userID)
	if err != nil {
		return fmt.Errorf("updating timezone: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// GetViewPrefs returns the raw view_prefs JSON for a user, or nil when unset.
func (r *userRepository) GetViewPrefs(ctx context.Context, userID string) ([]byte, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT view_prefs FROM users WHERE id = ?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading view prefs: %w", err)
	}
	return raw, nil
}

// SetViewPrefs stores the validated view_prefs JSON for a user.
func (r *userRepository) SetViewPrefs(ctx context.Context, userID string, prefs []byte) error {
	// RowsAffected is 0 both for a missing user and for an unchanged value
	// (MySQL reports changed rows), so existence is not inferred from it.
	if _, err := r.db.ExecContext(ctx, `UPDATE users SET view_prefs = ? WHERE id = ?`, string(prefs), userID); err != nil {
		return fmt.Errorf("updating view prefs: %w", err)
	}
	return nil
}

// GetNotifyPrefs returns the raw notify_prefs JSON for a user, or nil when unset.
func (r *userRepository) GetNotifyPrefs(ctx context.Context, userID string) ([]byte, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT notify_prefs FROM users WHERE id = ?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("reading notify prefs: %w", err)
	}
	return raw, nil
}

// SetNotifyPrefs stores the validated notify_prefs JSON for a user.
func (r *userRepository) SetNotifyPrefs(ctx context.Context, userID string, prefs []byte) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE users SET notify_prefs = ? WHERE id = ?`, string(prefs), userID); err != nil {
		return fmt.Errorf("updating notify prefs: %w", err)
	}
	return nil
}

// ListNotifyPrefs reads notify_prefs for many users in one query.
func (r *userRepository) ListNotifyPrefs(ctx context.Context, userIDs []string) (map[string][]byte, error) {
	out := map[string][]byte{}
	if len(userIDs) == 0 {
		return out, nil
	}
	ph := strings.TrimSuffix(strings.Repeat("?,", len(userIDs)), ",")
	args := make([]any, len(userIDs))
	for i, id := range userIDs {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, notify_prefs FROM users WHERE notify_prefs IS NOT NULL AND id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing notify prefs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, fmt.Errorf("scanning notify prefs: %w", err)
		}
		out[id] = raw
	}
	return out, rows.Err()
}

// AnonymizeUser empties a deleted account's row in one statement, then
// removes its password reset tokens.
func (r *userRepository) AnonymizeUser(ctx context.Context, userID, email, displayName, passwordHash string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET email = ?, display_name = ?, password_hash = ?,
		        avatar_path = NULL, is_admin = FALSE, totp_secret = NULL, totp_enabled = FALSE,
		        timezone = NULL, pending_email = NULL, email_verify_token = NULL, email_verify_expires = NULL,
		        admin_nav_pins = NULL, view_prefs = NULL, notify_prefs = NULL,
		        is_disabled = TRUE, deleted_at = NOW()
		  WHERE id = ? AND deleted_at IS NULL`,
		email, displayName, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("anonymizing user: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("user not found")
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM password_reset_tokens WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("removing reset tokens: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("removing recovery codes: %w", err)
	}
	return nil
}

// --- Two-factor ---

// SetTOTP stores the encrypted authenticator secret and whether two-factor
// is on. Turning it off passes nil, which clears the secret.
func (r *userRepository) SetTOTP(ctx context.Context, userID string, encryptedSecret *string, enabled bool) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, totp_enabled = ? WHERE id = ?`,
		encryptedSecret, enabled, userID)
	if err != nil {
		return fmt.Errorf("updating two-factor: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// ReplaceRecoveryCodes swaps a person's recovery codes for a new set in one
// transaction, so a failure never leaves them with half a set. An empty
// list just removes them.
func (r *userRepository) ReplaceRecoveryCodes(ctx context.Context, userID string, hashes []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_recovery_codes WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("removing recovery codes: %w", err)
	}
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_recovery_codes (user_id, code_hash) VALUES (?, ?)`, userID, h); err != nil {
			return fmt.Errorf("storing recovery code: %w", err)
		}
	}
	return tx.Commit()
}

// UseRecoveryCode marks an unused code as used and reports whether it
// matched. The used_at guard in the UPDATE makes each code single-use even
// when two sign-ins race.
func (r *userRepository) UseRecoveryCode(ctx context.Context, userID, hash string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE user_recovery_codes SET used_at = NOW()
		  WHERE user_id = ? AND code_hash = ? AND used_at IS NULL`, userID, hash)
	if err != nil {
		return false, fmt.Errorf("using recovery code: %w", err)
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// CountRecoveryCodes returns how many unused recovery codes a person has.
func (r *userRepository) CountRecoveryCodes(ctx context.Context, userID string) (int, error) {
	var n int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_recovery_codes WHERE user_id = ? AND used_at IS NULL`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("counting recovery codes: %w", err)
	}
	return n, nil
}

// UpdateDisplayName sets the display name for a user.
func (r *userRepository) UpdateDisplayName(ctx context.Context, userID, displayName string) error {
	query := `UPDATE users SET display_name = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, displayName, userID)
	if err != nil {
		return fmt.Errorf("updating display name: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// UpdateAvatarPath sets or clears the user's avatar image path. Idempotent:
// MariaDB reports zero rows affected when the new value equals the one
// already stored (e.g. clearing an avatar_path that is already NULL), which
// is not a missing user -- an existence check on that path tells the two
// apart so clearing an already-cleared avatar stays a success, not a 404.
func (r *userRepository) UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error {
	query := `UPDATE users SET avatar_path = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, avatarPath, userID)
	if err != nil {
		return fmt.Errorf("updating avatar path: %w", err)
	}
	n, _ := result.RowsAffected()
	if n > 0 {
		return nil
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("checking user existence after avatar path update: %w", err)
	}
	if !exists {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// ClearAvatarPathIfMatches clears avatar_path to NULL only if it still
// equals expectedPath, so a concurrent write (a user uploading a real
// avatar between the reconciler's listing and this call) is never undone.
func (r *userRepository) ClearAvatarPathIfMatches(ctx context.Context, userID, expectedPath string) (bool, error) {
	query := `UPDATE users SET avatar_path = NULL WHERE id = ? AND avatar_path = ?`
	result, err := r.db.ExecContext(ctx, query, userID, expectedPath)
	if err != nil {
		return false, fmt.Errorf("clearing avatar path if matches: %w", err)
	}
	n, _ := result.RowsAffected()
	return n > 0, nil
}

// ListLegacyAvatarPaths returns userID -> avatar_path for every row whose
// avatar_path starts with prefix. No wildcard characters are expected in
// prefix (it is always a compile-time constant), so no LIKE-escaping is done.
func (r *userRepository) ListLegacyAvatarPaths(ctx context.Context, prefix string) (map[string]string, error) {
	query := `SELECT id, avatar_path FROM users WHERE avatar_path LIKE ?`
	rows, err := r.db.QueryContext(ctx, query, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("listing legacy avatar paths: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id string
		var avatarPath sql.NullString
		if err := rows.Scan(&id, &avatarPath); err != nil {
			return nil, fmt.Errorf("scanning legacy avatar path row: %w", err)
		}
		if avatarPath.Valid {
			result[id] = avatarPath.String
		}
	}
	return result, rows.Err()
}

// --- Password Reset ---

// UpdatePassword sets a new password hash for a user.
func (r *userRepository) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	query := `UPDATE users SET password_hash = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// CreateResetToken inserts a new password reset token. The tokenHash is
// SHA-256(plaintext_token) — plaintext is never stored.
func (r *userRepository) CreateResetToken(ctx context.Context, userID, email, tokenHash string, expiresAt time.Time) error {
	query := `INSERT INTO password_reset_tokens (user_id, email, token_hash, expires_at)
	          VALUES (?, ?, ?, ?)`
	_, err := r.db.ExecContext(ctx, query, userID, email, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("creating reset token: %w", err)
	}
	return nil
}

// FindResetToken looks up a reset token by its hash. Returns the associated
// user ID, email, expiry, and used_at (nil if unused).
func (r *userRepository) FindResetToken(ctx context.Context, tokenHash string) (string, string, time.Time, *time.Time, error) {
	query := `SELECT user_id, email, expires_at, used_at
	          FROM password_reset_tokens WHERE token_hash = ?`
	var userID, email string
	var expiresAt time.Time
	var usedAt *time.Time
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(&userID, &email, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, nil, apperror.NewNotFound("invalid or expired reset token")
	}
	if err != nil {
		return "", "", time.Time{}, nil, fmt.Errorf("finding reset token: %w", err)
	}
	return userID, email, expiresAt, usedAt, nil
}

// MarkResetTokenUsed stamps the used_at column so the token can't be reused.
func (r *userRepository) MarkResetTokenUsed(ctx context.Context, tokenHash string) error {
	query := `UPDATE password_reset_tokens SET used_at = NOW() WHERE token_hash = ?`
	_, err := r.db.ExecContext(ctx, query, tokenHash)
	if err != nil {
		return fmt.Errorf("marking reset token used: %w", err)
	}
	return nil
}

// --- Email Change Verification ---

// SetPendingEmail stores a pending email change request with a verification token.
func (r *userRepository) SetPendingEmail(ctx context.Context, userID, pendingEmail, tokenHash string, expiresAt time.Time) error {
	query := `UPDATE users SET pending_email = ?, email_verify_token = ?, email_verify_expires = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, pendingEmail, tokenHash, expiresAt, userID)
	if err != nil {
		return fmt.Errorf("setting pending email: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}

// FindByEmailVerifyToken looks up a user by their email verification token hash.
// Returns the user ID, pending email, and token expiry.
func (r *userRepository) FindByEmailVerifyToken(ctx context.Context, tokenHash string) (string, string, time.Time, error) {
	query := `SELECT id, pending_email, email_verify_expires FROM users WHERE email_verify_token = ?`
	var userID string
	var pendingEmail string
	var expiresAt time.Time
	err := r.db.QueryRowContext(ctx, query, tokenHash).Scan(&userID, &pendingEmail, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", time.Time{}, apperror.NewNotFound("invalid or expired verification link")
	}
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("finding email verify token: %w", err)
	}
	return userID, pendingEmail, expiresAt, nil
}

// ConfirmEmailChange atomically updates the user's email and clears the pending
// email verification fields.
func (r *userRepository) ConfirmEmailChange(ctx context.Context, userID, newEmail string) error {
	query := `UPDATE users SET email = ?, pending_email = NULL, email_verify_token = NULL, email_verify_expires = NULL WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, newEmail, userID)
	if err != nil {
		return fmt.Errorf("confirming email change: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return apperror.NewNotFound("user not found")
	}
	return nil
}
