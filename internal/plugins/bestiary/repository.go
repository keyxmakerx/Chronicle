package bestiary

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

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

	// Search & feeds.
	SearchPublications(ctx context.Context, filters SearchFilters) ([]Publication, int, error)
	ListNewest(ctx context.Context, page, perPage int) ([]Publication, int, error)
	ListTopRated(ctx context.Context, page, perPage int) ([]Publication, int, error)
	ListMostImported(ctx context.Context, page, perPage int) ([]Publication, int, error)

	// Creator stats.
	GetCreatorStats(ctx context.Context, creatorID string) (*CreatorStats, error)

	// Ratings.
	CreateRating(ctx context.Context, r *Rating) error
	UpdateRating(ctx context.Context, r *Rating) error
	DeleteRating(ctx context.Context, userID, publicationID string) error
	GetRating(ctx context.Context, userID, publicationID string) (*Rating, error)
	ListReviews(ctx context.Context, publicationID string, page, perPage int) ([]Rating, int, error)
	// AdjustRatingAggregates atomically updates rating_sum and rating_count on a publication.
	AdjustRatingAggregates(ctx context.Context, publicationID string, sumDelta, countDelta int) error

	// Favorites.
	AddFavorite(ctx context.Context, userID, publicationID string) error
	RemoveFavorite(ctx context.Context, userID, publicationID string) error
	IsFavorited(ctx context.Context, userID, publicationID string) (bool, error)
	ListFavorites(ctx context.Context, userID string, page, perPage int) ([]Publication, int, error)
	// AdjustFavoriteCount atomically updates the favorites counter on a publication.
	AdjustFavoriteCount(ctx context.Context, publicationID string, delta int) error

	// Imports.
	CreateImport(ctx context.Context, imp *Import) error
	ImportExists(ctx context.Context, publicationID, campaignID string) (bool, error)
	IncrementDownloads(ctx context.Context, publicationID string) error

	// Transactional rating operations (row-locked to prevent race conditions).
	RatePublication(ctx context.Context, publicationID, userID string, rating int, reviewText *string) (isNew bool, err error)
	RemoveRatingWithAggregates(ctx context.Context, publicationID, userID string) error

	// Transactional favorite operations (row-locked to prevent race conditions).
	ToggleFavoriteWithCount(ctx context.Context, publicationID, userID string) (favorited bool, err error)
	RemoveFavoriteWithCount(ctx context.Context, publicationID, userID string) error

	// Flagging (transactional with per-user deduplication).
	FlagPublicationAtomic(ctx context.Context, userID, publicationID string, reason *string, threshold int) error
	FlagExists(ctx context.Context, userID, publicationID string) (bool, error)
	IncrementFlaggedCount(ctx context.Context, publicationID string) (newCount int, err error)
	AutoFlagIfThreshold(ctx context.Context, publicationID string, threshold int) error

	// Moderation.
	ListFlagged(ctx context.Context, page, perPage int) ([]Publication, int, error)
	CreateModerationEntry(ctx context.Context, entry *ModerationLogEntry) error
	GetModerationLog(ctx context.Context, publicationID string) ([]ModerationLogEntry, error)
	GetBestiaryStats(ctx context.Context) (*BestiaryStats, error)
	SetReviewedBy(ctx context.Context, publicationID, moderatorID string) error

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

