package bestiary

import (
	"context"
	"encoding/json"
	"time"
)

// --- Visibility constants ---

const (
	// VisibilityDraft means only the creator can see the publication.
	VisibilityDraft = "draft"
	// VisibilityPublished means the publication is visible in search/browse.
	VisibilityPublished = "published"
	// VisibilityUnlisted means accessible by direct link only, not in search.
	VisibilityUnlisted = "unlisted"
	// VisibilityArchived means removed from search by creator or admin.
	VisibilityArchived = "archived"
	// VisibilityFlagged means auto-hidden after 3+ user flags, pending moderation.
	VisibilityFlagged = "flagged"
)

// ValidVisibilities is the set of allowed visibility values.
var ValidVisibilities = map[string]bool{
	VisibilityDraft:     true,
	VisibilityPublished: true,
	VisibilityUnlisted:  true,
	VisibilityArchived:  true,
	VisibilityFlagged:   true,
}

// --- Domain models ---

// Publication is a shared creature statblock published to the instance bestiary.
// Publications are instance-scoped (not campaign-scoped) — any authenticated
// user can browse them.
type Publication struct {
	ID               string          `json:"id"`
	CreatorID        string          `json:"creator_id"`
	SourceEntityID   *string         `json:"source_entity_id,omitempty"`
	SourceCampaignID *string         `json:"source_campaign_id,omitempty"`
	SystemID         string          `json:"system_id"`
	Name             string          `json:"name"`
	Slug             string          `json:"slug"`
	Description      *string         `json:"description,omitempty"`
	FlavorText       *string         `json:"flavor_text,omitempty"`
	ArtworkMediaID   *string         `json:"artwork_media_id,omitempty"`
	StatblockJSON    json.RawMessage `json:"statblock_json"`
	Version          int             `json:"version"`
	Tags             json.RawMessage `json:"tags,omitempty"`
	Organization     *string         `json:"organization,omitempty"`
	Role             *string         `json:"role,omitempty"`
	Level            *int            `json:"level,omitempty"`
	Downloads        int             `json:"downloads"`
	RatingSum        int             `json:"rating_sum"`
	RatingCount      int             `json:"rating_count"`
	Favorites        int             `json:"favorites"`
	Visibility       string          `json:"visibility"`
	FlaggedCount     int             `json:"flagged_count"`
	ReviewedBy       *string         `json:"reviewed_by,omitempty"`
	ReviewedAt       *time.Time      `json:"reviewed_at,omitempty"`
	HubID            *string         `json:"hub_id,omitempty"`
	HubSyncedAt      *time.Time      `json:"hub_synced_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// RatingAverage returns the average rating, or 0 if unrated.
func (p *Publication) RatingAverage() float64 {
	if p.RatingCount == 0 {
		return 0
	}
	return float64(p.RatingSum) / float64(p.RatingCount)
}

// Rating is a user's 1-5 star rating with optional text review.
type Rating struct {
	ID            string    `json:"id"`
	PublicationID string    `json:"publication_id"`
	UserID        string    `json:"user_id"`
	Rating        int       `json:"rating"`
	ReviewText    *string   `json:"review_text,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Favorite is a user's bookmark on a publication.
type Favorite struct {
	UserID        string    `json:"user_id"`
	PublicationID string    `json:"publication_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// Import tracks a publication imported into a campaign as an entity.
type Import struct {
	ID            string    `json:"id"`
	PublicationID string    `json:"publication_id"`
	UserID        string    `json:"user_id"`
	CampaignID    string    `json:"campaign_id"`
	EntityID      *string   `json:"entity_id,omitempty"`
	ImportedAt    time.Time `json:"imported_at"`
}

// ModerationLogEntry records an admin moderation action on a publication.
type ModerationLogEntry struct {
	ID            string    `json:"id"`
	PublicationID string    `json:"publication_id"`
	ModeratorID   string    `json:"moderator_id"`
	Action        string    `json:"action"`
	Reason        *string   `json:"reason,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// --- Request / Input DTOs ---

// CreatePublicationInput is the validated input for publishing a creature.
type CreatePublicationInput struct {
	SourceEntityID   *string         `json:"source_entity_id,omitempty"`
	SourceCampaignID *string         `json:"source_campaign_id,omitempty"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitempty"`
	FlavorText       *string         `json:"flavor_text,omitempty"`
	Tags             json.RawMessage `json:"tags,omitempty"`
	StatblockJSON    json.RawMessage `json:"statblock_json"`
	Visibility       string          `json:"visibility"`
}

// UpdatePublicationInput is the validated input for updating a publication.
type UpdatePublicationInput struct {
	Name          *string         `json:"name,omitempty"`
	Description   *string         `json:"description,omitempty"`
	FlavorText    *string         `json:"flavor_text,omitempty"`
	Tags          json.RawMessage `json:"tags,omitempty"`
	StatblockJSON json.RawMessage `json:"statblock_json,omitempty"`
}

// ChangeVisibilityInput is the validated input for changing publication visibility.
type ChangeVisibilityInput struct {
	Visibility string `json:"visibility"`
}

// --- Response DTOs ---

// PublicationSummary is the lightweight representation used in list/search views.
// It omits the full statblock_json to reduce payload size.
type PublicationSummary struct {
	ID            string          `json:"id"`
	CreatorID     string          `json:"creator_id"`
	SystemID      string          `json:"system_id"`
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Description   *string         `json:"description,omitempty"`
	Tags          json.RawMessage `json:"tags,omitempty"`
	Organization  *string         `json:"organization,omitempty"`
	Role          *string         `json:"role,omitempty"`
	Level         *int            `json:"level,omitempty"`
	Downloads     int             `json:"downloads"`
	RatingAverage float64         `json:"rating_average"`
	RatingCount   int             `json:"rating_count"`
	Favorites     int             `json:"favorites"`
	Version       int             `json:"version"`
	Visibility    string          `json:"visibility"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// PublicationListResult is a paginated list of publication summaries.
type PublicationListResult struct {
	Results    []PublicationSummary `json:"results"`
	Total      int                 `json:"total"`
	Page       int                 `json:"page"`
	PerPage    int                 `json:"per_page"`
	TotalPages int                 `json:"total_pages"`
}

// ReviewListResult is a paginated list of ratings with review text.
type ReviewListResult struct {
	Reviews    []Rating `json:"reviews"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	PerPage    int      `json:"per_page"`
	TotalPages int      `json:"total_pages"`
}

// SearchFilters holds the query parameters for bestiary search.
type SearchFilters struct {
	Query        string `json:"q,omitempty"`
	LevelMin     *int   `json:"level_min,omitempty"`
	LevelMax     *int   `json:"level_max,omitempty"`
	Organization string `json:"organization,omitempty"`
	Role         string `json:"role,omitempty"`
	Tags         string `json:"tags,omitempty"`
	CreatorID    string `json:"creator_id,omitempty"`
	SystemID     string `json:"system_id,omitempty"`
	Sort         string `json:"sort,omitempty"`
	Page         int    `json:"page"`
	PerPage      int    `json:"per_page"`
}

// CreatorStats holds aggregated statistics for a bestiary creator.
type CreatorStats struct {
	PublicationCount int     `json:"publication_count"`
	TotalDownloads   int     `json:"total_downloads"`
	TotalRatings     int     `json:"total_ratings"`
	AverageRating    float64 `json:"average_rating"`
}

// CreatorProfile is the public profile of a bestiary creator, combining
// user info (fetched via UserFetcher) with aggregated publication stats.
type CreatorProfile struct {
	UserID      string       `json:"user_id"`
	DisplayName string       `json:"display_name"`
	AvatarURL   string       `json:"avatar_url,omitempty"`
	Stats       CreatorStats `json:"stats"`
}

// UserInfo is the minimal user data needed for creator profiles.
// Populated by the UserFetcher cross-plugin interface.
type UserInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// UserFetcher retrieves public user information for creator profiles.
// Implemented via an adapter wrapping the auth service in routes.go.
type UserFetcher interface {
	GetUserPublicInfo(ctx context.Context, userID string) (*UserInfo, error)
}

// BestiaryStats holds aggregate statistics for the admin dashboard.
type BestiaryStats struct {
	TotalPublications int `json:"total_publications"`
	PublishedCount    int `json:"published_count"`
	FlaggedCount      int `json:"flagged_count"`
	TotalRatings      int `json:"total_ratings"`
	TotalImports      int `json:"total_imports"`
	TotalCreators     int `json:"total_creators"`
}

// ModerateInput is the request body for admin moderation actions.
type ModerateInput struct {
	Action string  `json:"action"` // approve, archive, restore
	Reason *string `json:"reason,omitempty"`
}

// EntityCreator creates campaign entities from imported bestiary statblocks.
// Implemented via an adapter wrapping the entities service in routes.go.
type EntityCreator interface {
	CreateFromStatblock(ctx context.Context, campaignID, userID, name string, statblock json.RawMessage) (entityID string, err error)
}

// CampaignRoleChecker verifies a user's role in a campaign.
// Implemented via an adapter wrapping the campaigns service in routes.go.
type CampaignRoleChecker interface {
	HasMinRole(ctx context.Context, campaignID, userID string, minRole int) (bool, error)
}

// ImportResult is the response returned after importing a publication into a campaign.
type ImportResult struct {
	EntityID      string `json:"entity_id"`
	CampaignID    string `json:"campaign_id"`
	PublicationID string `json:"publication_id"`
	CreatureName  string `json:"creature_name"`
}

// FlagInput is the request body for flagging a publication.
type FlagInput struct {
	Reason *string `json:"reason,omitempty"`
}

// SummaryFromPublication converts a full Publication to a summary for list views.
func SummaryFromPublication(p *Publication) PublicationSummary {
	return PublicationSummary{
		ID:            p.ID,
		CreatorID:     p.CreatorID,
		SystemID:      p.SystemID,
		Name:          p.Name,
		Slug:          p.Slug,
		Description:   p.Description,
		Tags:          p.Tags,
		Organization:  p.Organization,
		Role:          p.Role,
		Level:         p.Level,
		Downloads:     p.Downloads,
		RatingAverage: p.RatingAverage(),
		RatingCount:   p.RatingCount,
		Favorites:     p.Favorites,
		Version:       p.Version,
		Visibility:    p.Visibility,
		CreatedAt:     p.CreatedAt,
		UpdatedAt:     p.UpdatedAt,
	}
}
