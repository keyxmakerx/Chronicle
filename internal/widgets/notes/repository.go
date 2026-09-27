package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// NoteRepository defines the data access contract for note operations.
type NoteRepository interface {
	Create(ctx context.Context, note *Note) error
	FindByID(ctx context.Context, id string) (*Note, error)
	Update(ctx context.Context, note *Note) error
	Delete(ctx context.Context, id string) error

	// ListVisible returns the notes in campaignID that v can read (the SQL
	// twin of Note.CanView), narrowed by scope. Pinned first, then newest.
	ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error)

	// ListTree returns the id, parent, owner and folder flag of every note in
	// the campaign, with no visibility filter. Internal structure only (cycle
	// and ownership checks when filing or deleting folders); never returned
	// to a client.
	ListTree(ctx context.Context, campaignID string) ([]TreeRow, error)

	// ReparentToTop moves the given notes out of any folder.
	ReparentToTop(ctx context.Context, ids []string) error

	// FindByIDs loads the given notes, in no particular order. Unfiltered,
	// like FindByID: the caller applies CanView to each.
	FindByIDs(ctx context.Context, ids []string) ([]Note, error)

	// ListVisibleLinking returns the notes v can see whose body links to the
	// note (kind LinkNote) or page (LinkPage) targetID, newest first.
	ListVisibleLinking(ctx context.Context, campaignID string, v permissions.Viewer, kind, targetID string) ([]Note, error)

	// ListSharedByCampaign returns every campaign-wide-shared note in the
	// campaign regardless of which user owns it.
	//
	// Deliberately has NO user filter: it exists so campaign export can
	// capture the shared note corpus, which is campaign data rather than
	// per-user data. Every other list method here is user-scoped and must
	// stay that way (ADR-013). Callers of this one must be owner-gated —
	// today the only caller is the campaign export adapter, and no HTTP
	// route reaches it directly.
	ListSharedByCampaign(ctx context.Context, campaignID string) ([]Note, error)

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

// AttachmentRepository defines the data access contract for note attachments.
type AttachmentRepository interface {
	CreateAttachment(ctx context.Context, a *NoteAttachment) error
	ListByNote(ctx context.Context, noteID string) ([]NoteAttachment, error)
	FindAttachmentByID(ctx context.Context, id string) (*NoteAttachment, error)
	DeleteAttachment(ctx context.Context, id string) error
	UpdateTranscript(ctx context.Context, id string, transcript string) error

	// NotesWithAttachments returns the ids of the campaign's notes that have
	// at least one attachment. Unfiltered: callers intersect it with notes
	// the viewer can see.
	NotesWithAttachments(ctx context.Context, campaignID string) (map[string]bool, error)
}

// noteRepository is the MariaDB implementation of NoteRepository.
type noteRepository struct {
	db *sql.DB
}

// NewNoteRepository creates a new MariaDB-backed note repository.
func NewNoteRepository(db *sql.DB) NoteRepository {
	return &noteRepository{db: db}
}

// NewAttachmentRepository creates a new MariaDB-backed attachment repository.
func NewAttachmentRepository(db *sql.DB) AttachmentRepository {
	return &noteRepository{db: db}
}

// noteColumns is the SELECT column list for notes queries.
const noteColumns = `id, campaign_id, user_id, entity_id, linked_note_id, parent_id, is_folder,
	title, content, entry, entry_html, color, pinned, archived_at, is_shared, shared_with, shared_with_gm,
	last_edited_by, locked_by, locked_at, created_at, updated_at`

// ListScope narrows ListVisible. The zero value lists every visible note.
type ListScope struct {
	// Kind is "" (all), "campaign" (Journal notes and folders: no page),
	// "entity" (the jots on EntityID) or "jots" (every jot on any page).
	Kind     string
	EntityID string
}

// TreeRow is the structural shape of one note, for folder safety checks.
type TreeRow struct {
	ID       string
	ParentID *string
	UserID   string
	IsFolder bool
}