// SearchPublications performs a filtered, paginated search across published publications.
// Builds a dynamic WHERE clause based on the provided filters.
func (r *bestiaryRepo) SearchPublications(ctx context.Context, filters SearchFilters) ([]Publication, int, error) {
	offset := (filters.Page - 1) * filters.PerPage

	var where []string
	var args []any

	where = append(where, "visibility = ?")
	args = append(args, VisibilityPublished)

	// Full-text search on name + description.
	if filters.Query != "" {
		sanitized := sanitizeFTQuery(filters.Query)
		if sanitized != "" {
			where = append(where, "MATCH(name, description) AGAINST(? IN BOOLEAN MODE)")
			args = append(args, sanitized)
		}
	}

	if filters.LevelMin != nil {
		where = append(where, "level >= ?")
		args = append(args, *filters.LevelMin)
	}
	if filters.LevelMax != nil {
		where = append(where, "level <= ?")
		args = append(args, *filters.LevelMax)
	}
	if filters.Organization != "" {
		where = append(where, "organization = ?")
		args = append(args, filters.Organization)
	}
	if filters.Role != "" {
		where = append(where, "role = ?")
		args = append(args, filters.Role)
	}
	if filters.CreatorID != "" {
		where = append(where, "creator_id = ?")
		args = append(args, filters.CreatorID)
	}
	if filters.SystemID != "" {
		where = append(where, "system_id = ?")
		args = append(args, filters.SystemID)
	}
	if filters.Tags != "" {
		// Filter by individual tags using JSON_CONTAINS.
		for _, tag := range strings.Split(filters.Tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				where = append(where, "JSON_CONTAINS(tags, ?)")
				args = append(args, fmt.Sprintf("%q", tag))
			}
		}
	}

	whereClause := strings.Join(where, " AND ")

	// Count total matches.
	var total int
	countQ := "SELECT COUNT(*) FROM bestiary_publications WHERE " + whereClause
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("search count: %w", err)
	}

	// Determine sort order.
	orderBy := "created_at DESC" // default
	switch filters.Sort {
	case "newest":
		orderBy = "created_at DESC"
	case "top_rated":
		orderBy = "CASE WHEN rating_count >= 3 THEN rating_sum / rating_count ELSE 0 END DESC, rating_count DESC"
	case "most_imported":
		orderBy = "downloads DESC"
	case "trending":
		// Time-decayed score: higher weight for recent activity.
		orderBy = "downloads DESC, rating_count DESC, created_at DESC"
	}

	listQ := "SELECT " + pubCols + " FROM bestiary_publications WHERE " + whereClause +
		" ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	listArgs := append(args, filters.PerPage, offset)

	rows, err := r.db.QueryContext(ctx, listQ, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("search list: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// ListNewest returns the most recently published publications.
func (r *bestiaryRepo) ListNewest(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	return r.listWithOrder(ctx, "created_at DESC", page, perPage)
}

// ListTopRated returns publications with the highest average rating (min 3 ratings).
func (r *bestiaryRepo) ListTopRated(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_publications WHERE visibility = ? AND rating_count >= 3`,
		VisibilityPublished,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count top rated: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications
		 WHERE visibility = ? AND rating_count >= 3
		 ORDER BY (rating_sum / rating_count) DESC, rating_count DESC
		 LIMIT ? OFFSET ?`,
		VisibilityPublished, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list top rated: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// ListMostImported returns publications with the highest download count.
func (r *bestiaryRepo) ListMostImported(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	return r.listWithOrder(ctx, "downloads DESC, created_at DESC", page, perPage)
}

// GetCreatorStats returns aggregated publication statistics for a creator.
func (r *bestiaryRepo) GetCreatorStats(ctx context.Context, creatorID string) (*CreatorStats, error) {
	stats := &CreatorStats{}
	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COUNT(*),
		   COALESCE(SUM(downloads), 0),
		   COALESCE(SUM(rating_count), 0),
		   CASE WHEN SUM(rating_count) > 0
		        THEN SUM(rating_sum) / SUM(rating_count)
		        ELSE 0
		   END
		 FROM bestiary_publications
		 WHERE creator_id = ? AND visibility IN (?, ?)`,
		creatorID, VisibilityPublished, VisibilityUnlisted,
	).Scan(&stats.PublicationCount, &stats.TotalDownloads, &stats.TotalRatings, &stats.AverageRating)
	if err != nil {
		return nil, fmt.Errorf("get creator stats: %w", err)
	}
	return stats, nil
}

// --- Rating repository methods ---

// CreateRating inserts a new rating.
func (r *bestiaryRepo) CreateRating(ctx context.Context, rt *Rating) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bestiary_ratings (id, publication_id, user_id, rating, review_text)
		 VALUES (?, ?, ?, ?, ?)`,
		rt.ID, rt.PublicationID, rt.UserID, rt.Rating, rt.ReviewText,
	)
	if err != nil {
		return fmt.Errorf("insert rating: %w", err)
	}
	return nil
}

// UpdateRating updates an existing rating's score and review text.
func (r *bestiaryRepo) UpdateRating(ctx context.Context, rt *Rating) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_ratings SET rating = ?, review_text = ? WHERE id = ?`,
		rt.Rating, rt.ReviewText, rt.ID,
	)
	if err != nil {
		return fmt.Errorf("update rating: %w", err)
	}
	return nil
}

// DeleteRating removes a user's rating on a publication.
func (r *bestiaryRepo) DeleteRating(ctx context.Context, userID, publicationID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM bestiary_ratings WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	)
	if err != nil {
		return fmt.Errorf("delete rating: %w", err)
	}
	return nil
}

// GetRating returns a user's rating on a publication, or nil if not rated.
func (r *bestiaryRepo) GetRating(ctx context.Context, userID, publicationID string) (*Rating, error) {
	rt := &Rating{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, publication_id, user_id, rating, review_text, created_at, updated_at
		 FROM bestiary_ratings WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	).Scan(&rt.ID, &rt.PublicationID, &rt.UserID, &rt.Rating, &rt.ReviewText, &rt.CreatedAt, &rt.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rating: %w", err)
	}
	return rt, nil
}

// ListReviews returns paginated ratings with review text for a publication.
func (r *bestiaryRepo) ListReviews(ctx context.Context, publicationID string, page, perPage int) ([]Rating, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_ratings WHERE publication_id = ? AND review_text IS NOT NULL AND review_text != ''`,
		publicationID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count reviews: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, publication_id, user_id, rating, review_text, created_at, updated_at
		 FROM bestiary_ratings
		 WHERE publication_id = ? AND review_text IS NOT NULL AND review_text != ''
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		publicationID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list reviews: %w", err)
	}
	defer rows.Close()

	var ratings []Rating
	for rows.Next() {
		var rt Rating
		if err := rows.Scan(&rt.ID, &rt.PublicationID, &rt.UserID, &rt.Rating, &rt.ReviewText, &rt.CreatedAt, &rt.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan review: %w", err)
		}
		ratings = append(ratings, rt)
	}
	return ratings, total, rows.Err()
}

