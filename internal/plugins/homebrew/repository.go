package homebrew

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
)

// errDuplicate is a recipe name already used in the campaign.
var errDuplicate = errors.New("duplicate name")

// Repository is the persistence boundary for recipes.
type Repository interface {
	List(ctx context.Context, campaignID string) ([]Recipe, error)
	Get(ctx context.Context, campaignID, id string) (Recipe, bool, error)
	Count(ctx context.Context, campaignID string) (int, error)
	Create(ctx context.Context, r Recipe) error
	// Update writes r only if the stored version is still expected, and
	// reports whether it did.
	Update(ctx context.Context, r Recipe, expected int) (bool, error)
	Delete(ctx context.Context, campaignID, id string) (bool, error)
}

type sqlRepository struct{ db *sql.DB }

// NewRepository returns the MariaDB repository.
func NewRepository(db *sql.DB) Repository { return &sqlRepository{db: db} }

func isDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

const recipeCols = `id, campaign_id, name, doc, version, COALESCE(created_by, ''), created_at, updated_at`

func scanRecipe(sc interface{ Scan(...any) error }) (Recipe, error) {
	var r Recipe
	var doc string
	if err := sc.Scan(&r.ID, &r.CampaignID, &r.Name, &doc, &r.Version, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return r, err
	}
	if err := json.Unmarshal([]byte(doc), &r.Doc); err != nil {
		return r, fmt.Errorf("decode recipe %s: %w", r.ID, err)
	}
	return r, nil
}

func (s *sqlRepository) List(ctx context.Context, campaignID string) ([]Recipe, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+recipeCols+` FROM homebrew_recipes WHERE campaign_id = ? ORDER BY name`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	defer rows.Close()
	out := []Recipe{}
	for rows.Next() {
		r, err := scanRecipe(rows)
		if err != nil {
			return nil, fmt.Errorf("list recipes: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *sqlRepository) Get(ctx context.Context, campaignID, id string) (Recipe, bool, error) {
	r, err := scanRecipe(s.db.QueryRowContext(ctx,
		`SELECT `+recipeCols+` FROM homebrew_recipes WHERE id = ? AND campaign_id = ?`, id, campaignID))
	if errors.Is(err, sql.ErrNoRows) {
		return Recipe{}, false, nil
	}
	if err != nil {
		return Recipe{}, false, fmt.Errorf("get recipe: %w", err)
	}
	return r, true, nil
}

func (s *sqlRepository) Count(ctx context.Context, campaignID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM homebrew_recipes WHERE campaign_id = ?`, campaignID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count recipes: %w", err)
	}
	return n, nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *sqlRepository) Create(ctx context.Context, r Recipe) error {
	doc, err := json.Marshal(r.Doc)
	if err != nil {
		return fmt.Errorf("encode recipe: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO homebrew_recipes (id, campaign_id, name, doc, version, created_by) VALUES (?, ?, ?, ?, 1, ?)`,
		r.ID, r.CampaignID, r.Name, string(doc), nullString(r.CreatedBy))
	if isDuplicate(err) {
		return errDuplicate
	}
	if err != nil {
		return fmt.Errorf("create recipe: %w", err)
	}
	return nil
}

func (s *sqlRepository) Update(ctx context.Context, r Recipe, expected int) (bool, error) {
	doc, err := json.Marshal(r.Doc)
	if err != nil {
		return false, fmt.Errorf("encode recipe: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE homebrew_recipes SET name = ?, doc = ?, version = version + 1
		 WHERE id = ? AND campaign_id = ? AND version = ?`,
		r.Name, string(doc), r.ID, r.CampaignID, expected)
	if isDuplicate(err) {
		return false, errDuplicate
	}
	if err != nil {
		return false, fmt.Errorf("update recipe: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update recipe: %w", err)
	}
	return n == 1, nil
}

func (s *sqlRepository) Delete(ctx context.Context, campaignID, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM homebrew_recipes WHERE id = ? AND campaign_id = ?`, id, campaignID)
	if err != nil {
		return false, fmt.Errorf("delete recipe: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete recipe: %w", err)
	}
	return n == 1, nil
}
