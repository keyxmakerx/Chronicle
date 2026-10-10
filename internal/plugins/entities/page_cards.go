package entities

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// PageCard is the little another feature shows for a page it links to: its
// name, picture and category.
type PageCard struct {
	ID        string
	Name      string
	ImagePath string
	TypeName  string
	TypeSlug  string
}

// PageCards reads many pages of one campaign in a single query, for features
// that show lots of linked pages at once (notice boards). It only names pages;
// whether a viewer may see them is still FilterViewableEntityIDs' decision.
type PageCards interface {
	// InCampaign returns the live pages among ids that belong to campaignID;
	// a missing, deleted or foreign id is simply absent.
	InCampaign(ctx context.Context, campaignID string, ids []string) (map[string]PageCard, error)
}

// maxPageCards bounds one query's IN list; callers with more ids are served
// in chunks.
const maxPageCards = 500

// NewPageCards builds the reader.
func NewPageCards(db *sql.DB) PageCards { return &pageCards{db: db} }

type pageCards struct{ db *sql.DB }

func (p *pageCards) InCampaign(ctx context.Context, campaignID string, ids []string) (map[string]PageCard, error) {
	out := make(map[string]PageCard, len(ids))
	seen := make(map[string]bool, len(ids))
	uniq := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	for start := 0; start < len(uniq); start += maxPageCards {
		chunk := uniq[start:min(start+maxPageCards, len(uniq))]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, campaignID)
		for _, id := range chunk {
			args = append(args, id)
		}
		query := `SELECT e.id, e.name, e.image_path, et.name, et.slug
		          FROM entities e
		          INNER JOIN entity_types et ON et.id = e.entity_type_id
		          WHERE e.campaign_id = ? AND e.id IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(chunk)), ", ") + `)` + liveOnly
		if err := p.scan(ctx, query, args, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (p *pageCards) scan(ctx context.Context, query string, args []any, out map[string]PageCard) error {
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reading page cards: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var c PageCard
		var img sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &img, &c.TypeName, &c.TypeSlug); err != nil {
			return fmt.Errorf("scanning page card: %w", err)
		}
		c.ImagePath = img.String
		out[c.ID] = c
	}
	return rows.Err()
}
