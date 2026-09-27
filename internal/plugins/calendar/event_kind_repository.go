// Package calendar - event_kind_repository.go persists the campaign's event
// kinds (calendar_event_kinds): one shared list per campaign, referenced by
// events across every calendar in it (see model.go's EventKind doc).
package calendar

import (
	"context"
	"database/sql"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// EventKindRepository defines persistence for a campaign's event kinds.
// Every write is scoped by campaignID in the SQL itself (not just checked
// after the fact), so a caller can never mutate another campaign's kind by
// guessing its numeric id.
type EventKindRepository interface {
	Create(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error)
	Update(ctx context.Context, id int, campaignID string, input EventKindInput) error
	Delete(ctx context.Context, id int, campaignID string) error
	GetByID(ctx context.Context, id int, campaignID string) (*EventKind, error)
	List(ctx context.Context, campaignID string) ([]EventKind, error)
}

// eventKindRepo is the MariaDB implementation of EventKindRepository.
type eventKindRepo struct {
	db *sql.DB
}

// NewEventKindRepository creates a new MariaDB-backed event kind repository.
func NewEventKindRepository(db *sql.DB) EventKindRepository {
	return &eventKindRepo{db: db}
}

// eventKindCols is the column list for event kind queries.
const eventKindCols = `id, campaign_id, slug, name, icon, color, sort_order, default_announced`

func scanEventKind(scanner interface{ Scan(...any) error }) (*EventKind, error) {
	var k EventKind
	err := scanner.Scan(&k.ID, &k.CampaignID, &k.Slug, &k.Name, &k.Icon, &k.Color, &k.SortOrder, &k.DefaultAnnounced)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

// isDuplicateEntry reports whether err is a MariaDB unique-constraint
// violation, the same driver-error-text check internal/widgets/tags'
// repository uses (the driver gives no typed error to match on instead).
func isDuplicateEntry(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Duplicate entry")
}

// validateEventKindInput rejects a DefaultAnnounced value that is neither
// AnnouncedAhead nor AnnouncedOnDay, the only two values EffectiveAnnounced
// understands; anything else would sit in the column looking valid while
// never actually taking effect for events of this kind.
func validateEventKindInput(input EventKindInput) error {
	if input.DefaultAnnounced != AnnouncedAhead && input.DefaultAnnounced != AnnouncedOnDay {
		return apperror.NewValidation("default_announced must be \"" + AnnouncedAhead + "\" or \"" + AnnouncedOnDay + "\"")
	}
	return nil
}

// Create inserts a new event kind and returns the persisted row. A slug
// collision within the campaign (the unique key is (campaign_id, slug), so
// a different campaign may reuse the same slug freely) is reported as a
// conflict rather than a raw driver error.
func (r *eventKindRepo) Create(ctx context.Context, campaignID string, input EventKindInput) (*EventKind, error) {
	if err := validateEventKindInput(input); err != nil {
		return nil, err
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO calendar_event_kinds (campaign_id, slug, name, icon, color, sort_order, default_announced)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		campaignID, input.Slug, input.Name, input.Icon, input.Color, input.SortOrder, input.DefaultAnnounced,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return nil, apperror.NewConflict("an event kind with this slug already exists in this campaign")
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, int(id), campaignID)
}

// Update modifies an existing kind. campaignID is part of the WHERE clause,
// not just an after-the-fact check: a caller from campaign B can never
// affect campaign A's row even if it guesses A's id. A production DSN
// without clientFoundRows counts changed rows, not matched rows, so saving
// back identical values also reports zero rows; an existence check tells
// that apart from a genuine not-found before returning an error.
func (r *eventKindRepo) Update(ctx context.Context, id int, campaignID string, input EventKindInput) error {
	if err := validateEventKindInput(input); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE calendar_event_kinds
		   SET slug = ?, name = ?, icon = ?, color = ?, sort_order = ?, default_announced = ?
		 WHERE id = ? AND campaign_id = ?`,
		input.Slug, input.Name, input.Icon, input.Color, input.SortOrder, input.DefaultAnnounced,
		id, campaignID,
	)
	if err != nil {
		if isDuplicateEntry(err) {
			return apperror.NewConflict("an event kind with this slug already exists in this campaign")
		}
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM calendar_event_kinds WHERE id = ? AND campaign_id = ?)`,
			id, campaignID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return apperror.NewNotFound("event kind not found")
		}
	}
	return nil
}

// Delete removes a kind. Events referencing it keep their row (kind_id is
// set to NULL by the FK, ON DELETE SET NULL) rather than being deleted.
func (r *eventKindRepo) Delete(ctx context.Context, id int, campaignID string) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM calendar_event_kinds WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return apperror.NewNotFound("event kind not found")
	}
	return nil
}

// GetByID returns one kind scoped to a campaign, or nil if it doesn't exist
// (in that campaign, or at all).
func (r *eventKindRepo) GetByID(ctx context.Context, id int, campaignID string) (*EventKind, error) {
	k, err := scanEventKind(r.db.QueryRowContext(ctx,
		`SELECT `+eventKindCols+` FROM calendar_event_kinds WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

// List returns every kind for a campaign, ordered by sort_order.
func (r *eventKindRepo) List(ctx context.Context, campaignID string) ([]EventKind, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eventKindCols+` FROM calendar_event_kinds WHERE campaign_id = ? ORDER BY sort_order, name`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kinds []EventKind
	for rows.Next() {
		k, err := scanEventKind(rows)
		if err != nil {
			return nil, err
		}
		kinds = append(kinds, *k)
	}
	return kinds, rows.Err()
}
