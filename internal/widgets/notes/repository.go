package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// NoteRepository defines the data access contract for note operations.
type NoteRepository interface {
	Create(ctx context.Context, note *Note) error
	FindByID(ctx context.Context, id string) (*Note, error)
	Update(ctx context.Context, note *Note) error
	Delete(ctx context.Context, id string) error

	// ListByUserAndCampaign returns all notes for a user in a campaign
	// (own notes + shared notes from other users).
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error)

	// ListByEntity returns notes for a user scoped to a specific entity
	// (own notes + shared notes).
	ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error)

	// ListCampaignWide returns campaign-wide notes not scoped to any entity
	// (own notes + shared notes).
	ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error)

	// AcquireLock attempts to set locked_by/locked_at for a note. Returns
	// true if the lock was acquired, false if another user holds a live lock.
	AcquireLock(ctx context.Context, noteID, userID string) (bool, error)

	// ReleaseLock clears the lock on a note (only if held by the given user).
	ReleaseLock(ctx context.Context, noteID, userID string) error

	// ForceReleaseLock clears the lock regardless of who holds it.
	ForceReleaseLock(ctx context.Context, noteID string) error

	// RefreshLock updates locked_at to keep the lock alive (heartbeat).
	RefreshLock(ctx context.Context, noteID, userID string) error

	// CreateVersion inserts a version snapshot.
	CreateVersion(ctx context.Context, v *NoteVersion) error

	// ListVersions returns version history for a note, newest first.
	ListVersions(ctx context.Context, noteID string, limit int) ([]NoteVersion, error)

	// FindVersionByID retrieves a specific version.
	FindVersionByID(ctx context.Context, id string) (*NoteVersion, error)

	// PruneVersions deletes the oldest versions beyond the keep count.
	PruneVersions(ctx context.Context, noteID string, keep int) error
}

// noteRepository is the MariaDB implementation of NoteRepository.
type noteRepository struct {
	db *sql.DB
}

// NewNoteRepository creates a new MariaDB-backed note repository.
func NewNoteRepository(db *sql.DB) NoteRepository {
	return &noteRepository{db: db}
}

// noteColumns is the SELECT column list for notes queries.
const noteColumns = `id, campaign_id, user_id, entity_id, title, content,
	entry, entry_html, color, pinned, is_shared, last_edited_by,
	locked_by, locked_at, created_at, updated_at`

// Create inserts a new note into the database.
func (r *noteRepository) Create(ctx context.Context, note *Note) error {
	contentJSON, err := json.Marshal(note.Content)
	if err != nil {
		return fmt.Errorf("marshaling note content: %w", err)
	}

	query := `INSERT INTO notes
		(id, campaign_id, user_id, entity_id, title, content, entry, entry_html,
		 color, pinned, is_shared, last_edited_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		note.ID, note.CampaignID, note.UserID, note.EntityID,
		note.Title, contentJSON, note.Entry, note.EntryHTML,
		note.Color, note.Pinned, note.IsShared, note.LastEditedBy,
	)
	if err != nil {
		return fmt.Errorf("inserting note: %w", err)
	}
	return nil
}

// FindByID retrieves a note by its ID.
func (r *noteRepository) FindByID(ctx context.Context, id string) (*Note, error) {
	query := `SELECT ` + noteColumns + ` FROM notes WHERE id = ?`
	note, err := r.scanNote(r.db.QueryRowContext(ctx, query, id))
	if err != nil {
		return nil, err
	}
	return note, nil
}

// Update saves changes to an existing note.
func (r *noteRepository) Update(ctx context.Context, note *Note) error {
	contentJSON, err := json.Marshal(note.Content)
	if err != nil {
		return fmt.Errorf("marshaling note content: %w", err)
	}

	query := `UPDATE notes
		SET title = ?, content = ?, entry = ?, entry_html = ?,
		    color = ?, pinned = ?, is_shared = ?, last_edited_by = ?,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		note.Title, contentJSON, note.Entry, note.EntryHTML,
		note.Color, note.Pinned, note.IsShared, note.LastEditedBy,
		note.ID,
	)
	if err != nil {
		return fmt.Errorf("updating note: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("note not found")
	}
	return nil
}

