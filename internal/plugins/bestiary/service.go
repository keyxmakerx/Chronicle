package bestiary

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// maxStatblockSize is the maximum allowed size for statblock JSON (100KB).
const maxStatblockSize = 100 * 1024

// defaultPerPage is the default number of results per page.
const defaultPerPage = 20

// maxPerPage is the maximum allowed results per page.
const maxPerPage = 50

// slugPattern matches only lowercase alphanumeric characters and hyphens.
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// BestiaryService defines business logic for the community bestiary.
type BestiaryService interface {
	// Publish creates a new publication from validated input.
	Publish(ctx context.Context, creatorID string, input CreatePublicationInput) (*Publication, error)
	// GetBySlug returns a publication by its URL slug.
	GetBySlug(ctx context.Context, slug string) (*Publication, error)
	// GetByID returns a publication by its UUID.
	GetByID(ctx context.Context, id string) (*Publication, error)
	// Update modifies an existing publication (creator only).
	Update(ctx context.Context, userID, publicationID string, input UpdatePublicationInput) (*Publication, error)
	// Archive soft-deletes a publication (creator only).
	Archive(ctx context.Context, userID, publicationID string) error
	// ChangeVisibility changes the visibility state (creator only).
	ChangeVisibility(ctx context.Context, userID, publicationID string, visibility string) error
	// ListPublished returns paginated published publications.
	ListPublished(ctx context.Context, page, perPage int) (*PublicationListResult, error)
	// ListMyCreations returns paginated publications by the requesting user.
	ListMyCreations(ctx context.Context, userID string, page, perPage int) (*PublicationListResult, error)

	// Search performs filtered, paginated search across published publications.
	Search(ctx context.Context, filters SearchFilters) (*PublicationListResult, error)
	// ListNewest returns the most recently published publications.
	ListNewest(ctx context.Context, page, perPage int) (*PublicationListResult, error)
	// ListTopRated returns publications with the highest average rating.
	ListTopRated(ctx context.Context, page, perPage int) (*PublicationListResult, error)
	// ListMostImported returns publications with the most downloads.
	ListMostImported(ctx context.Context, page, perPage int) (*PublicationListResult, error)
	// GetCreatorProfile returns a creator's public profile with stats.
	GetCreatorProfile(ctx context.Context, userID string) (*CreatorProfile, error)

	// Rate creates or updates a rating on a publication.
	Rate(ctx context.Context, userID, publicationID string, rating int, reviewText *string) error
	// RemoveRating removes a user's rating on a publication.
	RemoveRating(ctx context.Context, userID, publicationID string) error
	// ListReviews returns paginated reviews for a publication.
	ListReviews(ctx context.Context, publicationID string, page, perPage int) (*ReviewListResult, error)
	// ToggleFavorite adds or removes a favorite, returning the new state.
	ToggleFavorite(ctx context.Context, userID, publicationID string) (favorited bool, err error)
	// RemoveFavorite removes a user's favorite on a publication.
	RemoveFavorite(ctx context.Context, userID, publicationID string) error
	// ListFavorites returns paginated publications favorited by a user.
	ListFavorites(ctx context.Context, userID string, page, perPage int) (*PublicationListResult, error)

	// Import imports a publication's creature into a campaign.
	Import(ctx context.Context, userID, publicationID, campaignID string) (*ImportResult, error)
	// Fork imports a publication as an editable copy into a campaign.
	Fork(ctx context.Context, userID, publicationID, campaignID string) (*ImportResult, error)
	// Flag flags a publication for moderation.
	Flag(ctx context.Context, userID, publicationID string, reason *string) error

	// SetUserFetcher sets the cross-plugin interface for user lookups.
	SetUserFetcher(uf UserFetcher)
	// SetEntityCreator sets the cross-plugin interface for entity creation.
	SetEntityCreator(ec EntityCreator)
	// SetCampaignRoleChecker sets the cross-plugin interface for campaign role checks.
	SetCampaignRoleChecker(rc CampaignRoleChecker)
}

