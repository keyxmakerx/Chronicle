package bestiary

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// BestiaryRepository defines persistence operations for bestiary publications.
type BestiaryRepository interface {
	// Publication CRUD.
	CreatePublication(ctx context.Context, p *Publication) error
	GetByID(ctx context.Context, id string) (*Publication, error)
	GetBySlug(ctx context.Context, slug string) (*Publication, error)
	UpdatePublication(ctx context.Context, p *Publication) error
	ArchivePublication(ctx context.Context, id string) error
	UpdateVisibility(ctx context.Context, id, visibility string) error

	// Listing.
	ListPublished(ctx context.Context, page, perPage int) ([]Publication, int, error)
	ListByCreator(ctx context.Context, creatorID string, includeAll bool, page, perPage int) ([]Publication, int, error)

	// Slug uniqueness.
	SlugExists(ctx context.Context, slug string) (bool, error)
}

// bestiaryRepo is the MariaDB implementation of BestiaryRepository.
type bestiaryRepo struct {
	db *sql.DB
}

// NewBestiaryRepository creates a new MariaDB-backed bestiary repository.
func NewBestiaryRepository(db *sql.DB) BestiaryRepository {
	return &bestiaryRepo{db: db}
}

// pubCols is the column list for publication queries.
const pubCols = `id, creator_id, source_entity_id, source_campaign_id, system_id,
       name, slug, description, flavor_text, artwork_media_id,
       statblock_json, version, tags, organization, role, level,
       downloads, rating_sum, rating_count, favorites,
       visibility, flagged_count, reviewed_by, reviewed_at,
       hub_id, hub_synced_at, created_at, updated_at`

// scanPublication reads a row into a Publication struct.
func scanPublication(scanner interface{ Scan(...any) error }) (*Publication, error) {
	p := &Publication{}
	var statblock, tags []byte
	err := scanner.Scan(
		&p.ID, &p.CreatorID, &p.SourceEntityID, &p.SourceCampaignID, &p.SystemID,
		&p.Name, &p.Slug, &p.Description, &p.FlavorText, &p.ArtworkMediaID,
		&statblock, &p.Version, &tags, &p.Organization, &p.Role, &p.Level,
		&p.Downloads, &p.RatingSum, &p.RatingCount, &p.Favorites,
		&p.Visibility, &p.FlaggedCount, &p.ReviewedBy, &p.ReviewedAt,
		&p.HubID, &p.HubSyncedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.StatblockJSON = json.RawMessage(statblock)
	if tags != nil {
		p.Tags = json.RawMessage(tags)
	}
	return p, nil
}

// CreatePublication inserts a new publication.
func (r *bestiaryRepo) CreatePublication(ctx context.Context, p *Publication) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bestiary_publications
		 (id, creator_id, source_entity_id, source_campaign_id, system_id,
		  name, slug, description, flavor_text, artwork_media_id,
		  statblock_json, version, tags, organization, role, level, visibility)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.CreatorID, p.SourceEntityID, p.SourceCampaignID, p.SystemID,
		p.Name, p.Slug, p.Description, p.FlavorText, p.ArtworkMediaID,
		string(p.StatblockJSON), p.Version, nullableJSON(p.Tags),
		p.Organization, p.Role, p.Level, p.Visibility,
	)
	if err != nil {
		return fmt.Errorf("insert publication: %w", err)
	}
	return nil
}

// GetByID returns a publication by its UUID.
func (r *bestiaryRepo) GetByID(ctx context.Context, id string) (*Publication, error) {
	p, err := scanPublication(r.db.QueryRowContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications WHERE id = ?`, id))
	if err != nil {
		return nil, fmt.Errorf("get publication by id: %w", err)
	}
	if p == nil {
		return nil, apperror.NewNotFound("publication not found")
	}
	return p, nil
}

// GetBySlug returns a published/unlisted publication by its URL slug.
func (r *bestiaryRepo) GetBySlug(ctx context.Context, slug string) (*Publication, error) {
	p, err := scanPublication(r.db.QueryRowContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications WHERE slug = ?`, slug))
	if err != nil {
		return nil, fmt.Errorf("get publication by slug: %w", err)
	}
	if p == nil {
		return nil, apperror.NewNotFound("publication not found")
	}
	return p, nil
}

