package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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

	// Admin operations.
	ListUsers(ctx context.Context, offset, limit int) ([]User, int, error)
	UpdateIsAdmin(ctx context.Context, id string, isAdmin bool) error
	CountUsers(ctx context.Context) (int, error)
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
	                 is_admin, totp_secret, totp_enabled, created_at, last_login_at
	          FROM users WHERE id = ?`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.AvatarPath,
		&user.IsAdmin,
		&user.TOTPSecret,
		&user.TOTPEnabled,
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
	                 is_admin, totp_secret, totp_enabled, created_at, last_login_at
	          FROM users WHERE email = ?`

	user := &User{}
	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&user.PasswordHash,
		&user.AvatarPath,
		&user.IsAdmin,
		&user.TOTPSecret,
		&user.TOTPEnabled,
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

	query := `SELECT id, email, display_name, password_hash, avatar_path,
	                 is_admin, totp_secret, totp_enabled, created_at, last_login_at
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
			&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &u.AvatarPath,
			&u.IsAdmin, &u.TOTPSecret, &u.TOTPEnabled, &u.CreatedAt, &u.LastLoginAt,
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

// CountUsers returns the total number of registered users.
func (r *userRepository) CountUsers(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}
	return count, nil
}
