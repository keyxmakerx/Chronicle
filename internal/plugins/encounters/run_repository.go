// Hand-written SQL for the encounter_runs table.
//
// SECURITY: the campaign id is the key, so a run is only ever read, changed
// or ended for the campaign the route resolved.
package encounters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RunRow is a stored run: its JSON document and edit version.
type RunRow struct {
	Data    []byte
	Version int
}

// RunRepository is the persistence boundary for runs.
type RunRepository interface {
	// GetRun returns the campaign's run; found is false when none is on.
	GetRun(ctx context.Context, campaignID string) (r RunRow, found bool, err error)
	// CreateRun stores a new run at version 1. ok is false when the
	// campaign already has one.
	CreateRun(ctx context.Context, campaignID string, data []byte, userID string) (ok bool, err error)
	// UpdateRun writes the run if the stored version is still expected and
	// bumps it. ok is false on a lost race or when the run has ended.
	UpdateRun(ctx context.Context, campaignID string, data []byte, expected int, userID string) (ok bool, err error)
	// EndRun deletes the campaign's run and returns it as it was. ended is
	// false when none was on, so of two ends at once only one gets the run;
	// ending none is not an error.
	EndRun(ctx context.Context, campaignID string) (r RunRow, ended bool, err error)
}

type sqlRunRepository struct{ db *sql.DB }

// NewRunRepository creates a MariaDB-backed run repository.
func NewRunRepository(db *sql.DB) RunRepository { return &sqlRunRepository{db: db} }

func (s *sqlRunRepository) GetRun(ctx context.Context, campaignID string) (RunRow, bool, error) {
	var r RunRow
	var data string
	err := s.db.QueryRowContext(ctx,
		`SELECT data, version FROM encounter_runs WHERE campaign_id = ?`, campaignID).Scan(&data, &r.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return RunRow{}, false, nil
	}
	if err != nil {
		return RunRow{}, false, fmt.Errorf("get encounter run: %w", err)
	}
	r.Data = []byte(data)
	return r, true, nil
}

func (s *sqlRunRepository) CreateRun(ctx context.Context, campaignID string, data []byte, userID string) (bool, error) {
	// INSERT IGNORE turns the primary key clash of a second start into
	// "not created" instead of an error, so two DMs pressing Start at once
	// get one fight.
	res, err := s.db.ExecContext(ctx,
		`INSERT IGNORE INTO encounter_runs (campaign_id, data, version, updated_by) VALUES (?, ?, 1, ?)`,
		campaignID, string(data), nullable(userID))
	if err != nil {
		return false, fmt.Errorf("insert encounter run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert encounter run: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRunRepository) UpdateRun(ctx context.Context, campaignID string, data []byte, expected int, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE encounter_runs SET data = ?, version = version + 1, updated_by = ?
		 WHERE campaign_id = ? AND version = ?`,
		string(data), nullable(userID), campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("update encounter run: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update encounter run: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRunRepository) EndRun(ctx context.Context, campaignID string) (RunRow, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RunRow{}, false, fmt.Errorf("end encounter run: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var r RunRow
	var data string
	err = tx.QueryRowContext(ctx,
		`SELECT data, version FROM encounter_runs WHERE campaign_id = ? FOR UPDATE`, campaignID).Scan(&data, &r.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return RunRow{}, false, nil
	}
	if err != nil {
		return RunRow{}, false, fmt.Errorf("end encounter run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM encounter_runs WHERE campaign_id = ?`, campaignID); err != nil {
		return RunRow{}, false, fmt.Errorf("end encounter run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return RunRow{}, false, fmt.Errorf("end encounter run: %w", err)
	}
	r.Data = []byte(data)
	return r, true, nil
}

// ShownRepository keeps what scenes showed to players, for the players'
// Table and the night's record.
type ShownRepository interface {
	// AddShown records one shown item.
	AddShown(ctx context.Context, campaignID string, item ShownItem, userID string) error
	// ListShown returns the campaign's items shown since, oldest first, at
	// most limit of the newest.
	ListShown(ctx context.Context, campaignID string, since time.Time, limit int) ([]ShownItem, error)
}

type sqlShownRepository struct{ db *sql.DB }

// NewShownRepository creates a MariaDB-backed shown log.
func NewShownRepository(db *sql.DB) ShownRepository { return &sqlShownRepository{db: db} }

func (s *sqlShownRepository) AddShown(ctx context.Context, campaignID string, it ShownItem, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO encounter_shown (id, campaign_id, kind, label, text, entity_id, to_user, scene, shown_by, shown_at)
		 VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), ?)`,
		it.ID, campaignID, it.Kind, it.Label, it.Text, it.EntityID, it.To, it.Scene, userID, it.ShownAt.UTC())
	if err != nil {
		return fmt.Errorf("add shown item: %w", err)
	}
	return nil
}

func (s *sqlShownRepository) ListShown(ctx context.Context, campaignID string, since time.Time, limit int) ([]ShownItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, label, text, COALESCE(entity_id, ''), COALESCE(to_user, ''), scene, shown_at
		 FROM (SELECT * FROM encounter_shown WHERE campaign_id = ? AND shown_at >= ?
		       ORDER BY shown_at DESC, id DESC LIMIT ?) newest
		 ORDER BY shown_at, id`, campaignID, since.UTC(), limit)
	if err != nil {
		return nil, fmt.Errorf("list shown items: %w", err)
	}
	defer rows.Close()
	out := []ShownItem{}
	for rows.Next() {
		var it ShownItem
		if err := rows.Scan(&it.ID, &it.Kind, &it.Label, &it.Text, &it.EntityID, &it.To, &it.Scene, &it.ShownAt); err != nil {
			return nil, fmt.Errorf("scan shown item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}
