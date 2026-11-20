package systems

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// ReadingRepository holds what lets a campaign read its game system in its
// own words: ties from a campaign entry to the standard entry it replaces,
// the standard entries left out, the old names of renamed entries and the
// lines the players were told. Every method is scoped by campaign.
type ReadingRepository interface {
	// SetStandsFor ties the entry to a standard slug with the snapshot, or
	// with standsFor "" unties it and drops the snapshot. A second stand-in
	// for the same standard entry is a conflict.
	SetStandsFor(ctx context.Context, campaignID string, id int64, standsFor, seen string) error
	// SetStandsForSeen replaces the snapshot of an entry that stays tied.
	SetStandsForSeen(ctx context.Context, campaignID string, id int64, seen string) error
	// FindStandIn returns the entry tied to the standard slug; nil when none.
	FindStandIn(ctx context.Context, campaignID, systemID, fieldKey, slug string) (*SystemEntry, error)

	// ListLeftOut returns the left-out standard entries of a field ("" = all).
	ListLeftOut(ctx context.Context, campaignID, systemID, fieldKey string) ([]LeftOut, error)
	// PutLeftOut leaves a standard entry out, or changes its sentence.
	PutLeftOut(ctx context.Context, l *LeftOut) error
	// DeleteLeftOut brings it back; false when it was not left out.
	DeleteLeftOut(ctx context.Context, campaignID, systemID, fieldKey, slug string) (bool, error)

	// ListOldNames returns the old names in a field ("" = all).
	ListOldNames(ctx context.Context, campaignID, systemID, fieldKey string) ([]EntryOldName, error)
	// FindOldName is a case-insensitive lookup; nil when no entry had it.
	FindOldName(ctx context.Context, campaignID, systemID, fieldKey, name string) (*EntryOldName, error)
	// AddOldName records a name an entry gave up. Recording one it already
	// holds is a no-op.
	AddOldName(ctx context.Context, n *EntryOldName) error
	// DeleteOldName frees an old name; false when there was none.
	DeleteOldName(ctx context.Context, campaignID string, id int64) (bool, error)

	// AddNote stores a line the players were told.
	AddNote(ctx context.Context, n *EntryNote) error
	// ListNotes returns an entry's lines, newest first, at most limit.
	ListNotes(ctx context.Context, campaignID string, entryID int64, limit int) ([]EntryNote, error)
}

// errStandInTaken is the refusal for a second stand-in, whichever write hit
// the unique key.
var errStandInTaken = apperror.NewConflict("another entry already takes this standard entry's place")

// isStandInConflict reports whether a duplicate-key error came from the one
// stand-in per standard entry rule rather than from a slug or name.
func isStandInConflict(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062 && strings.Contains(me.Message, "stands_for")
}

