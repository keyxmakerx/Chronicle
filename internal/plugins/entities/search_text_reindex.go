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
	// ListSearchCandidates returns, in id order after afterID, up to limit
	// pages whose search_text may hold something not every reader may see.
	// typeID 0 means every campaign's pages that hold GM-only content or a
	// page mention, or whose category or own fields have a GM-only or
	// owner-only field; a typeID returns every page of that category.
	ListSearchCandidates(ctx context.Context, typeID int, afterID string, limit int) ([]SearchTextCandidate, error)
	// SetSearchText rewrites one page's search_text and nothing else.
	SetSearchText(ctx context.Context, id, searchText string) error
}

// SearchTextCandidate is one page the reindex recomputes.
type SearchTextCandidate struct {
	ID             string
	EntityTypeID   int
	EntryHTML      string
	FieldsData     map[string]any
	FieldOverrides *FieldOverrides
	SearchText     string
}

// ReindexSearchText recomputes search_text the way a save would now write it,
// so pages written before a rule tightened (GM-only text, mention names,
// GM-only and owner-only field values) get the same guarantee as new saves.
// typeID 0 sweeps every candidate page; a typeID redoes one category after
// its fields change. Only rows whose text changes are written, so a later run
// finds nothing to do. Returns how many rows it rewrote.
func ReindexSearchText(ctx context.Context, repo EntityRepository, types EntityTypeRepository, typeID int) (int, error) {
	store, ok := repo.(SearchTextReindexStore)
	if !ok {
		return 0, nil
	}
	fieldsOf := map[int][]FieldDefinition{}
	known := map[int]bool{}
	const batch = 200
	rewritten := 0
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return rewritten, err
		}
		rows, err := store.ListSearchCandidates(ctx, typeID, after, batch)
		if err != nil {
			return rewritten, err
		}
		for _, row := range rows {
			if _, done := fieldsOf[row.EntityTypeID]; !done {
				if et, err := types.FindByID(ctx, row.EntityTypeID); err == nil && et != nil {
					fieldsOf[row.EntityTypeID], known[row.EntityTypeID] = et.Fields, true
				} else {
					fieldsOf[row.EntityTypeID] = nil
				}
			}
			// A page whose category can't be read indexes no field values,
			// as a save would.
			var fields map[string]any
			if known[row.EntityTypeID] && len(row.FieldsData) > 0 {
				fields = searchableFields(row.FieldsData, fieldsOf[row.EntityTypeID], row.FieldOverrides)
			}
			want := buildSearchText(row.EntryHTML, fields)
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

// ListSearchCandidates implements SearchTextReindexStore. The HTML markers
// are the kinds of GM-only content sanitize.StripSecretsHTML removes, plus
// page mentions; the field markers are FieldDefinition's JSON keys, which are
// written only when set, matched loosely since a looser match costs only a
// row that is left as it is.
func (r *entityRepository) ListSearchCandidates(ctx context.Context, typeID int, afterID string, limit int) ([]SearchTextCandidate, error) {
	where := `(e.entry_html LIKE '%data-secret%' OR e.entry_html LIKE '%ce-img--gm%' OR e.entry_html LIKE '%ce-diagram--gm%'
		   OR e.entry_html LIKE '%ce-roll%' OR e.entry_html LIKE '%data-mention-id%'
		   OR et.fields LIKE '%"gm_only"%' OR et.fields LIKE '%"owner_only"%'
		   OR e.field_overrides LIKE '%"gm_only"%' OR e.field_overrides LIKE '%"owner_only"%')`
	args := []any{afterID}
	if typeID > 0 {
		where = `e.entity_type_id = ?`
		args = append(args, typeID)
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx,
		`SELECT e.id, e.entity_type_id, e.entry_html, e.fields_data, e.field_overrides, e.search_text
		 FROM entities e
		 JOIN entity_types et ON et.id = e.entity_type_id
		 WHERE e.id > ? AND `+where+`
		 ORDER BY e.id
		 LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("listing search_text candidates: %w", err)
	}
	defer rows.Close()
	var out []SearchTextCandidate
	for rows.Next() {
		var c SearchTextCandidate
		var html, search sql.NullString
		var fields, overrides []byte
		if err := rows.Scan(&c.ID, &c.EntityTypeID, &html, &fields, &overrides, &search); err != nil {
			return nil, fmt.Errorf("scanning search_text candidate: %w", err)
		}
		c.EntryHTML, c.SearchText = html.String, search.String
		if len(fields) > 0 {
			// A row whose fields don't parse is reindexed from its HTML alone,
			// which can only drop words, never add secret ones.
			_ = json.Unmarshal(fields, &c.FieldsData)
		}
		if len(overrides) > 0 {
			c.FieldOverrides = &FieldOverrides{}
			if json.Unmarshal(overrides, c.FieldOverrides) != nil {
				// Unreadable overrides may hide a restricted added field, so
				// the row indexes no field values.
				c.FieldsData = nil
				c.FieldOverrides = nil
			}
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
