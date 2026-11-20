package bestiary

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
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
	// ViewBySlug returns a publication by slug as userID may see it: hidden
	// ones are not found for anyone but the creator, and ids from the creator's
	// own campaign and moderation details are removed for everyone else.
	ViewBySlug(ctx context.Context, userID, slug string) (*Publication, error)
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
	// ListReviews returns paginated reviews for a publication the user may see.
	ListReviews(ctx context.Context, userID, publicationID string, page, perPage int) (*ReviewListResult, error)
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

	// Moderate performs an admin moderation action on a publication.
	Moderate(ctx context.Context, moderatorID, publicationID, action string, reason *string) error
	// GetModerationLog returns the moderation audit trail for a publication.
	GetModerationLog(ctx context.Context, publicationID string) ([]ModerationLogEntry, error)
	// ListFlagged returns paginated flagged publications for admin review.
	ListFlagged(ctx context.Context, page, perPage int) (*PublicationListResult, error)
	// GetStats returns aggregate bestiary statistics for admin dashboard.
	GetStats(ctx context.Context) (*BestiaryStats, error)

	// SetUserFetcher sets the cross-plugin interface for user lookups.
	SetUserFetcher(uf UserFetcher)
	// SetEntityCreator sets the cross-plugin interface for entity creation.
	SetEntityCreator(ec EntityCreator)
	// SetCampaignRoleChecker sets the cross-plugin interface for campaign role checks.
	SetCampaignRoleChecker(rc CampaignRoleChecker)
	// SetCampaignSystemFetcher sets the cross-plugin interface for resolving
	// a campaign's selected game system slug.
	SetCampaignSystemFetcher(csf CampaignSystemFetcher)
}

// flagThreshold is the number of user flags that triggers auto-flagging.
const flagThreshold = 3

