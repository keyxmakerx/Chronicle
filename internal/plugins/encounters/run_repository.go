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
	// DeleteRun ends the campaign's run; ending none is not an error.
	DeleteRun(ctx context.Context, campaignID string) error
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

func (s *sqlRunRepository) DeleteRun(ctx context.Context, campaignID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM encounter_runs WHERE campaign_id = ?`, campaignID); err != nil {
		return fmt.Errorf("delete encounter run: %w", err)
	}
	return nil
}