func (r *systemEntryRepository) SetStandsFor(ctx context.Context, campaignID string, id int64, standsFor, seen string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_system_entries SET stands_for = NULLIF(?, ''), stands_for_seen = NULLIF(?, '')
		  WHERE campaign_id = ? AND id = ?`, standsFor, seen, campaignID, id)
	if err != nil {
		if isStandInConflict(err) {
			return errStandInTaken
		}
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *systemEntryRepository) SetStandsForSeen(ctx context.Context, campaignID string, id int64, seen string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE campaign_system_entries SET stands_for_seen = NULLIF(?, '')
		  WHERE campaign_id = ? AND id = ? AND stands_for IS NOT NULL`, seen, campaignID, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *systemEntryRepository) FindStandIn(ctx context.Context, campaignID, systemID, fieldKey, slug string) (*SystemEntry, error) {
	e, err := scanSystemEntry(r.db.QueryRowContext(ctx,
		`SELECT `+systemEntryColumns+` FROM campaign_system_entries
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND stands_for = ? LIMIT 1`,
		campaignID, systemID, fieldKey, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return e, nil
}

func (r *systemEntryRepository) ListLeftOut(ctx context.Context, campaignID, systemID, fieldKey string) ([]LeftOut, error) {
	q := `SELECT campaign_id, system_id, field_key, slug, sentence, COALESCE(created_by, ''), created_at
	        FROM campaign_system_left_out WHERE campaign_id = ? AND system_id = ?`
	args := []any{campaignID, systemID}
	if fieldKey != "" {
		q += ` AND field_key = ?`
		args = append(args, fieldKey)
	}
	q += ` ORDER BY field_key, slug`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []LeftOut{}
	for rows.Next() {
		var l LeftOut
		if err := rows.Scan(&l.CampaignID, &l.SystemID, &l.FieldKey, &l.Slug, &l.Sentence, &l.CreatedBy, &l.CreatedAt); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *systemEntryRepository) PutLeftOut(ctx context.Context, l *LeftOut) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_system_left_out (campaign_id, system_id, field_key, slug, sentence, created_by)
		 VALUES (?, ?, ?, ?, ?, NULLIF(?, ''))
		 ON DUPLICATE KEY UPDATE sentence = VALUES(sentence)`,
		l.CampaignID, l.SystemID, l.FieldKey, l.Slug, l.Sentence, l.CreatedBy)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *systemEntryRepository) DeleteLeftOut(ctx context.Context, campaignID, systemID, fieldKey, slug string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_system_left_out WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND slug = ?`,
		campaignID, systemID, fieldKey, slug)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

const entryOldNameColumns = `id, campaign_id, system_id, field_key, name, entry_id, created_at`

func scanOldName(s rowScanner) (*EntryOldName, error) {
	var n EntryOldName
	if err := s.Scan(&n.ID, &n.CampaignID, &n.SystemID, &n.FieldKey, &n.Name, &n.EntryID, &n.CreatedAt); err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *systemEntryRepository) ListOldNames(ctx context.Context, campaignID, systemID, fieldKey string) ([]EntryOldName, error) {
	q := `SELECT ` + entryOldNameColumns + ` FROM campaign_system_entry_names WHERE campaign_id = ? AND system_id = ?`
	args := []any{campaignID, systemID}
	if fieldKey != "" {
		q += ` AND field_key = ?`
		args = append(args, fieldKey)
	}
	q += ` ORDER BY field_key, name`
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []EntryOldName{}
	for rows.Next() {
		n, err := scanOldName(rows)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, *n)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}

func (r *systemEntryRepository) FindOldName(ctx context.Context, campaignID, systemID, fieldKey, name string) (*EntryOldName, error) {
	// The column collation is case-insensitive, so = matches any casing.
	n, err := scanOldName(r.db.QueryRowContext(ctx,
		`SELECT `+entryOldNameColumns+` FROM campaign_system_entry_names
		  WHERE campaign_id = ? AND system_id = ? AND field_key = ? AND name = ?`,
		campaignID, systemID, fieldKey, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	return n, nil
}

func (r *systemEntryRepository) AddOldName(ctx context.Context, n *EntryOldName) error {
	// The service has already refused a name another entry holds, so a
	// duplicate here is the same entry recording it again.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_system_entry_names (campaign_id, system_id, field_key, name, entry_id)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE entry_id = entry_id`,
		n.CampaignID, n.SystemID, n.FieldKey, n.Name, n.EntryID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	return nil
}

func (r *systemEntryRepository) DeleteOldName(ctx context.Context, campaignID string, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM campaign_system_entry_names WHERE campaign_id = ? AND id = ?`, campaignID, id)
	if err != nil {
		return false, apperror.NewInternal(err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (r *systemEntryRepository) AddNote(ctx context.Context, n *EntryNote) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO campaign_system_entry_notes (campaign_id, entry_id, line, created_by)
		 VALUES (?, ?, ?, NULLIF(?, ''))`,
		n.CampaignID, n.EntryID, n.Line, n.CreatedBy)
	if err != nil {
		return apperror.NewInternal(err)
	}
	n.ID, _ = res.LastInsertId()
	return nil
}

func (r *systemEntryRepository) ListNotes(ctx context.Context, campaignID string, entryID int64, limit int) ([]EntryNote, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, campaign_id, entry_id, line, COALESCE(created_by, ''), created_at
		   FROM campaign_system_entry_notes WHERE campaign_id = ? AND entry_id = ?
		  ORDER BY created_at DESC, id DESC LIMIT ?`, campaignID, entryID, limit)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	defer func() { _ = rows.Close() }()
	out := []EntryNote{}
	for rows.Next() {
		var n EntryNote
		if err := rows.Scan(&n.ID, &n.CampaignID, &n.EntryID, &n.Line, &n.CreatedBy, &n.CreatedAt); err != nil {
			return nil, apperror.NewInternal(err)
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.NewInternal(err)
	}
	return out, nil
}