// Create inserts a new note into the database.
func (r *noteRepository) Create(ctx context.Context, note *Note) error {
	contentJSON, err := json.Marshal(note.Content)
	if err != nil {
		return fmt.Errorf("marshaling note content: %w", err)
	}

	query := `INSERT INTO notes
		(id, campaign_id, user_id, entity_id, linked_note_id, parent_id, is_folder,
		 title, content, entry, entry_html,
		 color, pinned, archived_at, is_shared, shared_with, shared_with_gm, last_edited_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	sharedWithJSON := MarshalSharedWith(note.SharedWith)

	_, err = r.db.ExecContext(ctx, query,
		note.ID, note.CampaignID, note.UserID, note.EntityID, note.LinkedNoteID,
		note.ParentID, note.IsFolder,
		note.Title, contentJSON, note.Entry, note.EntryHTML,
		note.Color, note.Pinned, note.ArchivedAt, note.IsShared, sharedWithJSON, note.SharedWithGM, note.LastEditedBy,
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

	sharedWithJSON := MarshalSharedWith(note.SharedWith)

	query := `UPDATE notes
		SET title = ?, content = ?, entry = ?, entry_html = ?,
		    color = ?, pinned = ?, archived_at = ?, is_shared = ?, shared_with = ?, shared_with_gm = ?,
		    linked_note_id = ?, last_edited_by = ?, parent_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query,
		note.Title, contentJSON, note.Entry, note.EntryHTML,
		note.Color, note.Pinned, note.ArchivedAt, note.IsShared, sharedWithJSON, note.SharedWithGM,
		note.LinkedNoteID, note.LastEditedBy, note.ParentID, note.ID,
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

// visibleFilter is the SQL twin of Note.CanView: the owner, a party share, a
// share naming the viewer, or a GM share when the viewer is a GM. The caller
// has already refused an anonymous viewer, so an empty user_id never matches.
func visibleFilter(v permissions.Viewer) (string, []any) {
	return `(user_id = ? OR is_shared = TRUE OR JSON_CONTAINS(shared_with, JSON_QUOTE(?), '$')
		OR (shared_with_gm = TRUE AND ?))`,
		[]any{v.UserID(), v.UserID(), permissions.CanSeeDmOnly(v.Role())}
}

// ListVisible returns the notes v can read in campaignID, narrowed by scope.
func (r *noteRepository) ListVisible(ctx context.Context, campaignID string, v permissions.Viewer, scope ListScope) ([]Note, error) {
	if v.UserID() == "" {
		return nil, nil
	}
	vis, visArgs := visibleFilter(v)
	where := `campaign_id = ? AND ` + vis
	args := append([]any{campaignID}, visArgs...)
	switch scope.Kind {
	case "":
	case "campaign":
		where += ` AND entity_id IS NULL`
	case "jots":
		where += ` AND entity_id IS NOT NULL`
	case "entity":
		where += ` AND entity_id = ?`
		args = append(args, scope.EntityID)
	default:
		return nil, fmt.Errorf("unknown note list scope %q", scope.Kind)
	}
	query := `SELECT ` + noteColumns + ` FROM notes WHERE ` + where + `
		ORDER BY pinned DESC, updated_at DESC`
	return r.scanNotes(ctx, query, args...)
}

// FindByIDs loads the given notes, unfiltered.
func (r *noteRepository) FindByIDs(ctx context.Context, ids []string) ([]Note, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return r.scanNotes(ctx, `SELECT `+noteColumns+` FROM notes WHERE id IN (`+placeholders+`)`, args...)
}

// linkAttr is the anchor attribute each link kind is stored under.
var linkAttr = map[string]string{LinkNote: "data-note-id", LinkPage: "data-mention-id"}

// ListVisibleLinking returns the visible notes whose entry_html carries an
// anchor pointing at targetID. The id is checked against idPattern, so it
// can hold no LIKE wildcard.
func (r *noteRepository) ListVisibleLinking(ctx context.Context, campaignID string, v permissions.Viewer, kind, targetID string) ([]Note, error) {
	a, ok := linkAttr[kind]
	if !ok || !idPattern.MatchString(targetID) || v.UserID() == "" {
		return nil, nil
	}
	vis, visArgs := visibleFilter(v)
	args := append([]any{campaignID}, visArgs...)
	args = append(args, "%"+a+`="`+targetID+`"%`)
	query := `SELECT ` + noteColumns + ` FROM notes
		WHERE campaign_id = ? AND ` + vis + ` AND entry_html LIKE ?
		ORDER BY updated_at DESC LIMIT 500`
	return r.scanNotes(ctx, query, args...)
}

// ListTree returns every note's structural row in the campaign, unfiltered.
func (r *noteRepository) ListTree(ctx context.Context, campaignID string) ([]TreeRow, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, parent_id, user_id, is_folder FROM notes WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("querying note tree: %w", err)
	}
	defer rows.Close()
	var out []TreeRow
	for rows.Next() {
		var t TreeRow
		if err := rows.Scan(&t.ID, &t.ParentID, &t.UserID, &t.IsFolder); err != nil {
			return nil, fmt.Errorf("scanning note tree row: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ReparentToTop clears parent_id on the given notes.
func (r *noteRepository) ReparentToTop(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	if _, err := r.db.ExecContext(ctx,
		`UPDATE notes SET parent_id = NULL WHERE id IN (`+placeholders+`)`, args...); err != nil {
		return fmt.Errorf("reparenting notes: %w", err)
	}
	return nil
}

// ListSharedByCampaign returns all campaign-wide-shared notes for a campaign,
// across every owner. Ordered oldest-first so an export is stable between runs
// and folders (created before the notes filed into them) tend to precede their
// children; the importer does not rely on that ordering.
func (r *noteRepository) ListSharedByCampaign(ctx context.Context, campaignID string) ([]Note, error) {
	query := `SELECT ` + noteColumns + `
		FROM notes WHERE campaign_id = ? AND is_shared = TRUE
		ORDER BY created_at ASC, id ASC`
	return r.scanNotes(ctx, query, campaignID)
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
			if err := json.Unmarshal(contentRaw, &v.Content); err != nil {
				return nil, fmt.Errorf("unmarshaling version content: %w", err)
			}
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
		if err := json.Unmarshal(contentRaw, &v.Content); err != nil {
			return nil, fmt.Errorf("unmarshaling version content: %w", err)
		}
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
	var sharedWithRaw *string

	err := row.Scan(
		&n.ID, &n.CampaignID, &n.UserID, &n.EntityID, &n.LinkedNoteID,
		&n.ParentID, &n.IsFolder,
		&n.Title, &contentRaw, &n.Entry, &n.EntryHTML,
		&n.Color, &n.Pinned, &n.ArchivedAt, &n.IsShared, &sharedWithRaw, &n.SharedWithGM,
		&n.LastEditedBy, &n.LockedBy, &n.LockedAt,
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
	n.SharedWith = UnmarshalSharedWith(sharedWithRaw)
	n.derive()
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
		var sharedWithRaw *string

		if err := rows.Scan(
			&n.ID, &n.CampaignID, &n.UserID, &n.EntityID, &n.LinkedNoteID,
			&n.ParentID, &n.IsFolder,
			&n.Title, &contentRaw, &n.Entry, &n.EntryHTML,
			&n.Color, &n.Pinned, &n.ArchivedAt, &n.IsShared, &sharedWithRaw, &n.SharedWithGM,
			&n.LastEditedBy, &n.LockedBy, &n.LockedAt,
			&n.CreatedAt, &n.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning note row: %w", err)
		}

		if len(contentRaw) > 0 {
			if err := json.Unmarshal(contentRaw, &n.Content); err != nil {
				return nil, fmt.Errorf("unmarshaling note content: %w", err)
			}
		}
		n.SharedWith = UnmarshalSharedWith(sharedWithRaw)
		n.derive()
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

// --- Attachment Repository Implementation ---

// CreateAttachment inserts a new note attachment record.
func (r *noteRepository) CreateAttachment(ctx context.Context, a *NoteAttachment) error {
	query := `INSERT INTO note_attachments
		(id, note_id, campaign_id, file_path, original_name, mime_type, file_size, duration_secs, transcript)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := r.db.ExecContext(ctx, query,
		a.ID, a.NoteID, a.CampaignID, a.FilePath, a.OriginalName,
		a.MimeType, a.FileSize, a.DurationSecs, a.Transcript,
	)
	if err != nil {
		return fmt.Errorf("inserting note attachment: %w", err)
	}
	return nil
}

// ListByNote returns all attachments for a given note, newest first.
func (r *noteRepository) ListByNote(ctx context.Context, noteID string) ([]NoteAttachment, error) {
	query := `SELECT id, note_id, campaign_id, file_path, original_name,
		mime_type, file_size, duration_secs, transcript, created_at, updated_at
		FROM note_attachments WHERE note_id = ?
		ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, noteID)
	if err != nil {
		return nil, fmt.Errorf("querying note attachments: %w", err)
	}
	defer rows.Close()

	var attachments []NoteAttachment
	for rows.Next() {
		var a NoteAttachment
		if err := rows.Scan(&a.ID, &a.NoteID, &a.CampaignID, &a.FilePath,
			&a.OriginalName, &a.MimeType, &a.FileSize, &a.DurationSecs,
			&a.Transcript, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning note attachment: %w", err)
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}

// FindAttachmentByID retrieves a single attachment by ID.
func (r *noteRepository) FindAttachmentByID(ctx context.Context, id string) (*NoteAttachment, error) {
	query := `SELECT id, note_id, campaign_id, file_path, original_name,
		mime_type, file_size, duration_secs, transcript, created_at, updated_at
		FROM note_attachments WHERE id = ?`

	var a NoteAttachment
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&a.ID, &a.NoteID, &a.CampaignID, &a.FilePath, &a.OriginalName,
		&a.MimeType, &a.FileSize, &a.DurationSecs, &a.Transcript,
		&a.CreatedAt, &a.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.NewNotFound("attachment not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning note attachment: %w", err)
	}
	return &a, nil
}

// DeleteAttachment removes an attachment record by ID.
func (r *noteRepository) DeleteAttachment(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM note_attachments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting note attachment: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("attachment not found")
	}
	return nil
}

// NotesWithAttachments returns the set of note ids with an attachment.
func (r *noteRepository) NotesWithAttachments(ctx context.Context, campaignID string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT note_id FROM note_attachments WHERE campaign_id = ?`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("querying notes with attachments: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning note id: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// UpdateTranscript sets the transcript text for an attachment.
func (r *noteRepository) UpdateTranscript(ctx context.Context, id string, transcript string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE note_attachments SET transcript = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		transcript, id,
	)
	if err != nil {
		return fmt.Errorf("updating attachment transcript: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.NewNotFound("attachment not found")
	}
	return nil
}