// flagThreshold is the number of user flags that triggers auto-flagging.
const flagThreshold = 3

// bestiaryService is the default BestiaryService implementation.
type bestiaryService struct {
	repo     BestiaryRepository
	users    UserFetcher
	entities EntityCreator
	roles    CampaignRoleChecker
}

// NewBestiaryService creates a BestiaryService backed by the given repository.
func NewBestiaryService(repo BestiaryRepository) BestiaryService {
	return &bestiaryService{repo: repo}
}

// generateID creates a random UUID v4 string.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Publish creates a new bestiary publication after validating all input.
func (s *bestiaryService) Publish(ctx context.Context, creatorID string, input CreatePublicationInput) (*Publication, error) {
	if err := validateName(input.Name); err != nil {
		return nil, err
	}
	if err := validateStatblock(input.StatblockJSON); err != nil {
		return nil, err
	}
	if err := validateVisibility(input.Visibility); err != nil {
		return nil, err
	}
	validateOptionalText(&input.Description, 5000)
	validateOptionalText(&input.FlavorText, 5000)

	// Extract denormalized fields from the statblock for filtering.
	org, role, level := extractStatblockFields(input.StatblockJSON)

	slug, err := s.generateUniqueSlug(ctx, input.Name)
	if err != nil {
		return nil, fmt.Errorf("generate slug: %w", err)
	}

	p := &Publication{
		ID:               generateID(),
		CreatorID:        creatorID,
		SourceEntityID:   input.SourceEntityID,
		SourceCampaignID: input.SourceCampaignID,
		SystemID:         "drawsteel",
		Name:             sanitizeText(input.Name),
		Slug:             slug,
		Description:      sanitizeOptionalText(input.Description),
		FlavorText:       sanitizeOptionalText(input.FlavorText),
		StatblockJSON:    input.StatblockJSON,
		Version:          1,
		Tags:             input.Tags,
		Organization:     org,
		Role:             role,
		Level:            level,
		Visibility:       input.Visibility,
	}

	if err := s.repo.CreatePublication(ctx, p); err != nil {
		return nil, fmt.Errorf("publish creature: %w", err)
	}
	return p, nil
}

// GetBySlug returns a publication visible by its URL slug.
func (s *bestiaryService) GetBySlug(ctx context.Context, slug string) (*Publication, error) {
	return s.repo.GetBySlug(ctx, slug)
}

// GetByID returns a publication by UUID.
func (s *bestiaryService) GetByID(ctx context.Context, id string) (*Publication, error) {
	return s.repo.GetByID(ctx, id)
}

// Update modifies a publication after verifying ownership.
func (s *bestiaryService) Update(ctx context.Context, userID, publicationID string, input UpdatePublicationInput) (*Publication, error) {
	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return nil, err
	}
	if err := requireCreator(pub, userID); err != nil {
		return nil, err
	}

	// Apply partial updates.
	if input.Name != nil {
		if err := validateName(*input.Name); err != nil {
			return nil, err
		}
		pub.Name = sanitizeText(*input.Name)
		// Regenerate slug when name changes.
		slug, err := s.generateUniqueSlug(ctx, pub.Name)
		if err != nil {
			return nil, fmt.Errorf("regenerate slug: %w", err)
		}
		pub.Slug = slug
	}
	if input.Description != nil {
		validateOptionalText(&input.Description, 5000)
		pub.Description = sanitizeOptionalText(input.Description)
	}
	if input.FlavorText != nil {
		validateOptionalText(&input.FlavorText, 5000)
		pub.FlavorText = sanitizeOptionalText(input.FlavorText)
	}
	if len(input.StatblockJSON) > 0 {
		if err := validateStatblock(input.StatblockJSON); err != nil {
			return nil, err
		}
		pub.StatblockJSON = input.StatblockJSON
		pub.Organization, pub.Role, pub.Level = extractStatblockFields(input.StatblockJSON)
	}
	if len(input.Tags) > 0 {
		pub.Tags = input.Tags
	}

	pub.Version++

	if err := s.repo.UpdatePublication(ctx, pub); err != nil {
		return nil, fmt.Errorf("update publication: %w", err)
	}
	return pub, nil
}