// AdjustRatingAggregates atomically updates rating_sum and rating_count.
func (r *bestiaryRepo) AdjustRatingAggregates(ctx context.Context, publicationID string, sumDelta, countDelta int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications
		 SET rating_sum = rating_sum + ?, rating_count = rating_count + ?
		 WHERE id = ?`,
		sumDelta, countDelta, publicationID,
	)
	if err != nil {
		return fmt.Errorf("adjust rating aggregates: %w", err)
	}
	return nil
}

// --- Favorite repository methods ---

// AddFavorite adds a favorite bookmark.
func (r *bestiaryRepo) AddFavorite(ctx context.Context, userID, publicationID string) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bestiary_favorites (user_id, publication_id) VALUES (?, ?)`,
		userID, publicationID,
	)
	if err != nil {
		return fmt.Errorf("add favorite: %w", err)
	}
	return nil
}

// RemoveFavorite removes a favorite bookmark.
func (r *bestiaryRepo) RemoveFavorite(ctx context.Context, userID, publicationID string) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM bestiary_favorites WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	)
	if err != nil {
		return fmt.Errorf("remove favorite: %w", err)
	}
	return nil
}

// IsFavorited checks if a user has favorited a publication.
func (r *bestiaryRepo) IsFavorited(ctx context.Context, userID, publicationID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bestiary_favorites WHERE user_id = ? AND publication_id = ?)`,
		userID, publicationID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check favorited: %w", err)
	}
	return exists, nil
}

// ListFavorites returns paginated publications favorited by a user.
func (r *bestiaryRepo) ListFavorites(ctx context.Context, userID string, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_favorites WHERE user_id = ?`, userID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count favorites: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications p
		 INNER JOIN bestiary_favorites f ON f.publication_id = p.id
		 WHERE f.user_id = ?
		 ORDER BY f.created_at DESC
		 LIMIT ? OFFSET ?`,
		userID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// AdjustFavoriteCount atomically updates the favorites counter.
func (r *bestiaryRepo) AdjustFavoriteCount(ctx context.Context, publicationID string, delta int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET favorites = favorites + ? WHERE id = ?`,
		delta, publicationID,
	)
	if err != nil {
		return fmt.Errorf("adjust favorite count: %w", err)
	}
	return nil
}

// --- Import repository methods ---

// CreateImport records an import of a publication into a campaign.
func (r *bestiaryRepo) CreateImport(ctx context.Context, imp *Import) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bestiary_imports (id, publication_id, user_id, campaign_id, entity_id)
		 VALUES (?, ?, ?, ?, ?)`,
		imp.ID, imp.PublicationID, imp.UserID, imp.CampaignID, imp.EntityID,
	)
	if err != nil {
		return fmt.Errorf("insert import: %w", err)
	}
	return nil
}

// ImportExists checks if a publication has already been imported into a campaign.
func (r *bestiaryRepo) ImportExists(ctx context.Context, publicationID, campaignID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bestiary_imports WHERE publication_id = ? AND campaign_id = ?)`,
		publicationID, campaignID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check import exists: %w", err)
	}
	return exists, nil
}

