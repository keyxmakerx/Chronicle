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

	// ListByUserAndCampaign returns all notes for a user in a campaign.
	ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error)

	// ListByEntity returns notes for a user scoped to a specific entity.
	ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error)

	// ListCampaignWide returns notes for a user that are not entity-scoped.
	ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error)
}

// noteRepository is the MariaDB implementation of NoteRepository.
type noteRepository struct {
	db *sql.DB
}

// NewNoteRepository creates a new MariaDB-backed note repository.
func NewNoteRepository(db *sql.DB) NoteRepository {
	return &noteRepository{db: db}
}

// Create inserts a new note into the database.
func (r *noteRepository) Create(ctx context.Context, note *Note) error {
	contentJSON, err := json.Marshal(note.Content)
	if err != nil {
		return fmt.Errorf("marshaling note content: %w", err)
	}

	query := `INSERT INTO notes (id, campaign_id, user_id, entity_id, title, content, color, pinned)
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = r.db.ExecContext(ctx, query,
		note.ID, note.CampaignID, note.UserID, note.EntityID,
		note.Title, contentJSON, note.Color, note.Pinned,
	)
	if err != nil {
		return fmt.Errorf("inserting note: %w", err)
	}
	return nil
}

// FindByID retrieves a note by its ID.
func (r *noteRepository) FindByID(ctx context.Context, id string) (*Note, error) {
	query := `SELECT id, campaign_id, user_id, entity_id, title, content, color, pinned, created_at, updated_at
	          FROM notes WHERE id = ?`

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

	query := `UPDATE notes SET title = ?, content = ?, color = ?, pinned = ?, updated_at = CURRENT_TIMESTAMP
	          WHERE id = ?`

	result, err := r.db.ExecContext(ctx, query, note.Title, contentJSON, note.Color, note.Pinned, note.ID)
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

// ListByUserAndCampaign returns all notes for a user in a campaign, pinned first.
func (r *noteRepository) ListByUserAndCampaign(ctx context.Context, userID, campaignID string) ([]Note, error) {
	query := `SELECT id, campaign_id, user_id, entity_id, title, content, color, pinned, created_at, updated_at
	          FROM notes WHERE user_id = ? AND campaign_id = ?
	          ORDER BY pinned DESC, updated_at DESC`

	return r.scanNotes(ctx, query, userID, campaignID)
}

// ListByEntity returns notes for a user scoped to a specific entity.
func (r *noteRepository) ListByEntity(ctx context.Context, userID, campaignID, entityID string) ([]Note, error) {
	query := `SELECT id, campaign_id, user_id, entity_id, title, content, color, pinned, created_at, updated_at
	          FROM notes WHERE user_id = ? AND campaign_id = ? AND entity_id = ?
	          ORDER BY pinned DESC, updated_at DESC`

	return r.scanNotes(ctx, query, userID, campaignID, entityID)
}

// ListCampaignWide returns campaign-wide notes (not scoped to any entity).
func (r *noteRepository) ListCampaignWide(ctx context.Context, userID, campaignID string) ([]Note, error) {
	query := `SELECT id, campaign_id, user_id, entity_id, title, content, color, pinned, created_at, updated_at
	          FROM notes WHERE user_id = ? AND campaign_id = ? AND entity_id IS NULL
	          ORDER BY pinned DESC, updated_at DESC`

	return r.scanNotes(ctx, query, userID, campaignID)
}

// scanNote scans a single note row.
func (r *noteRepository) scanNote(row *sql.Row) (*Note, error) {
	n := &Note{}
	var contentRaw []byte

	err := row.Scan(
		&n.ID, &n.CampaignID, &n.UserID, &n.EntityID,
		&n.Title, &contentRaw, &n.Color, &n.Pinned,
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
			&n.Title, &contentRaw, &n.Color, &n.Pinned,
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