// bestiaryService is the default BestiaryService implementation.
type bestiaryService struct {
	repo     BestiaryRepository
	users    UserFetcher
	entities EntityCreator
	roles    CampaignRoleChecker
	systems  CampaignSystemFetcher
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
	// Resolve the system tag. Priority: explicit input > source campaign's
	// selected system > empty string. We deliberately do NOT hard-code a
	// fallback slug here — bestiary content is system-dependent, and the
	// system tag must come from the publishing context. A hard-coded
	// fallback would silently mis-tag publications when the assumed
	// system isn't installed.
	if input.SystemID == "" && input.SourceCampaignID != nil && *input.SourceCampaignID != "" && s.systems != nil {
		if sysID, err := s.systems.GetCampaignSystemID(ctx, *input.SourceCampaignID); err == nil {
			input.SystemID = sysID
		}
	}
	validateOptionalText(&input.Description, 5000)
	validateOptionalText(&input.FlavorText, 5000)

	// Text is stored as written; every renderer escapes on output. Rewriting
	// it here lost real content (Draw Steel's "M < 1" potency tests) and put
	// entity codes in page names.
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
		SystemID:         input.SystemID,
		Name:             strings.TrimSpace(input.Name),
		Slug:             slug,
		Description:      trimOptionalText(input.Description),
		FlavorText:       trimOptionalText(input.FlavorText),
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

// ViewBySlug applies the visibility rule and the per-viewer redaction.
func (s *bestiaryService) ViewBySlug(ctx context.Context, userID, slug string) (*Publication, error) {
	pub, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if pub.Visibility != VisibilityPublished && pub.Visibility != VisibilityUnlisted && pub.CreatorID != userID {
		return nil, apperror.NewNotFound("publication not found")
	}
	return pub.ForViewer(userID), nil
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

	// Apply partial updates: only a key the caller sent changes anything.
	if input.Name.Present() && !input.Name.IsNull() {
		name, _ := input.Name.Get()
		if err := validateName(name); err != nil {
			return nil, err
		}
		pub.Name = strings.TrimSpace(name)
		// Regenerate slug when name changes.
		slug, err := s.generateUniqueSlug(ctx, pub.Name)
		if err != nil {
			return nil, fmt.Errorf("regenerate slug: %w", err)
		}
		pub.Slug = slug
	}
	// Description and flavor text are nullable: explicit null clears them.
	if input.Description.Present() {
		desc := input.Description.Ptr(nil)
		validateOptionalText(&desc, 5000)
		pub.Description = trimOptionalText(desc)
	}
	if input.FlavorText.Present() {
		flavor := input.FlavorText.Ptr(nil)
		validateOptionalText(&flavor, 5000)
		pub.FlavorText = trimOptionalText(flavor)
	}
	if patch.HasRawValue(input.StatblockJSON) {
		if err := validateStatblock(input.StatblockJSON); err != nil {
			return nil, err
		}
		pub.StatblockJSON = input.StatblockJSON
		pub.Organization, pub.Role, pub.Level = extractStatblockFields(input.StatblockJSON)
	}
	if patch.HasRawValue(input.Tags) {
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
	if err := s.requireNotModerated(ctx, pub); err != nil {
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
	if err := s.requireNotModerated(ctx, pub); err != nil {
		return err
	}

	return s.repo.UpdateVisibility(ctx, publicationID, visibility)
}

// requireNotModerated keeps a moderation decision out of the creator's hands.
// A flagged publication waits for a moderator, and one a moderator archived
// stays archived until a moderator restores it; otherwise the creator could
// archive a flagged publication and then publish it again, or undo a
// moderator's archive. The moderation log is the record of who archived it.
func (s *bestiaryService) requireNotModerated(ctx context.Context, pub *Publication) error {
	switch pub.Visibility {
	case VisibilityFlagged:
		return apperror.NewForbidden("publication is under moderation review")
	case VisibilityArchived:
		log, err := s.repo.GetModerationLog(ctx, pub.ID)
		if err != nil {
			return err
		}
		if len(log) > 0 && log[0].Action == "archive" {
			return apperror.NewForbidden("a moderator archived this publication")
		}
	}
	return nil
}

// viewable returns the publication if this user may see it: anyone may see a
// published or unlisted one, and only its creator sees the rest. It is the
// rule Show applies, for the routes that reach a publication by id.
func (s *bestiaryService) viewable(ctx context.Context, userID, publicationID string) (*Publication, error) {
	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return nil, err
	}
	if pub.Visibility != VisibilityPublished && pub.Visibility != VisibilityUnlisted && pub.CreatorID != userID {
		return nil, apperror.NewNotFound("publication not found")
	}
	return pub, nil
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
// The actual check-then-write-then-adjust is transactional in the repo layer.
func (s *bestiaryService) Rate(ctx context.Context, userID, publicationID string, rating int, reviewText *string) error {
	if rating < 1 || rating > 5 {
		return apperror.NewValidation("rating must be between 1 and 5")
	}
	if reviewText != nil && len(*reviewText) > 2000 {
		truncated := (*reviewText)[:2000]
		reviewText = &truncated
	}
	reviewText = trimOptionalText(reviewText)

	pub, err := s.viewable(ctx, userID, publicationID)
	if err != nil {
		return err
	}
	if pub.CreatorID == userID {
		return apperror.NewForbidden("you cannot rate your own publication")
	}

	_, err = s.repo.RatePublication(ctx, publicationID, userID, rating, reviewText)
	return err
}

// RemoveRating removes a user's rating and adjusts aggregates atomically.
func (s *bestiaryService) RemoveRating(ctx context.Context, userID, publicationID string) error {
	return s.repo.RemoveRatingWithAggregates(ctx, publicationID, userID)
}

// ListReviews returns paginated reviews (ratings with text) for a publication.
func (s *bestiaryService) ListReviews(ctx context.Context, userID, publicationID string, page, perPage int) (*ReviewListResult, error) {
	if _, err := s.viewable(ctx, userID, publicationID); err != nil {
		return nil, err
	}
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
// Returns the new favorited state. The toggle and count adjustment are
// transactional in the repo layer to prevent race conditions.
func (s *bestiaryService) ToggleFavorite(ctx context.Context, userID, publicationID string) (bool, error) {
	if _, err := s.viewable(ctx, userID, publicationID); err != nil {
		return false, err
	}

	return s.repo.ToggleFavoriteWithCount(ctx, publicationID, userID)
}

// RemoveFavorite removes a user's favorite on a publication. Idempotent.
// The delete and count adjustment are transactional in the repo layer.
func (s *bestiaryService) RemoveFavorite(ctx context.Context, userID, publicationID string) error {
	return s.repo.RemoveFavoriteWithCount(ctx, publicationID, userID)
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

// SetCampaignSystemFetcher sets the cross-plugin interface for resolving a
// campaign's selected game system slug.
func (s *bestiaryService) SetCampaignSystemFetcher(csf CampaignSystemFetcher) {
	s.systems = csf
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
//
// Only an Import is recorded in bestiary_imports: that table's unique key
// (publication, campaign) is what stops one creature being imported twice, and
// a fork is an editable copy that may be made any number of times. Recording
// forks there made a second fork fail after its page existed and blocked a
// plain Import for good.
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

	if !isFork {
		if err := s.refuseLiveImport(ctx, publicationID, campaignID); err != nil {
			return nil, err
		}
	}

	// Create the entity in the target campaign.
	name := pub.Name
	if isFork {
		name += " (Fork)"
	}
	entityID, err := s.entities.CreateFromStatblock(ctx, campaignID, userID, name, pub.SystemID, pub.StatblockJSON)
	if err != nil {
		return nil, fmt.Errorf("create entity from statblock: %w", err)
	}

	if !isFork {
		imp := &Import{
			ID:            generateID(),
			PublicationID: publicationID,
			UserID:        userID,
			CampaignID:    campaignID,
			EntityID:      &entityID,
		}
		// The page already exists, so a failed record (a concurrent import won
		// the unique key) must not turn the request into an error: the user has
		// their creature and the record only guards against repeat imports.
		if err := s.repo.CreateImport(ctx, imp); err != nil {
			slog.Warn("bestiary import: page created but import not recorded",
				slog.String("publication_id", publicationID),
				slog.String("campaign_id", campaignID),
				slog.Any("error", err))
		}
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

// refuseLiveImport returns Conflict when the creature was imported into the
// campaign and that page still exists. A record whose page was deleted is
// stale: it is removed so the creature can be imported again.
func (s *bestiaryService) refuseLiveImport(ctx context.Context, publicationID, campaignID string) error {
	imp, err := s.repo.GetImport(ctx, publicationID, campaignID)
	if err != nil {
		return fmt.Errorf("check import exists: %w", err)
	}
	if imp == nil {
		return nil
	}
	if imp.EntityID != nil {
		exists, err := s.entities.EntityExists(ctx, campaignID, *imp.EntityID)
		if err != nil {
			return fmt.Errorf("check imported page exists: %w", err)
		}
		if exists {
			return apperror.NewConflict("this creature has already been imported into this campaign")
		}
	}
	if err := s.repo.DeleteImport(ctx, publicationID, campaignID); err != nil {
		return fmt.Errorf("clear stale import: %w", err)
	}
	return nil
}

// Flag flags a publication for moderation. Auto-flags at 3+ unique user flags.
// Per-user deduplication, flag count increment, and auto-hide are all
// transactional in the repo layer to prevent abuse and race conditions.
func (s *bestiaryService) Flag(ctx context.Context, userID, publicationID string, reason *string) error {
	pub, err := s.viewable(ctx, userID, publicationID)
	if err != nil {
		return err
	}

	// Cannot flag own publications.
	if pub.CreatorID == userID {
		return apperror.NewForbidden("you cannot flag your own publication")
	}

	return s.repo.FlagPublicationAtomic(ctx, userID, publicationID, reason, flagThreshold)
}

// --- Admin moderation methods ---

// validModerationActions is the set of allowed moderation action values.
var validModerationActions = map[string]bool{
	"approve": true,
	"archive": true,
	"restore": true,
}

// Moderate performs an admin moderation action: approve (clear a flag),
// archive, or restore.
//
// Approve and restore put a publication back where it was before moderation
// hid it (PriorVisibility), never somewhere it was not: a draft stays a draft.
// Leaving the flagged state also zeroes the flag count, otherwise the very
// next flag would hide it again. A publication with no recorded prior state
// (hidden before that was tracked) goes back to published if it was flagged,
// which is the only way it could have been hidden then, and to draft if it was
// archived, so the creator decides whether to republish.
func (s *bestiaryService) Moderate(ctx context.Context, moderatorID, publicationID, action string, reason *string) error {
	if !validModerationActions[action] {
		return apperror.NewValidation(fmt.Sprintf("invalid moderation action: %q", action))
	}

	pub, err := s.repo.GetByID(ctx, publicationID)
	if err != nil {
		return err
	}

	newVisibility, prior, resetFlags, err := moderationOutcome(pub, action)
	if err != nil {
		return err
	}
	if err := s.repo.ApplyModeration(ctx, pub.ID, newVisibility, prior, resetFlags); err != nil {
		return err
	}

	// Stamp the reviewer.
	_ = s.repo.SetReviewedBy(ctx, publicationID, moderatorID)

	// Log the moderation action.
	entry := &ModerationLogEntry{
		ID:            generateID(),
		PublicationID: publicationID,
		ModeratorID:   moderatorID,
		Action:        action,
		Reason:        reason,
	}
	return s.repo.CreateModerationEntry(ctx, entry)
}

// moderationOutcome computes the visibility, the state to remember and
// whether to reset the flag count for a moderation action on pub.
func moderationOutcome(pub *Publication, action string) (visibility string, prior *string, resetFlags bool, err error) {
	switch action {
	case "archive":
		if pub.Visibility == VisibilityArchived {
			return VisibilityArchived, pub.PriorVisibility, false, nil
		}
		// A flagged publication already remembers what it was.
		if pub.Visibility == VisibilityFlagged {
			return VisibilityArchived, pub.PriorVisibility, true, nil
		}
		v := pub.Visibility
		return VisibilityArchived, &v, false, nil
	case "approve", "restore":
		switch pub.Visibility {
		case VisibilityFlagged:
			return restoredVisibility(pub, VisibilityPublished), nil, true, nil
		case VisibilityArchived:
			if action == "approve" {
				return "", nil, false, apperror.NewConflict("only a flagged publication can be approved; use restore for an archived one")
			}
			return restoredVisibility(pub, VisibilityDraft), nil, false, nil
		default:
			if action == "restore" {
				return "", nil, false, apperror.NewConflict("publication is not archived or flagged")
			}
			// Nothing hides it: approving only clears leftover flags.
			return pub.Visibility, nil, true, nil
		}
	}
	return "", nil, false, apperror.NewValidation(fmt.Sprintf("invalid moderation action: %q", action))
}

// restoredVisibility is the recorded prior state, or fallback when none is
// recorded.
func restoredVisibility(pub *Publication, fallback string) string {
	if pub.PriorVisibility != nil {
		switch *pub.PriorVisibility {
		case VisibilityDraft, VisibilityPublished, VisibilityUnlisted, VisibilityArchived:
			return *pub.PriorVisibility
		}
	}
	return fallback
}

// GetModerationLog returns the full audit trail for a publication.
func (s *bestiaryService) GetModerationLog(ctx context.Context, publicationID string) ([]ModerationLogEntry, error) {
	return s.repo.GetModerationLog(ctx, publicationID)
}

// ListFlagged returns flagged publications for admin review.
func (s *bestiaryService) ListFlagged(ctx context.Context, page, perPage int) (*PublicationListResult, error) {
	page, perPage = clampPagination(page, perPage)

	pubs, total, err := s.repo.ListFlagged(ctx, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("list flagged: %w", err)
	}

	return buildListResult(pubs, total, page, perPage), nil
}

// GetStats returns aggregate bestiary statistics for the admin dashboard.
func (s *bestiaryService) GetStats(ctx context.Context) (*BestiaryStats, error) {
	return s.repo.GetBestiaryStats(ctx)
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

// --- Text handling ---

// trimOptionalText trims an optional text pointer. It deliberately does not
// escape: stored text is what the author wrote, and escaping belongs to the
// renderer that knows its output context.
func trimOptionalText(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
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