// UpdatePublication updates the mutable fields of a publication.
func (r *bestiaryRepo) UpdatePublication(ctx context.Context, p *Publication) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET
		 name = ?, slug = ?, description = ?, flavor_text = ?,
		 statblock_json = ?, version = ?, tags = ?,
		 organization = ?, role = ?, level = ?
		 WHERE id = ?`,
		p.Name, p.Slug, p.Description, p.FlavorText,
		string(p.StatblockJSON), p.Version, nullableJSON(p.Tags),
		p.Organization, p.Role, p.Level, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update publication: %w", err)
	}
	return nil
}

// ArchivePublication soft-deletes a publication by setting visibility to archived.
func (r *bestiaryRepo) ArchivePublication(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET visibility = ? WHERE id = ?`,
		VisibilityArchived, id,
	)
	if err != nil {
		return fmt.Errorf("archive publication: %w", err)
	}
	return nil
}

// UpdateVisibility changes the visibility state of a publication.
func (r *bestiaryRepo) UpdateVisibility(ctx context.Context, id, visibility string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET visibility = ? WHERE id = ?`,
		visibility, id,
	)
	if err != nil {
		return fmt.Errorf("update visibility: %w", err)
	}
	return nil
}

// ListPublished returns paginated publications visible to all users.
func (r *bestiaryRepo) ListPublished(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_publications WHERE visibility = ?`,
		VisibilityPublished,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count published: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications
		 WHERE visibility = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		VisibilityPublished, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list published: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// ListByCreator returns paginated publications by a specific creator.
// If includeAll is true, returns all visibility states (for "my creations" view).
// Otherwise, returns only published/unlisted publications.
func (r *bestiaryRepo) ListByCreator(ctx context.Context, creatorID string, includeAll bool, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var countQuery, listQuery string
	var countArgs, listArgs []any

	if includeAll {
		countQuery = `SELECT COUNT(*) FROM bestiary_publications WHERE creator_id = ?`
		countArgs = []any{creatorID}
		listQuery = `SELECT ` + pubCols + ` FROM bestiary_publications
		 WHERE creator_id = ?
		 ORDER BY updated_at DESC
		 LIMIT ? OFFSET ?`
		listArgs = []any{creatorID, perPage, offset}
	} else {
		countQuery = `SELECT COUNT(*) FROM bestiary_publications WHERE creator_id = ? AND visibility IN (?, ?)`
		countArgs = []any{creatorID, VisibilityPublished, VisibilityUnlisted}
		listQuery = `SELECT ` + pubCols + ` FROM bestiary_publications
		 WHERE creator_id = ? AND visibility IN (?, ?)
		 ORDER BY updated_at DESC
		 LIMIT ? OFFSET ?`
		listArgs = []any{creatorID, VisibilityPublished, VisibilityUnlisted, perPage, offset}
	}

	var total int
	if err := r.db.QueryRowContext(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count by creator: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list by creator: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// SlugExists checks whether a slug is already taken.
func (r *bestiaryRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bestiary_publications WHERE slug = ?)`, slug,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check slug exists: %w", err)
	}
	return exists, nil
}

// --- Helpers ---

// scanPublications reads multiple rows into a slice of Publications.
func scanPublications(rows *sql.Rows) ([]Publication, error) {
	var pubs []Publication
	for rows.Next() {
		p, err := scanPublication(rows)
		if err != nil {
			return nil, fmt.Errorf("scan publication row: %w", err)
		}
		pubs = append(pubs, *p)
	}
	return pubs, rows.Err()
}

// nullableJSON returns nil for empty/null JSON, or the raw string for storage.
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
