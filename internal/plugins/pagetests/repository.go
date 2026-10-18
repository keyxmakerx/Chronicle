// Hand-written SQL for the page_tests table.
//
// SECURITY: every statement filters on campaign_id, so a test id from
// another campaign never reads, changes or deletes a row here.
package pagetests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Row is one stored test. PageName is filled only by ListForCampaign, which
// joins the page.
type Row struct {
	ID         string
	CampaignID string
	EntityID   string
	PageName   string
	Name       string
	Shown      bool
	Data       []byte
	Version    int
	UpdatedAt  time.Time
}

// Repository is the persistence boundary for tests.
type Repository interface {
	// ListForPage returns a page's tests, oldest first.
	ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error)
	// ListForCampaign returns the campaign's tests on live pages, newest
	// change first, at most limit.
	ListForCampaign(ctx context.Context, campaignID string, limit int) ([]Row, error)
	// GetMany returns the tests among ids that belong to campaignID.
	GetMany(ctx context.Context, campaignID string, ids []string) ([]Row, error)
	// Get returns one test; found is false when there is none.
	Get(ctx context.Context, campaignID, id string) (r Row, found bool, err error)
	// Create inserts a new test at version 1.
	Create(ctx context.Context, r Row, userID string) error
	// Update writes the test if the stored version is still expected and
	// bumps it. ok is false on a lost race or a missing row.
	Update(ctx context.Context, campaignID, id, name string, shown bool, data []byte, expected int, userID string) (ok bool, err error)
	// Delete removes one test; ok is false when there was none.
	Delete(ctx context.Context, campaignID, id string) (ok bool, err error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

const rowColumns = `t.id, t.campaign_id, t.entity_id, t.name, t.shown, t.data, t.version, t.updated_at`

func scanRow(sc interface{ Scan(...any) error }, extra ...any) (Row, error) {
	var r Row
	var data string
	dest := append([]any{&r.ID, &r.CampaignID, &r.EntityID, &r.Name, &r.Shown, &data, &r.Version, &r.UpdatedAt}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return Row{}, err
	}
	r.Data = []byte(data)
	return r, nil
}

func scanAll(rows *sql.Rows, withPage bool) ([]Row, error) {
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var page string
		var extra []any
		if withPage {
			extra = append(extra, &page)
		}
		r, err := scanRow(rows, extra...)
		if err != nil {
			return nil, fmt.Errorf("scan test: %w", err)
		}
		r.PageName = page
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sqlRepository) ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM page_tests t
		 WHERE t.entity_id = ? AND t.campaign_id = ?
		 ORDER BY t.created_at, t.id`,
		entityID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list page tests: %w", err)
	}
	return scanAll(rows, false)
}

func (s *sqlRepository) ListForCampaign(ctx context.Context, campaignID string, limit int) ([]Row, error) {
	// A trashed page's tests stay with it in the trash, so they are left
	// out here and come back if the page is restored.
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+`, e.name FROM page_tests t JOIN entities e ON e.id = t.entity_id
		 WHERE t.campaign_id = ? AND e.campaign_id = ? AND e.deleted_at IS NULL
		 ORDER BY t.updated_at DESC, t.id LIMIT ?`,
		campaignID, campaignID, limit)
	if err != nil {
		return nil, fmt.Errorf("list campaign tests: %w", err)
	}
	return scanAll(rows, true)
}

func (s *sqlRepository) GetMany(ctx context.Context, campaignID string, ids []string) ([]Row, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, campaignID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM page_tests t
		 WHERE t.campaign_id = ? AND t.id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`,
		args...)
	if err != nil {
		return nil, fmt.Errorf("get tests: %w", err)
	}
	return scanAll(rows, false)
}

func (s *sqlRepository) Get(ctx context.Context, campaignID, id string) (Row, bool, error) {
	r, err := scanRow(s.db.QueryRowContext(ctx,
		`SELECT `+rowColumns+` FROM page_tests t WHERE t.id = ? AND t.campaign_id = ?`,
		id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, false, nil
	}
	if err != nil {
		return Row{}, false, fmt.Errorf("get test: %w", err)
	}
	return r, true, nil
}

func nullable(userID string) any {
	if userID == "" {
		return nil
	}
	return userID
}

func (s *sqlRepository) Create(ctx context.Context, r Row, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO page_tests (id, campaign_id, entity_id, name, shown, data, version, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		r.ID, r.CampaignID, r.EntityID, r.Name, r.Shown, string(r.Data), nullable(userID), nullable(userID))
	if err != nil {
		return fmt.Errorf("insert test: %w", err)
	}
	return nil
}

func (s *sqlRepository) Update(ctx context.Context, campaignID, id, name string, shown bool, data []byte, expected int, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE page_tests SET name = ?, shown = ?, data = ?, version = version + 1, updated_by = ?
		 WHERE id = ? AND campaign_id = ? AND version = ?`,
		name, shown, string(data), nullable(userID), id, campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("update test: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update test: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM page_tests WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("delete test: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete test: %w", err)
	}
	return n == 1, nil
}
