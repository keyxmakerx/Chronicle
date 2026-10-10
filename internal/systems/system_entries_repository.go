package systems

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// SystemEntryRepository is the data access contract for a campaign's own
// pick-list entries. Every method is scoped by campaign, so one campaign can
// never read or write another's rows by guessing an id.
type SystemEntryRepository interface {
	// List returns the entries for a system, optionally for one field key
	// ("" = all), ordered by name. includeDirectors false hides 'directors'
	// entries.
	List(ctx context.Context, campaignID, systemID, fieldKey string, includeDirectors bool) ([]SystemEntry, error)
	// Get returns one entry; apperror not-found when absent.
	Get(ctx context.Context, campaignID string, id int64) (*SystemEntry, error)
	// FindByName is a case-insensitive name lookup within one field; nil when
	// there is none.
	FindByName(ctx context.Context, campaignID, systemID, fieldKey, name string) (*SystemEntry, error)
	// SlugExists reports whether the slug is taken in the field.
	SlugExists(ctx context.Context, campaignID, systemID, fieldKey, slug string) (bool, error)
	// Count returns how many entries a field has.
	Count(ctx context.Context, campaignID, systemID, fieldKey string) (int, error)
	// Create inserts the entry and fills its id; a duplicate slug is a conflict.
	Create(ctx context.Context, e *SystemEntry) error
	// Update replaces the editable columns of an existing entry.
	Update(ctx context.Context, e *SystemEntry) error
	// Delete removes an entry; false when there was none.
	Delete(ctx context.Context, campaignID string, id int64) (bool, error)
}

type systemEntryRepository struct {
	db *sql.DB
}

// NewSystemEntryRepository creates the MariaDB-backed repository.
func NewSystemEntryRepository(db *sql.DB) SystemEntryRepository {
	return &systemEntryRepository{db: db}
}

const systemEntryColumns = `id, campaign_id, system_id, field_key, slug, name, summary, description,
	properties, visibility, COALESCE(created_by, ''), created_at, updated_at`

func scanSystemEntry(s rowScanner) (*SystemEntry, error) {
	var e SystemEntry
	var props []byte
	if err := s.Scan(&e.ID, &e.CampaignID, &e.SystemID, &e.FieldKey, &e.Slug, &e.Name, &e.Summary,
		&e.Description, &props, &e.Visibility, &e.CreatedBy, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Properties = map[string]any{}
	if len(props) > 0 {
		// A damaged column degrades to "no details" instead of hiding the entry.
		if err := json.Unmarshal(props, &e.Properties); err != nil || e.Properties == nil {
			e.Properties = map[string]any{}
		}
	}
	return &e, nil
}

func (r *systemEntryRepository) List(ctx context.Context, campaignID, systemID, fieldKey string, includeDirectors bool) ([]SystemEntry, error) {
	q := `SELECT ` + systemEntryColumns + ` FROM campaign_system_entries WHERE campaign_id = ? AND system_id = ?`
	args := []any{campaignID, systemID}
	if fieldKey != "" {
		q += ` AND field_key = ?`
		args = append(args, fieldKey)
	}
	if !includeDirectors {
		q += ` AND visibility = 'everyone'`
	}
	q += ` ORDER BY field_key, name, id`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []SystemEntry{}
	for rows.Next() {
		e, err := scanSystemEntry(rows)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *systemEntryRepository) Get(ctx context.Context, campaignID string, id int64) (*SystemEntry, error) {
	e, err := scanSystemEntry(r.db.QueryRowContext(ctx,
		`SELECT `+systemEntryColumns+` FROM campaign_system_entries WHERE campaign_id = ? AND id = ?`, campaignID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("entry not found")
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return e, nil
}

func (r *systemEntryRepository) FindByName(ctx context.Context, campaignID, systemID, fieldKey, name string) (*SystemEntry, error) {
	// The column collation is case-insensitive, so = matches any casing.
	e, err := scanSystemEntry(r.db.QueryRowContext(ctx,
		`SELECT `+systemEntryColumns+` FROM campaign_system_entries
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND name = ? LIMIT 1`,
		campaignID, systemID, fieldKey, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return e, nil
}

func (r *systemEntryRepository) SlugExists(ctx context.Context, campaignID, systemID, fieldKey, slug string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_system_entries
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND slug = ?`,
		campaignID, systemID, fieldKey, slug).Scan(&n)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	return n > 0, nil
}

func (r *systemEntryRepository) Count(ctx context.Context, campaignID, systemID, fieldKey string) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM campaign_system_entries WHERE campaign_id = ? AND system_id = ? AND field_key = ?`,
		campaignID, systemID, fieldKey).Scan(&n)
	if err != nil {
		return 0, apperror.NewInternal(err)
	}
	return n, nil
}

func (r *systemEntryRepository) Create(ctx context.Context, e *SystemEntry) error {
	props, err := json.Marshal(e.Properties)
	if err != nil {
		return apperror.NewInternal(err)
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_system_entries
		   (campaign_id, system_id, field_key, slug, name, summary, description, properties, visibility, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''))`,
		e.CampaignID, e.SystemID, e.FieldKey, e.Slug, e.Name, e.Summary, e.Description, string(props), e.Visibility, e.CreatedBy)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1062 {
			return apperror.NewConflict("an entry with that name already exists")
		}
		return apperror.NewInternal(err)
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

func (r *systemEntryRepository) Update(ctx context.Context, e *SystemEntry) error {
	props, err := json.Marshal(e.Properties)
	if err != nil {
		return apperror.NewInternal(err)
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE campaign_system_entries
		    SET name = ?, summary = ?, description = ?, properties = ?, visibility = ?
		  WHERE campaign_id = ? AND id = ?`,
		e.Name, e.Summary, e.Description, string(props), e.Visibility, e.CampaignID, e.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *systemEntryRepository) Delete(ctx context.Context, campaignID string, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_system_entries WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
