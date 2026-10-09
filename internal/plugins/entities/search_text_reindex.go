package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// SearchTextReindexStore is the slice of the entity repository the
// search_text reindex needs. The MariaDB repository implements it.
type SearchTextReindexStore interface {
	// ListSecretSearchCandidates returns, in id order after afterID, up to
	// limit pages whose stored HTML holds GM-only content.
	ListSecretSearchCandidates(ctx context.Context, afterID string, limit int) ([]SearchTextCandidate, error)
	// SetSearchText rewrites one page's search_text and nothing else.
	SetSearchText(ctx context.Context, id, searchText string) error
}

// SearchTextCandidate is one page the reindex recomputes.
type SearchTextCandidate struct {
	ID         string
	EntryHTML  string
	FieldsData map[string]any
	SearchText string
}

// ReindexSecretSearchText recomputes search_text for every page whose HTML
// holds GM-only content, so pages written before search_text stopped
// indexing it get the same guarantee as new writes. Only rows whose text
// changes are written, so later boots find nothing to do. Returns how many
// rows it rewrote.
func ReindexSecretSearchText(ctx context.Context, repo EntityRepository) (int, error) {
	store, ok := repo.(SearchTextReindexStore)
	if !ok {
		return 0, nil
	}
	const batch = 200
	rewritten := 0
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return rewritten, err
		}
		rows, err := store.ListSecretSearchCandidates(ctx, after, batch)
		if err != nil {
			return rewritten, err
		}
		for _, row := range rows {
			want := buildSearchText(row.EntryHTML, row.FieldsData)
			if want == row.SearchText {
				continue
			}
			if err := store.SetSearchText(ctx, row.ID, want); err != nil {
				return rewritten, err
			}
			rewritten++
		}
		if len(rows) < batch {
			return rewritten, nil
		}
		after = rows[len(rows)-1].ID
	}
}

// ListSecretSearchCandidates implements SearchTextReindexStore. The markers
// are the three kinds of GM-only content sanitize.StripSecretsHTML removes.
func (r *entityRepository) ListSecretSearchCandidates(ctx context.Context, afterID string, limit int) ([]SearchTextCandidate, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, entry_html, fields_data, search_text
		 FROM entities
		 WHERE id > ?
		   AND (entry_html LIKE '%data-secret%' OR entry_html LIKE '%ce-img--gm%' OR entry_html LIKE '%ce-roll%')
		 ORDER BY id
		 LIMIT ?`, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing search_text candidates: %w", err)
	}
	defer rows.Close()
	var out []SearchTextCandidate
	for rows.Next() {
		var c SearchTextCandidate
		var html, search sql.NullString
		var fields []byte
		if err := rows.Scan(&c.ID, &html, &fields, &search); err != nil {
			return nil, fmt.Errorf("scanning search_text candidate: %w", err)
		}
		c.EntryHTML, c.SearchText = html.String, search.String
		if len(fields) > 0 {
			// A row whose fields don't parse is reindexed from its HTML alone,
			// which can only drop words, never add secret ones.
			_ = json.Unmarshal(fields, &c.FieldsData)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetSearchText implements SearchTextReindexStore. updated_at is left alone:
// the page's content did not change.
func (r *entityRepository) SetSearchText(ctx context.Context, id, searchText string) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE entities SET search_text = ? WHERE id = ?`, searchText, id); err != nil {
		return fmt.Errorf("setting search_text: %w", err)
	}
	return nil
}