// IncrementDownloads bumps the download counter by 1.
func (r *bestiaryRepo) IncrementDownloads(ctx context.Context, publicationID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET downloads = downloads + 1 WHERE id = ?`,
		publicationID,
	)
	if err != nil {
		return fmt.Errorf("increment downloads: %w", err)
	}
	return nil
}

// --- Flagging repository methods ---

// IncrementFlaggedCount bumps the flagged_count and returns the new value.
func (r *bestiaryRepo) IncrementFlaggedCount(ctx context.Context, publicationID string) (int, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET flagged_count = flagged_count + 1 WHERE id = ?`,
		publicationID,
	)
	if err != nil {
		return 0, fmt.Errorf("increment flagged count: %w", err)
	}

	var count int
	err = r.db.QueryRowContext(ctx,
		`SELECT flagged_count FROM bestiary_publications WHERE id = ?`,
		publicationID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("read flagged count: %w", err)
	}
	return count, nil
}

// AutoFlagIfThreshold sets visibility to flagged if flagged_count >= threshold.
func (r *bestiaryRepo) AutoFlagIfThreshold(ctx context.Context, publicationID string, threshold int) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET visibility = ?
		 WHERE id = ? AND flagged_count >= ? AND visibility = ?`,
		VisibilityFlagged, publicationID, threshold, VisibilityPublished,
	)
	if err != nil {
		return fmt.Errorf("auto-flag: %w", err)
	}
	return nil
}

// --- Moderation repository methods ---

// ListFlagged returns paginated publications in the flagged state for admin review.
func (r *bestiaryRepo) ListFlagged(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_publications WHERE visibility = ?`,
		VisibilityFlagged,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count flagged: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications
		 WHERE visibility = ?
		 ORDER BY flagged_count DESC, updated_at DESC
		 LIMIT ? OFFSET ?`,
		VisibilityFlagged, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list flagged: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
}

// CreateModerationEntry inserts a moderation log entry.
func (r *bestiaryRepo) CreateModerationEntry(ctx context.Context, entry *ModerationLogEntry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO bestiary_moderation_log (id, publication_id, moderator_id, action, reason)
		 VALUES (?, ?, ?, ?, ?)`,
		entry.ID, entry.PublicationID, entry.ModeratorID, entry.Action, entry.Reason,
	)
	if err != nil {
		return fmt.Errorf("insert moderation entry: %w", err)
	}
	return nil
}

// GetModerationLog returns all moderation entries for a publication, newest first.
func (r *bestiaryRepo) GetModerationLog(ctx context.Context, publicationID string) ([]ModerationLogEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, publication_id, moderator_id, action, reason, created_at
		 FROM bestiary_moderation_log
		 WHERE publication_id = ?
		 ORDER BY created_at DESC`,
		publicationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get moderation log: %w", err)
	}
	defer rows.Close()

	var entries []ModerationLogEntry
	for rows.Next() {
		var e ModerationLogEntry
		if err := rows.Scan(&e.ID, &e.PublicationID, &e.ModeratorID, &e.Action, &e.Reason, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan moderation entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetBestiaryStats returns aggregate statistics for the admin dashboard.
func (r *bestiaryRepo) GetBestiaryStats(ctx context.Context) (*BestiaryStats, error) {
	stats := &BestiaryStats{}

	err := r.db.QueryRowContext(ctx,
		`SELECT
		   COUNT(*),
		   SUM(CASE WHEN visibility = 'published' THEN 1 ELSE 0 END),
		   SUM(CASE WHEN visibility = 'flagged' THEN 1 ELSE 0 END),
		   COALESCE(SUM(rating_count), 0),
		   COALESCE(SUM(downloads), 0),
		   COUNT(DISTINCT creator_id)
		 FROM bestiary_publications`,
	).Scan(
		&stats.TotalPublications, &stats.PublishedCount, &stats.FlaggedCount,
		&stats.TotalRatings, &stats.TotalImports, &stats.TotalCreators,
	)
	if err != nil {
		return nil, fmt.Errorf("get bestiary stats: %w", err)
	}
	return stats, nil
}

// SetReviewedBy stamps the publication with the reviewing moderator and timestamp.
func (r *bestiaryRepo) SetReviewedBy(ctx context.Context, publicationID, moderatorID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE bestiary_publications SET reviewed_by = ?, reviewed_at = NOW() WHERE id = ?`,
		moderatorID, publicationID,
	)
	if err != nil {
		return fmt.Errorf("set reviewed by: %w", err)
	}
	return nil
}

