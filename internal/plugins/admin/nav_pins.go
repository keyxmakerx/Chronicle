package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// nav_pins.go holds a site admin's own pinned admin pages. Pins belong to one
// admin (a column on their user row), so pinning never changes another
// admin's menu and the pins follow the admin across devices.

// MaxAdminNavPins bounds the pins: the sidebar block is a short list.
const MaxAdminNavPins = 6

// NormalizeAdminNavPins checks pins against the admin pages that may be
// pinned (allowed holds item links, never Home) and returns them in order
// without repeats. An unknown link or more than MaxAdminNavPins distinct pins
// is refused rather than trimmed, so a caller never believes a pin was kept
// that was not.
func NormalizeAdminNavPins(pins []string, allowed map[string]bool) ([]string, error) {
	out := make([]string, 0, len(pins))
	seen := make(map[string]bool, len(pins))
	for _, p := range pins {
		if !allowed[p] {
			return nil, apperror.NewBadRequest("that page cannot be pinned")
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) > MaxAdminNavPins {
		return nil, apperror.NewBadRequest(fmt.Sprintf("you can pin at most %d pages", MaxAdminNavPins))
	}
	return out, nil
}

// dropUnknownAdminNavPins is the read-side counterpart: a stored link whose
// page was later renamed or removed is skipped instead of failing the page.
func dropUnknownAdminNavPins(stored []string, allowed map[string]bool) []string {
	out := make([]string, 0, len(stored))
	seen := make(map[string]bool, len(stored))
	for _, p := range stored {
		if !allowed[p] || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
		if len(out) == MaxAdminNavPins {
			break
		}
	}
	return out
}

// AdminNavPinRepository stores each admin's pinned pages.
type AdminNavPinRepository interface {
	// GetAdminNavPins returns the stored links in pin order; nil when none.
	GetAdminNavPins(ctx context.Context, userID string) ([]string, error)
	// SetAdminNavPins replaces the stored links; an empty list clears them.
	SetAdminNavPins(ctx context.Context, userID string, pins []string) error
}

// AdminNavPinService owns the rules for an admin's pinned pages.
type AdminNavPinService interface {
	// Pins returns the admin's pins, with links that are no longer admin pages dropped.
	Pins(ctx context.Context, userID string) ([]string, error)
	// UpdatePins replaces the admin's pins with the full ordered list and
	// returns what was stored.
	UpdatePins(ctx context.Context, userID string, pins []string) ([]string, error)
}

type adminNavPinService struct {
	repo    AdminNavPinRepository
	allowed map[string]bool
}

// NewAdminNavPinService builds the service. pinnable lists the admin page
// links that may be pinned; the sidebar's own data supplies it so this
// package does not keep a second copy that could drift from the menu.
func NewAdminNavPinService(repo AdminNavPinRepository, pinnable []string) AdminNavPinService {
	allowed := make(map[string]bool, len(pinnable))
	for _, h := range pinnable {
		allowed[h] = true
	}
	return &adminNavPinService{repo: repo, allowed: allowed}
}

// Pins returns the admin's own pins.
func (s *adminNavPinService) Pins(ctx context.Context, userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	stored, err := s.repo.GetAdminNavPins(ctx, userID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return dropUnknownAdminNavPins(stored, s.allowed), nil
}

// UpdatePins stores the admin's own pins. The row written is the caller's own
// because the user ID comes from the session, never from the request.
func (s *adminNavPinService) UpdatePins(ctx context.Context, userID string, pins []string) ([]string, error) {
	if userID == "" {
		return nil, apperror.NewForbidden("sign in to pin pages")
	}
	clean, err := NormalizeAdminNavPins(pins, s.allowed)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SetAdminNavPins(ctx, userID, clean); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return clean, nil
}

type adminNavPinRepository struct{ db *sql.DB }

// NewAdminNavPinRepository builds the SQL-backed repository.
func NewAdminNavPinRepository(db *sql.DB) AdminNavPinRepository {
	return &adminNavPinRepository{db: db}
}

// GetAdminNavPins reads the admin's stored pins.
func (r *adminNavPinRepository) GetAdminNavPins(ctx context.Context, userID string) ([]string, error) {
	var raw sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT admin_nav_pins FROM users WHERE id = ?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading admin nav pins: %w", err)
	}
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var pins []string
	if err := json.Unmarshal([]byte(raw.String), &pins); err != nil {
		return nil, fmt.Errorf("decoding admin nav pins: %w", err)
	}
	return pins, nil
}

// SetAdminNavPins replaces the admin's stored pins. Only this one column is
// written, so no other user field can be touched by a pin change.
func (r *adminNavPinRepository) SetAdminNavPins(ctx context.Context, userID string, pins []string) error {
	var value any
	if len(pins) > 0 {
		b, err := json.Marshal(pins)
		if err != nil {
			return fmt.Errorf("encoding admin nav pins: %w", err)
		}
		value = string(b)
	}
	if _, err := r.db.ExecContext(ctx, `UPDATE users SET admin_nav_pins = ? WHERE id = ?`, value, userID); err != nil {
		return fmt.Errorf("updating admin nav pins: %w", err)
	}
	return nil
}
