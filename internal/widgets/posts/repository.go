package posts

import (
	"context"
	"database/sql"
	"encoding/json"
)

// PostRepository defines the data access interface for entity posts.
type PostRepository interface {
	// Create inserts a new post into the database.
	Create(ctx context.Context, post *Post) error
	// FindByID returns a single post by its ID.
	FindByID(ctx context.Context, id string) (*Post, error)
	// ListByEntity returns all posts for an entity, ordered by sort_order.
	// When includeDMOnly is false, DM-only posts are filtered out.
	ListByEntity(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error)
	// Update modifies an existing post.
	Update(ctx context.Context, post *Post) error
	// Delete removes a post by its ID.
	Delete(ctx context.Context, id string) error
	// Reorder updates the sort_order for a list of post IDs within an entity.
	Reorder(ctx context.Context, entityID string, postIDs []string) error
}

type postRepository struct {
	db *sql.DB
}

// NewPostRepository creates a new MariaDB-backed post repository.
func NewPostRepository(db *sql.DB) PostRepository {
	return &postRepository{db: db}
}

func (r *postRepository) Create(ctx context.Context, post *Post) error {
	entryJSON, err := marshalEntry(post.Entry)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx,
		`INSERT INTO entity_posts (id, entity_id, campaign_id, name, entry, entry_html, is_private, sort_order, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		post.ID, post.EntityID, post.CampaignID, post.Name,
		entryJSON, post.EntryHTML, post.IsPrivate, post.SortOrder, post.CreatedBy,
	)
	return err
}

func (r *postRepository) FindByID(ctx context.Context, id string) (*Post, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, entity_id, campaign_id, name, entry, entry_html, is_private, sort_order, created_by, created_at, updated_at
		 FROM entity_posts WHERE id = ?`, id,
	)
	return scanPost(row)
}

func (r *postRepository) ListByEntity(ctx context.Context, entityID string, includeDMOnly bool) ([]Post, error) {
	query := `SELECT id, entity_id, campaign_id, name, entry, entry_html, is_private, sort_order, created_by, created_at, updated_at
		 FROM entity_posts WHERE entity_id = ?`
	if !includeDMOnly {
		query += ` AND is_private = FALSE`
	}
	query += ` ORDER BY sort_order ASC, created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		p, err := scanPostFromRows(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, *p)
	}
	return posts, rows.Err()
}

func (r *postRepository) Update(ctx context.Context, post *Post) error {
	entryJSON, err := marshalEntry(post.Entry)
	if err != nil {
		return err
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE entity_posts SET name = ?, entry = ?, entry_html = ?, is_private = ? WHERE id = ?`,
		post.Name, entryJSON, post.EntryHTML, post.IsPrivate, post.ID,
	)
	return err
}

func (r *postRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM entity_posts WHERE id = ?`, id)
	return err
}

func (r *postRepository) Reorder(ctx context.Context, entityID string, postIDs []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE entity_posts SET sort_order = ? WHERE id = ? AND entity_id = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for i, id := range postIDs {
		if _, err := stmt.ExecContext(ctx, i, id, entityID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// scanPost scans a single post from a *sql.Row.
func scanPost(row *sql.Row) (*Post, error) {
	var p Post
	var entryJSON sql.NullString
	var entryHTML sql.NullString

	err := row.Scan(
		&p.ID, &p.EntityID, &p.CampaignID, &p.Name,
		&entryJSON, &entryHTML,
		&p.IsPrivate, &p.SortOrder, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if entryJSON.Valid {
		p.Entry = json.RawMessage(entryJSON.String)
	}
	if entryHTML.Valid {
		p.EntryHTML = &entryHTML.String
	}
	return &p, nil
}

// scanPostFromRows scans a single post from a *sql.Rows.
func scanPostFromRows(rows *sql.Rows) (*Post, error) {
	var p Post
	var entryJSON sql.NullString
	var entryHTML sql.NullString

	err := rows.Scan(
		&p.ID, &p.EntityID, &p.CampaignID, &p.Name,
		&entryJSON, &entryHTML,
		&p.IsPrivate, &p.SortOrder, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if entryJSON.Valid {
		p.Entry = json.RawMessage(entryJSON.String)
	}
	if entryHTML.Valid {
		p.EntryHTML = &entryHTML.String
	}
	return &p, nil
}

// marshalEntry converts entry JSON to a nullable string for DB storage.
func marshalEntry(entry json.RawMessage) (*string, error) {
	if len(entry) == 0 {
		return nil, nil
	}
	s := string(entry)
	return &s, nil
}
