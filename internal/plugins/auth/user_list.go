// user_list.go holds the admin people-list search: the filter vocabulary,
// the WHERE builder and the two queries behind the search box, filter chips
// and pagination. Kept apart from repository.go so the query shape can be
// tested without a database.

package auth

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/database"
)

// UserFilter narrows the admin people list to one chip's worth of users.
type UserFilter string

const (
	// UserFilterAll applies no role/status narrowing.
	UserFilterAll UserFilter = ""
	// UserFilterAdmins keeps only users with is_admin set.
	UserFilterAdmins UserFilter = "admins"
	// UserFilterDisabled keeps only disabled accounts.
	UserFilterDisabled UserFilter = "disabled"
)

// ParseUserFilter maps a raw query value to a known filter. Anything
// unrecognised means "all" so a stale or hand-edited URL still lists people.
func ParseUserFilter(raw string) UserFilter {
	switch UserFilter(raw) {
	case UserFilterAdmins:
		return UserFilterAdmins
	case UserFilterDisabled:
		return UserFilterDisabled
	default:
		return UserFilterAll
	}
}

// UserSearchOptions are the already-validated inputs of the people list.
type UserSearchOptions struct {
	// Query matches display name or email as a case-insensitive substring.
	Query   string
	Filter  UserFilter
	Offset  int
	PerPage int
}

// UserFilterCounts are the chip counts for one search text, each counting
// the users that chip would show.
type UserFilterCounts struct {
	All      int
	Admins   int
	Disabled int
}

// For returns the count a chip shows for the given filter.
func (c UserFilterCounts) For(f UserFilter) int {
	switch f {
	case UserFilterAdmins:
		return c.Admins
	case UserFilterDisabled:
		return c.Disabled
	default:
		return c.All
	}
}

// buildUserSearchWhere returns the WHERE clause (including the keyword, or
// empty) and its placeholder args. User text only ever travels as an arg;
// the clause itself is assembled from fixed fragments.
func buildUserSearchWhere(query string, filter UserFilter) (string, []any) {
	var clause string
	var args []any
	if query != "" {
		p := database.ContainsPattern(query)
		clause = fmt.Sprintf(" WHERE (display_name LIKE ? ESCAPE '%s' OR email LIKE ? ESCAPE '%s')",
			database.LikeEscapeChar, database.LikeEscapeChar)
		args = append(args, p, p)
	}
	var cond string
	switch filter {
	case UserFilterAdmins:
		cond = "is_admin = 1"
	case UserFilterDisabled:
		cond = "is_disabled = 1"
	}
	if cond != "" {
		if clause == "" {
			clause = " WHERE " + cond
		} else {
			clause += " AND " + cond
		}
	}
	return clause, args
}

// CountUserFilters returns the chip counts for a search text in a single
// aggregate query, so the page never pays one query per chip. The counts
// ignore the active chip on purpose: they show what each chip would list.
func (r *userRepository) CountUserFilters(ctx context.Context, query string) (UserFilterCounts, error) {
	where, args := buildUserSearchWhere(query, UserFilterAll)
	q := `SELECT COUNT(*), COALESCE(SUM(is_admin = 1), 0), COALESCE(SUM(is_disabled = 1), 0) FROM users` + where

	var c UserFilterCounts
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&c.All, &c.Admins, &c.Disabled); err != nil {
		return UserFilterCounts{}, fmt.Errorf("counting user filters: %w", err)
	}
	return c, nil
}

// SearchUsers returns one page of users matching the search text and chip,
// newest first. password_hash and totp_secret stay out of the column list:
// admin list views never need credential data.
func (r *userRepository) SearchUsers(ctx context.Context, opts UserSearchOptions) ([]User, error) {
	where, args := buildUserSearchWhere(opts.Query, opts.Filter)
	q := `SELECT id, email, display_name, avatar_path,
	             is_admin, is_disabled, totp_enabled, timezone,
	             created_at, last_login_at
	      FROM users` + where + ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, opts.PerPage, opts.Offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("searching users: %w", err)
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
			return nil, fmt.Errorf("scanning user row: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
