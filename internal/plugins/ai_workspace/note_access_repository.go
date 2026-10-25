// note_access_repository.go holds the SQL for the owner's switches on which
// shared notes the AI may change (ai_note_access, a plugin migration).
package ai_workspace

import (
	"context"
	"database/sql"
	"errors"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
)

// NoteAccessStore keeps the owner's switches per campaign.
type NoteAccessStore interface {
	// Get is the campaign's switches; both off when never set.
	Get(ctx context.Context, campaignID string) (records.NoteAccess, error)
	// Set saves them, recording who changed them.
	Set(ctx context.Context, campaignID, userID string, a records.NoteAccess) error
}

type noteAccessRepository struct{ db *sql.DB }

// NewNoteAccessRepository creates the note access store.
func NewNoteAccessRepository(db *sql.DB) NoteAccessStore { return &noteAccessRepository{db: db} }

func (r *noteAccessRepository) Get(ctx context.Context, campaignID string) (records.NoteAccess, error) {
	var a records.NoteAccess
	err := r.db.QueryRowContext(ctx,
		`SELECT co_dms, scribes FROM ai_note_access WHERE campaign_id = ?`, campaignID).Scan(&a.CoDMs, &a.Scribes)
	if errors.Is(err, sql.ErrNoRows) {
		return records.NoteAccess{}, nil
	}
	if err != nil {
		return records.NoteAccess{}, memoryDBError("reading note access", err)
	}
	return a, nil
}

func (r *noteAccessRepository) Set(ctx context.Context, campaignID, userID string, a records.NoteAccess) error {
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO ai_note_access (campaign_id, co_dms, scribes, updated_by) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE co_dms = VALUES(co_dms), scribes = VALUES(scribes), updated_by = VALUES(updated_by)`,
		campaignID, a.CoDMs, a.Scribes, userID); err != nil {
		return memoryDBError("saving note access", err)
	}
	return nil
}
