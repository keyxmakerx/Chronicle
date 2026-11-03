// Hand-written SQL for the hero_layers table.
//
// SECURITY: every statement filters on campaign_id, so a layer id from
// another campaign never reads or deletes a row here.
package transformations

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Row is one stored layer.
type Row struct {
	ID         string
	CampaignID string
	EntityID   string
	Name       string
	Source     string
	Data       []byte
	CreatedAt  time.Time
}

// Repository is the persistence boundary for layers.
type Repository interface {
	// ListForPage returns a page's layers, oldest first.
	ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error)
	// Get returns one layer; found is false when there is none.
	Get(ctx context.Context, campaignID, id string) (r Row, found bool, err error)
	// Create inserts a layer unless the page already has max of them;
	// ok is false when it was full.
	Create(ctx context.Context, r Row, userID string, max int) (ok bool, err error)
	// Delete removes one layer; ok is false when there was none.
	Delete(ctx context.Context, campaignID, id string) (ok bool, err error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository creates a MariaDB-backed repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

const rowColumns = `id, campaign_id, entity_id, name, source, data, created_at`

func scanRow(sc interface{ Scan(...any) error }) (Row, error) {
	var r Row
	var data string
	if err := sc.Scan(&r.ID, &r.CampaignID, &r.EntityID, &r.Name, &r.Source, &data, &r.CreatedAt); err != nil {
		return Row{}, err
	}
	r.Data = []byte(data)
	return r, nil
}

func (s *sqlRepository) ListForPage(ctx context.Context, campaignID, entityID string) ([]Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+rowColumns+` FROM hero_layers
		 WHERE entity_id = ? AND campaign_id = ?
		 ORDER BY created_at, id`,
		entityID, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list layers: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		r, err := scanRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan layer: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sqlRepository) Get(ctx context.Context, campaignID, id string) (Row, bool, error) {
	r, err := scanRow(s.db.QueryRowContext(ctx,
		`SELECT `+rowColumns+` FROM hero_layers WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if err == sql.ErrNoRows {
		return Row{}, false, nil
	}
	if err != nil {
		return Row{}, false, fmt.Errorf("get layer: %w", err)
	}
	return r, true, nil
}

func nullable(userID string) any {
	if userID == "" {
		return nil
	}
	return userID
}

// Create counts and inserts in one statement, so two Directors adding at
// once can't pass the limit between a count and an insert.
func (s *sqlRepository) Create(ctx context.Context, r Row, userID string, max int) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO hero_layers (id, campaign_id, entity_id, name, source, data, created_by)
		 SELECT ?, ?, ?, ?, ?, ?, ? FROM DUAL
		 WHERE (SELECT COUNT(*) FROM hero_layers WHERE entity_id = ? AND campaign_id = ?) < ?`,
		r.ID, r.CampaignID, r.EntityID, r.Name, r.Source, string(r.Data), nullable(userID),
		r.EntityID, r.CampaignID, max)
	if err != nil {
		return false, fmt.Errorf("insert layer: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("insert layer: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM hero_layers WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("delete layer: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete layer: %w", err)
	}
	return n == 1, nil
}
