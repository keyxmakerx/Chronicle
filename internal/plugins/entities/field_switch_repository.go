package entities

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
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
	// category and its pages as they were. It refuses with a Conflict when
	// the category's answer or field list is no longer base.
	Switch(ctx context.Context, sw *FieldSwitch, base switchBase, fields []FieldDefinition, presetCategory *string, move valueMove) (int, error)
	// SetAside returns the values a switch took off one page.
	SetAside(ctx context.Context, entityID string) (map[string]any, error)

	// ForCampaign returns every answered category in a campaign, by category
	// id, for the campaign backup.
	ForCampaign(ctx context.Context, campaignID string) (map[int]*FieldSwitch, error)
	// SetAsideForCampaign returns every page's kept values in a campaign, by
	// page id, for the campaign backup.
	SetAsideForCampaign(ctx context.Context, campaignID string) (map[string]map[string]any, error)
	// Clear removes a category's answer, so the offer shows again.
	Clear(ctx context.Context, typeID int) error
	// RestoreSetAside writes one page's kept values as a backup carried them.
	RestoreSetAside(ctx context.Context, entityID string, typeID int, values map[string]any) error
}

// switchBase is what a switch was worked out from: the category's answer
// ("" when it had none) and its field list. The new field list and the value
// moves are only right for that base.
type switchBase struct {
	State  string
	Fields []FieldDefinition
}

// errSwitchMoved refuses a switch whose base changed under it, such as a
// second "Switch back" pressed while the first was running.
var errSwitchMoved = apperror.NewConflict("this category's fields changed while switching; reload the page and try again")

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

func (r *fieldSwitchRepository) ForCampaign(ctx context.Context, campaignID string) (map[int]*FieldSwitch, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT entity_type_id, campaign_id, system_id, preset_slug, state, old_fields, old_preset_category, updated_by, updated_at
		 FROM entity_type_field_switches WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing field switches: %w", err)
	}
	defer rows.Close()
	out := make(map[int]*FieldSwitch)
	for rows.Next() {
		sw := &FieldSwitch{}
		var oldRaw []byte
		if err := rows.Scan(&sw.EntityTypeID, &sw.CampaignID, &sw.SystemID, &sw.PresetSlug, &sw.State, &oldRaw, &sw.OldPresetCategory, &sw.UpdatedBy, &sw.UpdatedAt); err != nil {
			return nil, fmt.Errorf("reading field switch: %w", err)
		}
		if len(oldRaw) > 0 && string(oldRaw) != "null" {
			if err := json.Unmarshal(oldRaw, &sw.OldFields); err != nil {
				return nil, fmt.Errorf("reading field switch old fields: %w", err)
			}
		}
		out[sw.EntityTypeID] = sw
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing field switches: %w", err)
	}
	return out, nil
}

func (r *fieldSwitchRepository) SetAsideForCampaign(ctx context.Context, campaignID string) (map[string]map[string]any, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT s.entity_id, s.field_values
		 FROM entity_set_aside_values s JOIN entities e ON e.id = s.entity_id
		 WHERE e.campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("listing kept values: %w", err)
	}
	defer rows.Close()
	out := make(map[string]map[string]any)
	for rows.Next() {
		var id string
		var raw []byte
		if err := rows.Scan(&id, &raw); err != nil {
			return nil, fmt.Errorf("reading kept values: %w", err)
		}
		vals := map[string]any{}
		if err := json.Unmarshal(raw, &vals); err != nil {
			return nil, fmt.Errorf("reading page %s kept values: %w", id, err)
		}
		if len(vals) > 0 {
			out[id] = vals
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing kept values: %w", err)
	}
	return out, nil
}

func (r *fieldSwitchRepository) Clear(ctx context.Context, typeID int) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM entity_type_field_switches WHERE entity_type_id = ?`, typeID); err != nil {
		return fmt.Errorf("clearing field switch: %w", err)
	}
	return nil
}

func (r *fieldSwitchRepository) RestoreSetAside(ctx context.Context, entityID string, typeID int, values map[string]any) error {
	raw, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("encoding kept values: %w", err)
	}
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO entity_set_aside_values (entity_id, entity_type_id, field_values) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE entity_type_id = VALUES(entity_type_id), field_values = VALUES(field_values)`,
		entityID, typeID, raw); err != nil {
		return fmt.Errorf("restoring kept values: %w", err)
	}
	return nil
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

func (r *fieldSwitchRepository) Switch(ctx context.Context, sw *FieldSwitch, base switchBase, fields []FieldDefinition, presetCategory *string, move valueMove) (int, error) {
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return 0, fmt.Errorf("encoding fields: %w", err)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("starting field switch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the category first, so two presses at once run one after the
	// other. The new list and the moves were worked out before the lock, so
	// the second press is refused when the first changed what it read;
	// running it would move values a second time.
	var lockedFields []byte
	if err := tx.QueryRowContext(ctx, `SELECT fields FROM entity_types WHERE id = ? FOR UPDATE`, sw.EntityTypeID).Scan(&lockedFields); err != nil {
		return 0, fmt.Errorf("locking category: %w", err)
	}
	var lockedState string
	switch err := tx.QueryRowContext(ctx,
		`SELECT state FROM entity_type_field_switches WHERE entity_type_id = ? FOR UPDATE`, sw.EntityTypeID,
	).Scan(&lockedState); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return 0, fmt.Errorf("reading field switch: %w", err)
	}
	same, err := sameFieldList(lockedFields, base.Fields)
	if err != nil {
		return 0, err
	}
	if lockedState != base.State || !same {
		return 0, errSwitchMoved
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

// sameFieldList reports whether a stored field list is want. Both sides go
// through the same encoding, so spacing and key order in the stored JSON
// make no difference.
func sameFieldList(stored []byte, want []FieldDefinition) (bool, error) {
	var got []FieldDefinition
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &got); err != nil {
			return false, fmt.Errorf("reading category fields: %w", err)
		}
	}
	if len(got) == 0 || len(want) == 0 {
		return len(got) == len(want), nil
	}
	a, err := json.Marshal(got)
	if err != nil {
		return false, fmt.Errorf("encoding category fields: %w", err)
	}
	b, err := json.Marshal(want)
	if err != nil {
		return false, fmt.Errorf("encoding category fields: %w", err)
	}
	return bytes.Equal(a, b), nil
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