// Delete removes a note from the database.
func (r *noteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting note: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("note not found")
	}
	return nil
}

// ListByUserAndCampaign returns own + shared notes for a user in a campaign.
func (r *noteRepository) ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error) {
	query := `SELECT ` + noteColumns + `
		FROM notes WHERE campaign_id = ? AND (user_id = ? OR is_shared = TRUE)
		ORDER BY pinned DESC, updated_at DESC`
	return r.scanNotes(ctx, query, campaignID, userID)
}

// ListByEntity returns own + shared notes scoped to a specific entity.
func (r *noteRepository) ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error) {
	query := `SELECT ` + noteColumns + `
		FROM notes WHERE campaign_id = ? AND (user_id = ? OR is_shared = TRUE) AND entity_id = ?
		ORDER BY pinned DESC, updated_at DESC`
	return r.scanNotes(ctx, query, campaignID, userID, entityID)
}

// ListCampaignWide returns own + shared campaign-wide notes (not entity-scoped).
func (r *noteRepository) ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error) {
	query := `SELECT ` + noteColumns + `
		FROM notes WHERE campaign_id = ? AND (user_id = ? OR is_shared = TRUE) AND entity_id IS NULL
		ORDER BY pinned DESC, updated_at DESC`
	return r.scanNotes(ctx, query, campaignID, userID)
}