// --- Transactional rating methods ---

// RatePublication creates or updates a rating in a single transaction with row
// locking to prevent race conditions. Adjusts publication aggregates atomically.
func (r *bestiaryRepo) RatePublication(ctx context.Context, publicationID, userID string, rating int, reviewText *string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin rate tx: %w", err)
	}
	defer tx.Rollback()

	// Lock the existing rating row (if any) to prevent concurrent modification.
	var existingID string
	var existingRating int
	err = tx.QueryRowContext(ctx,
		`SELECT id, rating FROM bestiary_ratings
		 WHERE user_id = ? AND publication_id = ? FOR UPDATE`,
		userID, publicationID,
	).Scan(&existingID, &existingRating)

	isNew := err == sql.ErrNoRows
	if err != nil && !isNew {
		return false, fmt.Errorf("lock rating row: %w", err)
	}

	if isNew {
		newID := generateID()
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bestiary_ratings (id, publication_id, user_id, rating, review_text)
			 VALUES (?, ?, ?, ?, ?)`,
			newID, publicationID, userID, rating, reviewText,
		); err != nil {
			return false, fmt.Errorf("insert rating: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE bestiary_publications
			 SET rating_sum = rating_sum + ?, rating_count = rating_count + 1
			 WHERE id = ?`,
			rating, publicationID,
		); err != nil {
			return false, fmt.Errorf("adjust aggregates (new): %w", err)
		}
	} else {
		sumDelta := rating - existingRating
		if _, err := tx.ExecContext(ctx,
			`UPDATE bestiary_ratings SET rating = ?, review_text = ? WHERE id = ?`,
			rating, reviewText, existingID,
		); err != nil {
			return false, fmt.Errorf("update rating: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE bestiary_publications SET rating_sum = rating_sum + ? WHERE id = ?`,
			sumDelta, publicationID,
		); err != nil {
			return false, fmt.Errorf("adjust aggregates (update): %w", err)
		}
	}

	return isNew, tx.Commit()
}

