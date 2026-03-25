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
}

// bestiaryService is the default BestiaryService implementation.
type bestiaryService struct {
	repo BestiaryRepository
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
