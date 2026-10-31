package crafting

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// PackRow is a stored recipe pack. Body is the pack's JSON.
type PackRow struct {
	ID          string
	CampaignID  string
	CreatedBy   string
	Name        string
	Source      string
	Shared      bool
	RecipeCount int
	Body        string
	UpdatedAt   time.Time
}

// PackStore keeps recipe packs.
type PackStore interface {
	CreatePack(ctx context.Context, p PackRow) error
	GetPack(ctx context.Context, id string) (PackRow, bool, error)
	// ListPacks lists the packs of the given campaigns, and every shared
	// pack when shared is set, newest first, without their bodies.
	ListPacks(ctx context.Context, campaignIDs []string, shared bool, limit int) ([]PackRow, error)
	// UpdatePack renames a pack or shares it; ok is false when the pack is
	// not in the campaign.
	UpdatePack(ctx context.Context, campaignID, id, name string, shared bool) (ok bool, err error)
	DeletePack(ctx context.Context, campaignID, id string) error
}

type sqlPackStore struct{ db *sql.DB }

// NewPackStore creates a MariaDB-backed pack store.
func NewPackStore(db *sql.DB) PackStore { return &sqlPackStore{db: db} }

func (s *sqlPackStore) CreatePack(ctx context.Context, p PackRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO crafting_packs (id, campaign_id, created_by, name, source, shared, recipe_count, body) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CampaignID, nullString(p.CreatedBy), p.Name, p.Source, p.Shared, p.RecipeCount, p.Body)
	return err
}

func (s *sqlPackStore) GetPack(ctx context.Context, id string) (PackRow, bool, error) {
	var p PackRow
	var by sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, campaign_id, created_by, name, source, shared, recipe_count, body, updated_at FROM crafting_packs WHERE id = ?`, id).
		Scan(&p.ID, &p.CampaignID, &by, &p.Name, &p.Source, &p.Shared, &p.RecipeCount, &p.Body, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	p.CreatedBy = by.String
	return p, err == nil, err
}

func (s *sqlPackStore) ListPacks(ctx context.Context, campaignIDs []string, shared bool, limit int) ([]PackRow, error) {
	var where []string
	var args []any
	if len(campaignIDs) > 0 {
		where = append(where, "campaign_id IN (?"+strings.Repeat(", ?", len(campaignIDs)-1)+")")
		for _, id := range campaignIDs {
			args = append(args, id)
		}
	}
	if shared {
		where = append(where, "shared = 1")
	}
	if len(where) == 0 {
		return []PackRow{}, nil
	}
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, campaign_id, created_by, name, source, shared, recipe_count, updated_at FROM crafting_packs WHERE `+
			strings.Join(where, " OR ")+` ORDER BY updated_at DESC, id LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PackRow{}
	for rows.Next() {
		var p PackRow
		var by sql.NullString
		if err := rows.Scan(&p.ID, &p.CampaignID, &by, &p.Name, &p.Source, &p.Shared, &p.RecipeCount, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.CreatedBy = by.String
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *sqlPackStore) UpdatePack(ctx context.Context, campaignID, id, name string, shared bool) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE crafting_packs SET name = ?, shared = ? WHERE id = ? AND campaign_id = ?`, name, shared, id, campaignID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n > 0 {
		return true, nil
	}
	// MariaDB counts only changed rows; a save that changed nothing still
	// found its pack.
	var one int
	err = s.db.QueryRowContext(ctx, `SELECT 1 FROM crafting_packs WHERE id = ? AND campaign_id = ?`, id, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *sqlPackStore) DeletePack(ctx context.Context, campaignID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM crafting_packs WHERE id = ? AND campaign_id = ?`, id, campaignID)
	return err
}