// RemoveRatingWithAggregates deletes a rating and adjusts aggregates in a single
// transaction. Returns NotFound if the user hasn't rated this publication.
func (r *bestiaryRepo) RemoveRatingWithAggregates(ctx context.Context, publicationID, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove-rating tx: %w", err)
	}
	defer tx.Rollback()

	var ratingValue int
	err = tx.QueryRowContext(ctx,
		`SELECT rating FROM bestiary_ratings
		 WHERE user_id = ? AND publication_id = ? FOR UPDATE`,
		userID, publicationID,
	).Scan(&ratingValue)
	if err == sql.ErrNoRows {
		return apperror.NewNotFound("rating not found")
	}
	if err != nil {
		return fmt.Errorf("lock rating for removal: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bestiary_ratings WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	); err != nil {
		return fmt.Errorf("delete rating: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE bestiary_publications
		 SET rating_sum = rating_sum - ?, rating_count = rating_count - 1
		 WHERE id = ?`,
		ratingValue, publicationID,
	); err != nil {
		return fmt.Errorf("adjust aggregates (remove): %w", err)
	}

	return tx.Commit()
}

// --- Transactional favorite methods ---

// ToggleFavoriteWithCount adds or removes a favorite and adjusts the publication
// counter in a single transaction. Returns the new favorited state.
func (r *bestiaryRepo) ToggleFavoriteWithCount(ctx context.Context, publicationID, userID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin toggle-fav tx: %w", err)
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM bestiary_favorites
		 WHERE user_id = ? AND publication_id = ? FOR UPDATE`,
		userID, publicationID,
	).Scan(&exists)

	if err == sql.ErrNoRows {
		// Not favorited — add it.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO bestiary_favorites (user_id, publication_id) VALUES (?, ?)`,
			userID, publicationID,
		); err != nil {
			return false, fmt.Errorf("insert favorite: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE bestiary_publications SET favorites = favorites + 1 WHERE id = ?`,
			publicationID,
		); err != nil {
			return false, fmt.Errorf("increment favorites: %w", err)
		}
		return true, tx.Commit()
	}
	if err != nil {
		return false, fmt.Errorf("lock favorite row: %w", err)
	}

	// Already favorited — remove it.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bestiary_favorites WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	); err != nil {
		return false, fmt.Errorf("delete favorite: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bestiary_publications SET favorites = favorites - 1 WHERE id = ?`,
		publicationID,
	); err != nil {
		return false, fmt.Errorf("decrement favorites: %w", err)
	}
	return false, tx.Commit()
}

// RemoveFavoriteWithCount removes a favorite and decrements the counter in a
// single transaction. Idempotent — returns nil if not favorited.
func (r *bestiaryRepo) RemoveFavoriteWithCount(ctx context.Context, publicationID, userID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin remove-fav tx: %w", err)
	}
	defer tx.Rollback()

	var exists bool
	err = tx.QueryRowContext(ctx,
		`SELECT 1 FROM bestiary_favorites
		 WHERE user_id = ? AND publication_id = ? FOR UPDATE`,
		userID, publicationID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil // Idempotent — already not favorited.
	}
	if err != nil {
		return fmt.Errorf("lock favorite for removal: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM bestiary_favorites WHERE user_id = ? AND publication_id = ?`,
		userID, publicationID,
	); err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE bestiary_publications SET favorites = favorites - 1 WHERE id = ?`,
		publicationID,
	); err != nil {
		return fmt.Errorf("decrement favorites: %w", err)
	}

	return tx.Commit()
}

// --- Transactional flag methods ---

// FlagPublicationAtomic records a user flag, increments the flagged_count, and
// auto-hides the publication if the threshold is reached — all in one transaction.
// Returns Conflict if the user has already flagged this publication.
func (r *bestiaryRepo) FlagPublicationAtomic(ctx context.Context, userID, publicationID string, reason *string, threshold int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin flag tx: %w", err)
	}
	defer tx.Rollback()

	// Insert flag — composite PK prevents duplicates.
	_, err = tx.ExecContext(ctx,
		`INSERT INTO bestiary_flags (user_id, publication_id, reason) VALUES (?, ?, ?)`,
		userID, publicationID, reason,
	)
	if err != nil {
		// Duplicate key error means the user already flagged this publication.
		if strings.Contains(err.Error(), "Duplicate") || strings.Contains(err.Error(), "duplicate") {
			return apperror.NewConflict("you have already flagged this publication")
		}
		return fmt.Errorf("insert flag: %w", err)
	}

	// Atomically increment flagged_count and retrieve the new value.
	if _, err := tx.ExecContext(ctx,
		`UPDATE bestiary_publications
		 SET flagged_count = LAST_INSERT_ID(flagged_count + 1)
		 WHERE id = ?`,
		publicationID,
	); err != nil {
		return fmt.Errorf("increment flagged count: %w", err)
	}

	var newCount int
	if err := tx.QueryRowContext(ctx, `SELECT LAST_INSERT_ID()`).Scan(&newCount); err != nil {
		return fmt.Errorf("read flagged count: %w", err)
	}

	// Auto-hide if threshold reached.
	if newCount >= threshold {
		if _, err := tx.ExecContext(ctx,
			`UPDATE bestiary_publications SET visibility = ?
			 WHERE id = ? AND visibility = ?`,
			VisibilityFlagged, publicationID, VisibilityPublished,
		); err != nil {
			return fmt.Errorf("auto-flag publication: %w", err)
		}
	}

	return tx.Commit()
}

// FlagExists checks if a user has already flagged a publication.
func (r *bestiaryRepo) FlagExists(ctx context.Context, userID, publicationID string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM bestiary_flags WHERE user_id = ? AND publication_id = ?)`,
		userID, publicationID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check flag exists: %w", err)
	}
	return exists, nil
}

// listWithOrder is a helper that lists published publications with a custom ORDER BY.
func (r *bestiaryRepo) listWithOrder(ctx context.Context, orderBy string, page, perPage int) ([]Publication, int, error) {
	offset := (page - 1) * perPage

	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM bestiary_publications WHERE visibility = ?`,
		VisibilityPublished,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count for list: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+pubCols+` FROM bestiary_publications
		 WHERE visibility = ?
		 ORDER BY `+orderBy+`
		 LIMIT ? OFFSET ?`,
		VisibilityPublished, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list with order: %w", err)
	}
	defer rows.Close()

	pubs, err := scanPublications(rows)
	if err != nil {
		return nil, 0, err
	}
	return pubs, total, nil
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

// sanitizeFTQuery strips MySQL full-text search operators to prevent injection.
func sanitizeFTQuery(q string) string {
	replacer := strings.NewReplacer(
		"+", "", "-", "", "*", "", "~", "",
		"<", "", ">", "", "(", "", ")", "",
		"@", "", "\"", "",
	)
	return strings.TrimSpace(replacer.Replace(q))
}

// nullableJSON returns nil for empty/null JSON, or the raw string for storage.
func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