// Archive soft-deletes a publication after verifying ownership.
func (s *bestiaryService) Archive(ctx context.Context, userID, publicationID string) error {
	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return err
	}
	if err := requireCreator(pub, userID); err != nil {
		return err
	}
	return s.repo.ArchivePublication(ctx, publicationID)
}

// ChangeVisibility updates the visibility state after verifying ownership.
// Creators can only set draft, published, unlisted, or archived.
func (s *bestiaryService) ChangeVisibility(ctx context.Context, userID, publicationID string, visibility string) error {
	if err := validateVisibility(visibility); err != nil {
		return err
	}
	// Creators cannot set flagged — that's automatic via moderation.
	if visibility == VisibilityFlagged {
		return apperror.NewForbidden("cannot manually flag a publication")
	}

	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return err
	}
	if err := requireCreator(pub, userID); err != nil {
		return err
	}
	// Cannot change visibility of a flagged publication (admin must unflag first).
	if pub.Visibility == VisibilityFlagged {
		return apperror.NewForbidden("publication is under moderation review")
	}

	return s.repo.UpdateVisibility(ctx, publicationID, visibility)
}

// ListPublished returns a paginated list of published publications.
func (s *bestiaryService) ListPublished(ctx context.Context, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListPublished(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list published: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// ListMyCreations returns the current user's publications across all visibility states.
func (s *bestiaryService) ListMyCreations(ctx context.Context, userID string, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListByCreator(ctx, userID, true, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list my creations: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// SetUserFetcher sets the cross-plugin interface for looking up user info.
func (s *bestiaryService) SetUserFetcher(uf UserFetcher) {
	s.users = uf
}

// Search performs a filtered, paginated search.
func (s *bestiaryService) Search(ctx context.Context, filters SearchFilters) (*PublicationListResult, error) {
	filters.Page, filters.PerPage = clampPagination(filters.Page, filters.PerPage)

	pubs, total, err := s.repo.SearchPublications(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return buildListResult(pubs, total, filters.Page, filters.PerPage), nil
}

// ListNewest returns the most recently published publications.
func (s *bestiaryService) ListNewest(ctx context.Context, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListNewest(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list newest: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// ListTopRated returns publications with the highest average rating (min 3 ratings).
func (s *bestiaryService) ListTopRated(ctx context.Context, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListTopRated(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list top rated: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// ListMostImported returns publications with the most downloads.
func (s *bestiaryService) ListMostImported(ctx context.Context, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListMostImported(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list most imported: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// GetCreatorProfile builds a creator's public profile from user info and stats.
func (s *bestiaryService) GetCreatorProfile(ctx context.Context, userID string) (*CreatorProfile, error) {
	stats, err := s.repo.GetCreatorStats(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("creator stats: %w", err)
	}

	profile := &CreatorProfile{
		UserID: userID,
		Stats:  *stats,
	}

	// Enrich with display name if UserFetcher is wired.
	if s.users != nil {
		info, err := s.users.GetUserPublicInfo(ctx, userID)
		if err == nil && info != nil {
			profile.DisplayName = info.DisplayName
			profile.AvatarURL = info.AvatarURL
		}
	}

	return profile, nil
}

// --- Rating & Favorite methods ---

// Rate creates or updates a user's rating on a publication.
// Self-rating is prevented: creators cannot rate their own publications.
func (s *bestiaryService) Rate(ctx context.Context, userID, publicationID string, rating int, reviewText *string) error {
	if rating < 1 || rating > 5 {
		return apperror.NewValidation("rating must be between 1 and 5")
	}
	if reviewText != nil && len(*reviewText) > 2000 {
		truncated := (*reviewText)[:2000]
		reviewText = &truncated
	}
	reviewText = sanitizeOptionalText(reviewText)

	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return err
	}
	if pub.CreatorID == userID {
		return apperror.NewForbidden("you cannot rate your own publication")
	}

	existing, err := s.repo.GetRating(ctx, userID, publicationID)
	if err != nil {
		return fmt.Errorf("check existing rating: %w", err)
	}

	if existing != nil {
		// Update existing rating; adjust aggregates by the difference.
		sumDelta := rating - existing.Rating
		existing.Rating = rating
		existing.ReviewText = reviewText
		if err := s.repo.UpdateRating(ctx, existing); err != nil {
			return err
		}
		return s.repo.AdjustRatingAggregates(ctx, publicationID, sumDelta, 0)
	}

	// New rating.
	rt := &Rating{
		ID:            generateID(),
		PublicationID: publicationID,
		UserID:        userID,
		Rating:        rating,
		ReviewText:    reviewText,
	}
	if err := s.repo.CreateRating(ctx, rt); err != nil {
		return err
	}
	return s.repo.AdjustRatingAggregates(ctx, publicationID, rating, 1)
}

// RemoveRating removes a user's rating and adjusts aggregates.
func (s *bestiaryService) RemoveRating(ctx context.Context, userID, publicationID string) error {
	existing, err := s.repo.GetRating(ctx, userID, publicationID)
	if err != nil {
		return fmt.Errorf("get rating for removal: %w", err)
	}
	if existing == nil {
		return apperror.NewNotFound("rating not found")
	}

	if err := s.repo.DeleteRating(ctx, userID, publicationID); err != nil {
		return err
	}
	return s.repo.AdjustRatingAggregates(ctx, publicationID, -existing.Rating, -1)
}

// ListReviews returns paginated reviews (ratings with text) for a publication.
func (s *bestiaryService) ListReviews(ctx context.Context, publicationID string, page, perPage int) (*ReviewListResult, error) {
	page, perPage = clampPagination(page, perPage)

	reviews, total, err := s.repo.ListReviews(ctx, publicationID, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list reviews: %w", err)
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}

	return &ReviewListResult{
		Reviews:    reviews,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

// ToggleFavorite adds a favorite if not already favorited, or removes it.
// Returns the new favorited state.
func (s *bestiaryService) ToggleFavorite(ctx context.Context, userID, publicationID string) (bool, error) {
	// Verify publication exists.
	if _, err := s.repo.GetByID(ctx, publicationID); err != nil {
		return false, err
	}

	favorited, err := s.repo.IsFavorited(ctx, userID, publicationID)
	if err != nil {
		return false, fmt.Errorf("check favorited: %w", err)
	}

	if favorited {
		if err := s.repo.RemoveFavorite(ctx, userID, publicationID); err != nil {
			return false, err
		}
		_ = s.repo.AdjustFavoriteCount(ctx, publicationID, -1)
		return false, nil
	}

	if err := s.repo.AddFavorite(ctx, userID, publicationID); err != nil {
		return false, err
	}
	_ = s.repo.AdjustFavoriteCount(ctx, publicationID, 1)
	return true, nil
}

// RemoveFavorite removes a user's favorite on a publication.
func (s *bestiaryService) RemoveFavorite(ctx context.Context, userID, publicationID string) error {
	favorited, err := s.repo.IsFavorited(ctx, userID, publicationID)
	if err != nil {
		return fmt.Errorf("check favorited: %w", err)
	}
	if !favorited {
		return nil // Idempotent — already not favorited.
	}

	if err := s.repo.RemoveFavorite(ctx, userID, publicationID); err != nil {
		return err
	}
	return s.repo.AdjustFavoriteCount(ctx, publicationID, -1)
}

// ListFavorites returns paginated publications favorited by a user.
func (s *bestiaryService) ListFavorites(ctx context.Context, userID string, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListFavorites(ctx, userID, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// SetEntityCreator sets the cross-plugin interface for creating entities.
func (s *bestiaryService) SetEntityCreator(ec EntityCreator) {
	s.entities = ec
}

// SetCampaignRoleChecker sets the cross-plugin interface for campaign role checks.
func (s *bestiaryService) SetCampaignRoleChecker(rc CampaignRoleChecker) {
	s.roles = rc
}

// Import imports a publication's creature into a campaign as a new entity.
func (s *bestiaryService) Import(ctx context.Context, userID, publicationID, campaignID string) (*ImportResult, error) {
	return s.importOrFork(ctx, userID, publicationID, campaignID, false)
}

// Fork imports a publication as an editable copy with "(Fork)" suffix.
func (s *bestiaryService) Fork(ctx context.Context, userID, publicationID, campaignID string) (*ImportResult, error) {
	return s.importOrFork(ctx, userID, publicationID, campaignID, true)
}

// importOrFork is the shared logic for Import and Fork.
func (s *bestiaryService) importOrFork(ctx context.Context, userID, publicationID, campaignID string, isFork bool) (*ImportResult, error) {
	if s.entities == nil {
		return nil, apperror.NewInternal(fmt.Errorf("entity creator not configured"))
	}
	if s.roles == nil {
		return nil, apperror.NewInternal(fmt.Errorf("campaign role checker not configured"))
	}

	// Verify user has Scribe+ role in the target campaign (role >= 2).
	hasRole, err := s.roles.HasMinRole(ctx, campaignID, userID, 2)
	if err != nil {
		return nil, fmt.Errorf("check campaign role: %w", err)
	}
	if !hasRole {
		return nil, apperror.NewForbidden("you need Scribe or higher role in the target campaign")
	}

	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return nil, err
	}

	// Only published or unlisted publications can be imported.
	if pub.Visibility != VisibilityPublished && pub.Visibility != VisibilityUnlisted {
		return nil, apperror.NewForbidden("publication is not available for import")
	}

	// Check for duplicate import (not for forks — forks always create new entities).
	if !isFork {
		exists, err := s.repo.ImportExists(ctx, publicationID, campaignID)
		if err != nil {
			return nil, fmt.Errorf("check import exists: %w", err)
		}
		if exists {
			return nil, apperror.NewConflict("this creature has already been imported into this campaign")
		}
	}

	// Create the entity in the target campaign.
	name := pub.Name
	if isFork {
		name += " (Fork)"
	}
	entityID, err := s.entities.CreateFromStatblock(ctx, campaignID, userID, name, pub.StatblockJSON)
	if err != nil {
		return nil, fmt.Errorf("create entity from statblock: %w", err)
	}

	// Record the import.
	imp := &Import{
		ID:            generateID(),
		PublicationID: publicationID,
		UserID:        userID,
		CampaignID:    campaignID,
		EntityID:      &entityID,
	}
	if err := s.repo.CreateImport(ctx, imp); err != nil {
		return nil, err
	}

	// Increment download counter.
	_ = s.repo.IncrementDownloads(ctx, publicationID)

	return &ImportResult{
		EntityID:      entityID,
		CampaignID:    campaignID,
		PublicationID: publicationID,
		CreatureName:  name,
	}, nil
}

// Flag flags a publication for moderation. Auto-flags at 3+ unique flags.
func (s *bestiaryService) Flag(ctx context.Context, userID, publicationID string, reason *string) error {
	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return err
	}

	// Cannot flag own publications.
	if pub.CreatorID == userID {
		return apperror.NewForbidden("you cannot flag your own publication")
	}

	// Note: reason text will be stored in moderation log in Phase 5.
	// For now we only track the flag count.

	newCount, err := s.repo.IncrementFlaggedCount(ctx, publicationID)
	if err != nil {
		return err
	}

	// Auto-flag when threshold reached.
	if newCount >= flagThreshold {
		_ = s.repo.AutoFlagIfThreshold(ctx, publicationID, flagThreshold)
	}

	return nil
}

// --- Validation helpers ---

// validateName checks that the publication name is non-empty and within bounds.
func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return apperror.NewValidation("creature name is required")
	}
	if len(name) > 200 {
		return apperror.NewValidation("creature name must be 200 characters or less")
	}
	return nil
}

// validateStatblock performs size and structural validation on the statblock JSON.
func validateStatblock(raw json.RawMessage) error {
	if len(raw) == 0 {
		return apperror.NewValidation("statblock_json is required")
	}
	if len(raw) > maxStatblockSize {
		return apperror.NewValidation("statblock_json exceeds maximum size of 100KB")
	}
	// Verify it's valid JSON.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return apperror.NewValidation("statblock_json must be a valid JSON object")
	}
	// Require name field in statblock.
	if _, ok := parsed["name"]; !ok {
		return apperror.NewValidation("statblock_json must contain a 'name' field")
	}
	return nil
}

// validateVisibility checks that the visibility value is valid.
func validateVisibility(v string) error {
	if !ValidVisibilities[v] {
		return apperror.NewValidation(fmt.Sprintf("invalid visibility: %q", v))
	}
	return nil
}

// validateOptionalText truncates an optional text pointer if it exceeds max length.
func validateOptionalText(s **string, maxLen int) {
	if *s == nil {
		return
	}
	v := **s
	if len(v) > maxLen {
		truncated := v[:maxLen]
		*s = &truncated
	}
}

// --- Ownership ---

// requireCreator returns a Forbidden error if the user is not the publication creator.
func requireCreator(pub *Publication, userID string) error {
	if pub.CreatorID != userID {
		return apperror.NewForbidden("you can only modify your own publications")
	}
	return nil
}

// --- Slug generation ---

// generateUniqueSlug creates a URL-safe slug from the name, appending a numeric
// suffix if the slug is already taken (e.g. "ashen-wyrm", "ashen-wyrm-2").
func (s *bestiaryService) generateUniqueSlug(ctx context.Context, name string) (string, error) {
	base := slugPattern.ReplaceAllString(strings.ToLower(name), "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "creature"
	}
	if len(base) > 100 {
		base = base[:100]
	}

	slug := base
	for i := 2; i < 100; i++ {
		exists, err := s.repo.SlugExists(ctx, slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	// Fallback: append UUID fragment for guaranteed uniqueness.
	return fmt.Sprintf("%s-%s", base, generateID()[:8]), nil
}

// --- Denormalization ---

// extractStatblockFields pulls level, organization, and role from the statblock
// JSON for indexed filtering. Returns nil pointers for missing fields.
func extractStatblockFields(raw json.RawMessage) (*string, *string, *int) {
	var sb struct {
		Level        *int    `json:"level"`
		Organization *string `json:"organization"`
		Role         *string `json:"role"`
	}
	if err := json.Unmarshal(raw, &sb); err != nil {
		return nil, nil, nil
	}
	return sb.Organization, sb.Role, sb.Level
}

// --- Text sanitization ---

// sanitizeText escapes HTML entities to prevent XSS.
func sanitizeText(s string) string {
	return html.EscapeString(strings.TrimSpace(s))
}

// sanitizeOptionalText escapes an optional text pointer.
func sanitizeOptionalText(s *string) *string {
	if s == nil {
		return nil
	}
	v := sanitizeText(*s)
	return &v
}

// --- Pagination ---

// clampPagination normalizes page and perPage to safe bounds.
func clampPagination(page, perPage int) (int, int) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage
}

// buildListResult converts a slice of Publications into a paginated response.
func buildListResult(pubs []Publication, total, page, perPage int) *PublicationListResult {
	summaries := make([]PublicationSummary, len(pubs))
	for i := range pubs {
		summaries[i] = SummaryFromPublication(&pubs[i])
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return &PublicationListResult{
		Results:    summaries,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}
}
