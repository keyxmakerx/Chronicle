// Package calendar — era_version_repository.go: MariaDB reads and writes for
// entity_era_versions. Hand-written SQL per the project conventions; page and
// era deletes cascade in the database.
package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

type eraVersionRepo struct {
	db *sql.DB
}

// NewEraVersionRepository builds the MariaDB-backed EraVersionRepository.
func NewEraVersionRepository(db *sql.DB) EraVersionRepository {
	return &eraVersionRepo{db: db}
}

const eraVersionCols = `entity_id, era_id, name, image_path, entry, entry_html, entry_rev, fields_data, updated_by, updated_at`

func scanEraVersion(sc interface{ Scan(...any) error }) (*EntityEraVersion, error) {
	var v EntityEraVersion
	var name, img, entry, html, fields, by sql.NullString
	if err := sc.Scan(&v.EntityID, &v.EraID, &name, &img, &entry, &html, &v.EntryRev, &fields, &by, &v.UpdatedAt); err != nil {
		return nil, err
	}
	v.Name = nullStr(name)
	v.ImagePath = nullStr(img)
	v.Entry = nullStr(entry)
	v.EntryHTML = nullStr(html)
	v.UpdatedBy = nullStr(by)
	if fields.Valid && fields.String != "" {
		if err := json.Unmarshal([]byte(fields.String), &v.FieldsData); err != nil {
			return nil, err
		}
	}
	return &v, nil
}

func nullStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

func (r *eraVersionRepo) EraInCampaign(ctx context.Context, eraID int, campaignID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM calendar_eras er JOIN calendars c ON c.id = er.calendar_id
		  WHERE er.id = ? AND c.campaign_id = ?)`, eraID, campaignID).Scan(&ok)
	return ok, err
}

func (r *eraVersionRepo) EntityInCampaign(ctx context.Context, entityID, campaignID string) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entities WHERE id = ? AND campaign_id = ? AND deleted_at IS NULL)`,
		entityID, campaignID).Scan(&ok)
	return ok, err
}

func (r *eraVersionRepo) Get(ctx context.Context, entityID string, eraID int) (*EntityEraVersion, error) {
	v, err := scanEraVersion(r.db.QueryRowContext(ctx,
		`SELECT `+eraVersionCols+` FROM entity_era_versions WHERE entity_id = ? AND era_id = ?`, entityID, eraID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (r *eraVersionRepo) EraIDs(ctx context.Context, entityID string) ([]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT era_id FROM entity_era_versions WHERE entity_id = ?`, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *eraVersionRepo) ForEntities(ctx context.Context, entityIDs []string, eraID int) (map[string]*EntityEraVersion, error) {
	out := map[string]*EntityEraVersion{}
	if len(entityIDs) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(entityIDs)+1)
	args = append(args, eraID)
	for _, id := range entityIDs {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+eraVersionCols+` FROM entity_era_versions WHERE era_id = ? AND entity_id IN (?`+
			strings.Repeat(",?", len(entityIDs)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		v, err := scanEraVersion(rows)
		if err != nil {
			return nil, err
		}
		out[v.EntityID] = v
	}
	return out, rows.Err()
}

func (r *eraVersionRepo) Create(ctx context.Context, entityID string, eraID int, userID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT IGNORE INTO entity_era_versions (entity_id, era_id, updated_by) VALUES (?, ?, ?)`,
		entityID, eraID, nullIfEmpty(userID))
	return err
}

// exists tells "no such version" apart from an update that changed nothing,
// which RowsAffected reports the same way.
func (r *eraVersionRepo) exists(ctx context.Context, entityID string, eraID int) (bool, error) {
	var ok bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM entity_era_versions WHERE entity_id = ? AND era_id = ?)`,
		entityID, eraID).Scan(&ok)
	return ok, err
}

func (r *eraVersionRepo) SetIdentity(ctx context.Context, entityID string, eraID int, name, imagePath patch.Field[string], userID string) (bool, error) {
	var sets []string
	var args []any
	if name.Present() {
		sets = append(sets, "name = ?")
		args = append(args, name.Ptr(nil))
	}
	if imagePath.Present() {
		sets = append(sets, "image_path = ?")
		args = append(args, imagePath.Ptr(nil))
	}
	if len(sets) > 0 {
		sets = append(sets, "updated_by = ?")
		args = append(args, nullIfEmpty(userID), entityID, eraID)
		if _, err := r.db.ExecContext(ctx,
			`UPDATE entity_era_versions SET `+strings.Join(sets, ", ")+` WHERE entity_id = ? AND era_id = ?`, args...); err != nil {
			return false, err
		}
	}
	return r.exists(ctx, entityID, eraID)
}

func (r *eraVersionRepo) SaveEntry(ctx context.Context, entityID string, eraID int, entry, entryHTML string, baseRev *int, userID string) (int, bool, error) {
	var entryArg, htmlArg any
	if entry != "" {
		entryArg = entry
	}
	if entryHTML != "" {
		htmlArg = entryHTML
	}
	q := `UPDATE entity_era_versions SET entry = ?, entry_html = ?, entry_rev = entry_rev + 1, updated_by = ?
	      WHERE entity_id = ? AND era_id = ?`
	args := []any{entryArg, htmlArg, nullIfEmpty(userID), entityID, eraID}
	if baseRev != nil {
		q += ` AND entry_rev = ?`
		args = append(args, *baseRev)
	}
	res, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, false, nil
	}
	var rev int
	if err := r.db.QueryRowContext(ctx,
		`SELECT entry_rev FROM entity_era_versions WHERE entity_id = ? AND era_id = ?`, entityID, eraID).Scan(&rev); err != nil {
		return 0, false, err
	}
	return rev, true, nil
}

func (r *eraVersionRepo) SaveFields(ctx context.Context, entityID string, eraID int, fieldsJSON *string, userID string) (bool, error) {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE entity_era_versions SET fields_data = ?, updated_by = ? WHERE entity_id = ? AND era_id = ?`,
		fieldsJSON, nullIfEmpty(userID), entityID, eraID); err != nil {
		return false, err
	}
	return r.exists(ctx, entityID, eraID)
}

func (r *eraVersionRepo) Delete(ctx context.Context, entityID string, eraID int) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM entity_era_versions WHERE entity_id = ? AND era_id = ?`, entityID, eraID)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
