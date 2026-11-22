package entities

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// FieldSwitchRepository stores the owner's answer to a system's switch offer
// and the values a switch took off each page (entity_type_field_switches,
// entity_set_aside_values).
type FieldSwitchRepository interface {
	// Get returns a category's answer, or nil when it has none.
	Get(ctx context.Context, typeID int) (*FieldSwitch, error)
	// SaveState records an answer that changes no page ("Not now").
	SaveState(ctx context.Context, sw *FieldSwitch) error
	// Switch rewrites the category's fields, moves every page's values with
	// move and records sw, all in one transaction, so a failure leaves the
	// category and its pages as they were.
	Switch(ctx context.Context, sw *FieldSwitch, fields []FieldDefinition, presetCategory *string, move valueMove) (int, error)
	// SetAside returns the values a switch took off one page.
	SetAside(ctx context.Context, entityID string) (map[string]any, error)
}

type fieldSwitchRepository struct {
	db *sql.DB
}

// NewFieldSwitchRepository creates the field switch repository.
func NewFieldSwitchRepository(db *sql.DB) FieldSwitchRepository {
	return &fieldSwitchRepository{db: db}
}

func (r *fieldSwitchRepository) Get(ctx context.Context, typeID int) (*FieldSwitch, error) {
	sw := &FieldSwitch{EntityTypeID: typeID}
	var oldRaw []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT campaign_id, system_id, preset_slug, state, old_fields, old_preset_category, updated_by, updated_at
		 FROM entity_type_field_switches WHERE entity_type_id = ?`, typeID,
	).Scan(&sw.CampaignID, &sw.SystemID, &sw.PresetSlug, &sw.State, &oldRaw, &sw.OldPresetCategory, &sw.UpdatedBy, &sw.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading field switch: %w", err)
	}
	if len(oldRaw) > 0 && string(oldRaw) != "null" {
		if err := json.Unmarshal(oldRaw, &sw.OldFields); err != nil {
			return nil, fmt.Errorf("reading field switch old fields: %w", err)
		}
	}
	return sw, nil
}

// upsertState is the shared write of a switch row, on the db or a transaction.
func upsertState(ctx context.Context, ex interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, sw *FieldSwitch) error {
	var old any
	if sw.OldFields != nil {
		b, err := json.Marshal(sw.OldFields)
		if err != nil {
			return fmt.Errorf("encoding old fields: %w", err)
		}
		old = b
	}
	_, err := ex.ExecContext(ctx,
		`INSERT INTO entity_type_field_switches
		   (entity_type_id, campaign_id, system_id, preset_slug, state, old_fields, old_preset_category, updated_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE system_id = VALUES(system_id), preset_slug = VALUES(preset_slug),
		   state = VALUES(state), old_fields = VALUES(old_fields),
		   old_preset_category = VALUES(old_preset_category), updated_by = VALUES(updated_by)`,
		sw.EntityTypeID, sw.CampaignID, sw.SystemID, sw.PresetSlug, sw.State, old, sw.OldPresetCategory, sw.UpdatedBy)
	if err != nil {
		return fmt.Errorf("saving field switch: %w", err)
	}
	return nil
}

func (r *fieldSwitchRepository) SaveState(ctx context.Context, sw *FieldSwitch) error {
	return upsertState(ctx, r.db, sw)
}

func (r *fieldSwitchRepository) Switch(ctx context.Context, sw *FieldSwitch, fields []FieldDefinition, presetCategory *string, move valueMove) (int, error) {
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return 0, fmt.Errorf("encoding fields: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting field switch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the category first, so two owners pressing at once run one after
	// the other and the second sees the first one's fields.
	var lockedID int
	if err := tx.QueryRowContext(ctx, `SELECT id FROM entity_types WHERE id = ? FOR UPDATE`, sw.EntityTypeID).Scan(&lockedID); err != nil {
		return 0, fmt.Errorf("locking category: %w", err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT e.id, e.fields_data, s.field_values
		 FROM entities e LEFT JOIN entity_set_aside_values s ON s.entity_id = e.id
		 WHERE e.entity_type_id = ? ORDER BY e.id FOR UPDATE`, sw.EntityTypeID)
	if err != nil {
		return 0, fmt.Errorf("reading pages: %w", err)
	}
	type pageMove struct {
		id         string
		data, kept map[string]any
	}
	var moves []pageMove
	for rows.Next() {
		var id string
		var dataRaw, keptRaw []byte
		if err := rows.Scan(&id, &dataRaw, &keptRaw); err != nil {
			rows.Close()
			return 0, fmt.Errorf("reading page: %w", err)
		}
		data := map[string]any{}
		if len(dataRaw) > 0 {
			if err := json.Unmarshal(dataRaw, &data); err != nil {
				rows.Close()
				return 0, fmt.Errorf("reading page %s values: %w", id, err)
			}
		}
		kept := map[string]any{}
		if len(keptRaw) > 0 {
			if err := json.Unmarshal(keptRaw, &kept); err != nil {
				rows.Close()
				return 0, fmt.Errorf("reading page %s kept values: %w", id, err)
			}
		}
		if d, k, changed := move.apply(data, kept); changed {
			moves = append(moves, pageMove{id: id, data: d, kept: k})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("reading pages: %w", err)
	}
	rows.Close()

	for _, m := range moves {
		dataJSON, err := json.Marshal(m.data)
		if err != nil {
			return 0, fmt.Errorf("encoding page values: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE entities SET fields_data = ? WHERE id = ?`, dataJSON, m.id); err != nil {
			return 0, fmt.Errorf("writing page values: %w", err)
		}
		if len(m.kept) == 0 {
			if _, err := tx.ExecContext(ctx, `DELETE FROM entity_set_aside_values WHERE entity_id = ?`, m.id); err != nil {
				return 0, fmt.Errorf("clearing kept values: %w", err)
			}
			continue
		}
		keptJSON, err := json.Marshal(m.kept)
		if err != nil {
			return 0, fmt.Errorf("encoding kept values: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO entity_set_aside_values (entity_id, entity_type_id, field_values) VALUES (?, ?, ?)
			 ON DUPLICATE KEY UPDATE entity_type_id = VALUES(entity_type_id), field_values = VALUES(field_values)`,
			m.id, sw.EntityTypeID, keptJSON); err != nil {
			return 0, fmt.Errorf("writing kept values: %w", err)
		}
	}

	if _, err := tx.ExecContext(ctx, `UPDATE entity_types SET fields = ?, preset_category = ? WHERE id = ?`,
		fieldsJSON, presetCategory, sw.EntityTypeID); err != nil {
		return 0, fmt.Errorf("writing category fields: %w", err)
	}
	if err := upsertState(ctx, tx, sw); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing field switch: %w", err)
	}
	return len(moves), nil
}

func (r *fieldSwitchRepository) SetAside(ctx context.Context, entityID string) (map[string]any, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT field_values FROM entity_set_aside_values WHERE entity_id = ?`, entityID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading kept values: %w", err)
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("reading kept values: %w", err)
	}
	return out, nil
}