// AcquireLock tries to take the edit lock. Stale locks (older than 5 min)
// are automatically reclaimed.
func (r *noteRepository) AcquireLock(ctx context.Context, noteID, userID string) (bool, error) {
	query := `UPDATE notes
		SET locked_by = ?, locked_at = NOW()
		WHERE id = ?
		  AND (locked_by IS NULL
		       OR locked_by = ?
		       OR locked_at < NOW() - INTERVAL 5 MINUTE)`

	result, err := r.db.ExecContext(ctx, query, userID, noteID, userID)
	if err != nil {
		return false, fmt.Errorf("acquiring note lock: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

// ReleaseLock clears the lock only if held by the specified user.
func (r *noteRepository) ReleaseLock(ctx context.Context, noteID, userID string) error {
	query := `UPDATE notes SET locked_by = NULL, locked_at = NULL
		WHERE id = ? AND locked_by = ?`
	_, err := r.db.ExecContext(ctx, query, noteID, userID)
	if err != nil {
		return fmt.Errorf("releasing note lock: %w", err)
	}
	return nil
}

// ForceReleaseLock clears the lock regardless of who holds it (owner override).
func (r *noteRepository) ForceReleaseLock(ctx context.Context, noteID string) error {
	query := `UPDATE notes SET locked_by = NULL, locked_at = NULL WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, noteID)
	if err != nil {
		return fmt.Errorf("force-releasing note lock: %w", err)
	}
	return nil
}

// RefreshLock updates locked_at to keep a lock alive (heartbeat).
func (r *noteRepository) RefreshLock(ctx context.Context, noteID, userID string) error {
	query := `UPDATE notes SET locked_at = NOW()
		WHERE id = ? AND locked_by = ?`
	result, err := r.db.ExecContext(ctx, query, noteID, userID)
	if err != nil {
		return fmt.Errorf("refreshing note lock: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewConflict("lock not held by this user")
	}
	return nil
}

// CreateVersion inserts a version history snapshot.
func (r *noteRepository) CreateVersion(ctx context.Context, v *NoteVersion) error {
	contentJSON, err := json.Marshal(v.Content)
	if err != nil {
		return fmt.Errorf("marshaling version content: %w", err)
	}

	query := `INSERT INTO note_versions (id, note_id, user_id, title, content, entry, entry_html)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		v.ID, v.NoteID, v.UserID, v.Title, contentJSON, v.Entry, v.EntryHTML,
	)
	if err != nil {
		return fmt.Errorf("inserting note version: %w", err)
	}
	return nil
}

// ListVersions returns version history for a note, newest first.
func (r *noteRepository) ListVersions(ctx context.Context, noteID string, limit int) ([]NoteVersion, error) {
	query := `SELECT id, note_id, user_id, title, content, entry, entry_html, created_at
		FROM note_versions WHERE note_id = ?
		ORDER BY created_at DESC LIMIT ?`

	rows, err := r.db.QueryContext(ctx, query, noteID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying note versions: %w", err)
	}
	defer rows.Close()

	var versions []NoteVersion
	for rows.Next() {
		v := NoteVersion{}
		var contentRaw []byte
		if err := rows.Scan(&v.ID, &v.NoteID, &v.UserID, &v.Title,
			&contentRaw, &v.Entry, &v.EntryHTML, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning note version: %w", err)
		}
		if len(contentRaw) > 0 {
			json.Unmarshal(contentRaw, &v.Content)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// FindVersionByID retrieves a specific version by its ID.
func (r *noteRepository) FindVersionByID(ctx context.Context, id string) (*NoteVersion, error) {
	query := `SELECT id, note_id, user_id, title, content, entry, entry_html, created_at
		FROM note_versions WHERE id = ?`

	v := &NoteVersion{}
	var contentRaw []byte
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&v.ID, &v.NoteID, &v.UserID, &v.Title,
		&contentRaw, &v.Entry, &v.EntryHTML, &v.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("note version not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning note version: %w", err)
	}
	if len(contentRaw) > 0 {
		json.Unmarshal(contentRaw, &v.Content)
	}
	return v, nil
}

// PruneVersions removes the oldest versions beyond the keep count.
func (r *noteRepository) PruneVersions(ctx context.Context, noteID string, keep int) error {
	query := `DELETE FROM note_versions
		WHERE note_id = ? AND id NOT IN (
			SELECT id FROM (
				SELECT id FROM note_versions WHERE note_id = ?
				ORDER BY created_at DESC LIMIT ?
			) AS recent
		)`
	_, err := r.db.ExecContext(ctx, query, noteID, noteID, keep)
	if err != nil {
		return fmt.Errorf("pruning note versions: %w", err)
	}
	return nil
}

// scanNote scans a single note row including all new columns.
func (r *noteRepository) scanNote(row *sql.Row) (*Note, error) {
	n := &Note{}
	var contentRaw []byte

	err := row.Scan(
		&n.ID, &n.CampaignID, &n.UserID, &n.EntityID,
		&n.Title, &contentRaw, &n.Entry, &n.EntryHTML,
		&n.Color, &n.Pinned, &n.IsShared, &n.LastEditedBy,
		&n.LockedBy, &n.LockedAt,
		&n.CreatedAt, &n.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("note not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning note: %w", err)
	}

	if len(contentRaw) > 0 {
		if err := json.Unmarshal(contentRaw, &n.Content); err != nil {
			return nil, fmt.Errorf("unmarshaling note content: %w", err)
		}
	}
	return n, nil
}

// scanNotes runs a query and scans multiple note rows.
func (r *noteRepository) scanNotes(ctx context.Context, query string, args ...any) ([]Note, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		n := Note{}
		var contentRaw []byte

		if err := rows.Scan(
			&n.ID, &n.CampaignID, &n.UserID, &n.EntityID,
			&n.Title, &contentRaw, &n.Entry, &n.EntryHTML,
			&n.Color, &n.Pinned, &n.IsShared, &n.LastEditedBy,
			&n.LockedBy, &n.LockedAt,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning note row: %w", err)
		}

		if len(contentRaw) > 0 {
			json.Unmarshal(contentRaw, &n.Content)
		}
		notes = append(notes, n)
	}
	return notes, rows.Err()
}
