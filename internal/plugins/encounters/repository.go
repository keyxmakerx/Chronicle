// Hand-written SQL for the encounters table.
//
// SECURITY: every statement filters on campaign_id, so an encounter id from
// another campaign never reads, changes or deletes a row here.
package encounters

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Row is one stored encounter. PageName is filled only by Search, which
// joins the page.
type Row struct {
	ID         string
	CampaignID string
	EntityID   string
	PageName   string
	Name       string
	Ready      bool
	Data       []byte
	Version    int
	UpdatedAt  time.Time
}

// Repository is the persistence boundary for encounters.
type Repository interface {
	// ListForPage returns a page's encounters, oldest first.
	ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error)
	// Get returns one encounter; found is false when there is none.
	Get(ctx context.Context, campaignID, id string) (r Row, found bool, err error)
	// Create inserts a new encounter at version 1.
	Create(ctx context.Context, r Row, userID string) error
	// Update writes the encounter if the stored version is still expected
	// and bumps it. ok is false on a lost race or a missing row.
	Update(ctx context.Context, campaignID, id, name string, ready bool, data []byte, expected int, userID string) (ok bool, err error)
	// Delete removes one encounter; ok is false when there was none.
	Delete(ctx context.Context, campaignID, id string) (ok bool, err error)
	// Search lists the campaign's encounters on live pages, ready ones
	// first, then the most recently changed. q, when set, matches the
	// encounter's or its page's name. total counts every match.
	Search(ctx context.Context, campaignID, q string, limit int) (rows []Row, total int, err error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

const rowColumns = `enc.id, enc.campaign_id, enc.entity_id, enc.name, enc.ready, enc.data, enc.version, enc.updated_at`

func scanRow(sc interface{ Scan(...any) error }, extra ...any) (Row, error) {
	var r Row
	var data string
	dest := append([]any{&r.ID, &r.CampaignID, &r.EntityID, &r.Name, &r.Ready, &data, &r.Version, &r.UpdatedAt}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return Row{}, err
	}
	r.Data = []byte(data)
	return r, nil
}

func (s *sqlRepository) ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM encounters enc
		 WHERE enc.entity_id = ? AND enc.campaign_id = ?
		 ORDER BY enc.created_at, enc.id`,
		entityID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list page encounters: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan encounter: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sqlRepository) Get(ctx context.Context, campaignID, id string) (Row, bool, error) {
	r, err := scanRow(s.db.QueryRowContext(ctx,
		`SELECT `+rowColumns+` FROM encounters enc WHERE enc.id = ? AND enc.campaign_id = ?`,
		id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Row{}, false, nil
	}
	if err != nil {
		return Row{}, false, fmt.Errorf("get encounter: %w", err)
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
		`INSERT INTO encounters (id, campaign_id, entity_id, name, ready, data, version, created_by, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		r.ID, r.CampaignID, r.EntityID, r.Name, r.Ready, string(r.Data), nullable(userID), nullable(userID))
	if err != nil {
		return fmt.Errorf("insert encounter: %w", err)
	}
	return nil
}

func (s *sqlRepository) Update(ctx context.Context, campaignID, id, name string, ready bool, data []byte, expected int, userID string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE encounters SET name = ?, ready = ?, data = ?, version = version + 1, updated_by = ?
		 WHERE id = ? AND campaign_id = ? AND version = ?`,
		name, ready, string(data), nullable(userID), id, campaignID, expected)
	if err != nil {
		return false, fmt.Errorf("update encounter: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update encounter: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM encounters WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("delete encounter: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete encounter: %w", err)
	}
	return n == 1, nil
}

// escapeLike makes %, _ and the backslash match literally in a LIKE pattern
// (backslash is MariaDB's default LIKE escape).
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *sqlRepository) Search(ctx context.Context, campaignID, q string, limit int) ([]Row, int, error) {
	// A trashed page's encounters stay with it in the trash, so they are
	// left out here and come back if the page is restored.
	where := ` FROM encounters enc JOIN entities e ON e.id = enc.entity_id
		 WHERE enc.campaign_id = ? AND e.campaign_id = ? AND e.deleted_at IS NULL`
	args := []any{campaignID, campaignID}
	if q = strings.TrimSpace(q); q != "" {
		pat := "%" + escapeLike(q) + "%"
		where += ` AND (enc.name LIKE ? OR e.name LIKE ?)`
		args = append(args, pat, pat)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*)`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count encounters: %w", err)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+`, e.name`+where+` ORDER BY enc.ready DESC, enc.updated_at DESC, enc.id LIMIT ?`,
		append(args, limit)...)
	if err != nil {
		return nil, 0, fmt.Errorf("search encounters: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var page string
		r, err := scanRow(rows, &page)
		if err != nil {
			return nil, 0, fmt.Errorf("scan encounter: %w", err)
		}
		r.PageName = page
		out = append(out, r)
	}
	return out, total, rows.Err()
}
