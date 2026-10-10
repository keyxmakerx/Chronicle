package bestiary

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- Mocks ---
//
// mockBestiaryRepo stubs the REPOSITORY's SQL so these tests exercise only
// the SERVICE's own orchestration (validation, ownership checks, partial
// updates, cross-plugin call sequencing) — not the real SQL. Every method
// has a func field with a safe zero-value default so a test only wires the
// calls it cares about.

type mockBestiaryRepo struct {
	createPublicationFn  func(ctx context.Context, p *Publication) error
	getByIDFn            func(ctx context.Context, id string) (*Publication, error)
	getBySlugFn          func(ctx context.Context, slug string) (*Publication, error)
	updatePublicationFn  func(ctx context.Context, p *Publication) error
	archivePublicationFn func(ctx context.Context, id string) error
	updateVisibilityFn   func(ctx context.Context, id, visibility string) error

	listPublishedFn func(ctx context.Context, page, perPage int) ([]Publication, int, error)
	listByCreatorFn func(ctx context.Context, creatorID string, includeAll bool, page, perPage int) ([]Publication, int, error)

	searchPublicationsFn func(ctx context.Context, filters SearchFilters) ([]Publication, int, error)
	listNewestFn         func(ctx context.Context, page, perPage int) ([]Publication, int, error)
	listTopRatedFn       func(ctx context.Context, page, perPage int) ([]Publication, int, error)
	listMostImportedFn   func(ctx context.Context, page, perPage int) ([]Publication, int, error)

	getCreatorStatsFn func(ctx context.Context, creatorID string) (*CreatorStats, error)

	ratePublicationFn        func(ctx context.Context, publicationID, userID string, rating int, reviewText *string) (bool, error)
	removeRatingWithAggFn    func(ctx context.Context, publicationID, userID string) error
	listReviewsFn            func(ctx context.Context, publicationID string, page, perPage int) ([]Rating, int, error)
	adjustRatingAggregatesFn func(ctx context.Context, publicationID string, sumDelta, countDelta int) error

	toggleFavoriteWithCountFn func(ctx context.Context, publicationID, userID string) (bool, error)
	removeFavoriteWithCountFn func(ctx context.Context, publicationID, userID string) error
	listFavoritesFn           func(ctx context.Context, userID string, page, perPage int) ([]Publication, int, error)

	createImportFn       func(ctx context.Context, imp *Import) error
	importExistsFn       func(ctx context.Context, publicationID, campaignID string) (bool, error)
	incrementDownloadsFn func(ctx context.Context, publicationID string) error

	flagPublicationAtomicFn func(ctx context.Context, userID, publicationID string, reason *string, threshold int) error

	listFlaggedFn           func(ctx context.Context, page, perPage int) ([]Publication, int, error)
	createModerationEntryFn func(ctx context.Context, entry *ModerationLogEntry) error
	getModerationLogFn      func(ctx context.Context, publicationID string) ([]ModerationLogEntry, error)
	getBestiaryStatsFn      func(ctx context.Context) (*BestiaryStats, error)
	setReviewedByFn         func(ctx context.Context, publicationID, moderatorID string) error

	slugExistsFn func(ctx context.Context, slug string) (bool, error)

	// call recorders, for asserting the service called through with the
	// right arguments rather than just that it didn't error.
	updateVisibilityCalls   []string
	incrementDownloadsCalls int
}

func (m *mockBestiaryRepo) CreatePublication(ctx context.Context, p *Publication) error {
	if m.createPublicationFn != nil {
		return m.createPublicationFn(ctx, p)
	}
	return nil
}

func (m *mockBestiaryRepo) GetByID(ctx context.Context, id string) (*Publication, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, apperror.NewNotFound("publication not found")
}

func (m *mockBestiaryRepo) GetBySlug(ctx context.Context, slug string) (*Publication, error) {
	if m.getBySlugFn != nil {
		return m.getBySlugFn(ctx, slug)
	}
	return nil, apperror.NewNotFound("publication not found")
}

func (m *mockBestiaryRepo) UpdatePublication(ctx context.Context, p *Publication) error {
	if m.updatePublicationFn != nil {
		return m.updatePublicationFn(ctx, p)
	}
	return nil
}

func (m *mockBestiaryRepo) ArchivePublication(ctx context.Context, id string) error {
	if m.archivePublicationFn != nil {
		return m.archivePublicationFn(ctx, id)
	}
	return nil
}

func (m *mockBestiaryRepo) UpdateVisibility(ctx context.Context, id, visibility string) error {
	m.updateVisibilityCalls = append(m.updateVisibilityCalls, id+":"+visibility)
	if m.updateVisibilityFn != nil {
		return m.updateVisibilityFn(ctx, id, visibility)
	}
	return nil
}

func (m *mockBestiaryRepo) ListPublished(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	if m.listPublishedFn != nil {
		return m.listPublishedFn(ctx, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) ListByCreator(ctx context.Context, creatorID string, includeAll bool, page, perPage int) ([]Publication, int, error) {
	if m.listByCreatorFn != nil {
		return m.listByCreatorFn(ctx, creatorID, includeAll, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) SearchPublications(ctx context.Context, filters SearchFilters) ([]Publication, int, error) {
	if m.searchPublicationsFn != nil {
		return m.searchPublicationsFn(ctx, filters)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) ListNewest(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	if m.listNewestFn != nil {
		return m.listNewestFn(ctx, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) ListTopRated(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	if m.listTopRatedFn != nil {
		return m.listTopRatedFn(ctx, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) ListMostImported(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	if m.listMostImportedFn != nil {
		return m.listMostImportedFn(ctx, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) GetCreatorStats(ctx context.Context, creatorID string) (*CreatorStats, error) {
	if m.getCreatorStatsFn != nil {
		return m.getCreatorStatsFn(ctx, creatorID)
	}
	return &CreatorStats{}, nil
}

func (m *mockBestiaryRepo) CreateRating(ctx context.Context, r *Rating) error { return nil }
func (m *mockBestiaryRepo) UpdateRating(ctx context.Context, r *Rating) error { return nil }
func (m *mockBestiaryRepo) DeleteRating(ctx context.Context, userID, publicationID string) error {
	return nil
}
func (m *mockBestiaryRepo) GetRating(ctx context.Context, userID, publicationID string) (*Rating, error) {
	return nil, nil
}

func (m *mockBestiaryRepo) ListReviews(ctx context.Context, publicationID string, page, perPage int) ([]Rating, int, error) {
	if m.listReviewsFn != nil {
		return m.listReviewsFn(ctx, publicationID, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) AdjustRatingAggregates(ctx context.Context, publicationID string, sumDelta, countDelta int) error {
	if m.adjustRatingAggregatesFn != nil {
		return m.adjustRatingAggregatesFn(ctx, publicationID, sumDelta, countDelta)
	}
	return nil
}

func (m *mockBestiaryRepo) AddFavorite(ctx context.Context, userID, publicationID string) error {
	return nil
}
func (m *mockBestiaryRepo) RemoveFavorite(ctx context.Context, userID, publicationID string) error {
	return nil
}
func (m *mockBestiaryRepo) IsFavorited(ctx context.Context, userID, publicationID string) (bool, error) {
	return false, nil
}

func (m *mockBestiaryRepo) ListFavorites(ctx context.Context, userID string, page, perPage int) ([]Publication, int, error) {
	if m.listFavoritesFn != nil {
		return m.listFavoritesFn(ctx, userID, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) AdjustFavoriteCount(ctx context.Context, publicationID string, delta int) error {
	return nil
}

func (m *mockBestiaryRepo) CreateImport(ctx context.Context, imp *Import) error {
	if m.createImportFn != nil {
		return m.createImportFn(ctx, imp)
	}
	return nil
}

func (m *mockBestiaryRepo) ImportExists(ctx context.Context, publicationID, campaignID string) (bool, error) {
	if m.importExistsFn != nil {
		return m.importExistsFn(ctx, publicationID, campaignID)
	}
	return false, nil
}

func (m *mockBestiaryRepo) IncrementDownloads(ctx context.Context, publicationID string) error {
	m.incrementDownloadsCalls++
	if m.incrementDownloadsFn != nil {
		return m.incrementDownloadsFn(ctx, publicationID)
	}
	return nil
}

func (m *mockBestiaryRepo) RatePublication(ctx context.Context, publicationID, userID string, rating int, reviewText *string) (bool, error) {
	if m.ratePublicationFn != nil {
		return m.ratePublicationFn(ctx, publicationID, userID, rating, reviewText)
	}
	return true, nil
}

func (m *mockBestiaryRepo) RemoveRatingWithAggregates(ctx context.Context, publicationID, userID string) error {
	if m.removeRatingWithAggFn != nil {
		return m.removeRatingWithAggFn(ctx, publicationID, userID)
	}
	return nil
}

func (m *mockBestiaryRepo) ToggleFavoriteWithCount(ctx context.Context, publicationID, userID string) (bool, error) {
	if m.toggleFavoriteWithCountFn != nil {
		return m.toggleFavoriteWithCountFn(ctx, publicationID, userID)
	}
	return true, nil
}

func (m *mockBestiaryRepo) RemoveFavoriteWithCount(ctx context.Context, publicationID, userID string) error {
	if m.removeFavoriteWithCountFn != nil {
		return m.removeFavoriteWithCountFn(ctx, publicationID, userID)
	}
	return nil
}

func (m *mockBestiaryRepo) FlagPublicationAtomic(ctx context.Context, userID, publicationID string, reason *string, threshold int) error {
	if m.flagPublicationAtomicFn != nil {
		return m.flagPublicationAtomicFn(ctx, userID, publicationID, reason, threshold)
	}
	return nil
}

func (m *mockBestiaryRepo) FlagExists(ctx context.Context, userID, publicationID string) (bool, error) {
	return false, nil
}

func (m *mockBestiaryRepo) IncrementFlaggedCount(ctx context.Context, publicationID string) (int, error) {
	return 0, nil
}

func (m *mockBestiaryRepo) AutoFlagIfThreshold(ctx context.Context, publicationID string, threshold int) error {
	return nil
}

func (m *mockBestiaryRepo) ListFlagged(ctx context.Context, page, perPage int) ([]Publication, int, error) {
	if m.listFlaggedFn != nil {
		return m.listFlaggedFn(ctx, page, perPage)
	}
	return nil, 0, nil
}

func (m *mockBestiaryRepo) CreateModerationEntry(ctx context.Context, entry *ModerationLogEntry) error {
	if m.createModerationEntryFn != nil {
		return m.createModerationEntryFn(ctx, entry)
	}
	return nil
}

func (m *mockBestiaryRepo) GetModerationLog(ctx context.Context, publicationID string) ([]ModerationLogEntry, error) {
	if m.getModerationLogFn != nil {
		return m.getModerationLogFn(ctx, publicationID)
	}
	return nil, nil
}

func (m *mockBestiaryRepo) GetBestiaryStats(ctx context.Context) (*BestiaryStats, error) {
	if m.getBestiaryStatsFn != nil {
		return m.getBestiaryStatsFn(ctx)
	}
	return &BestiaryStats{}, nil
}

func (m *mockBestiaryRepo) SetReviewedBy(ctx context.Context, publicationID, moderatorID string) error {
	if m.setReviewedByFn != nil {
		return m.setReviewedByFn(ctx, publicationID, moderatorID)
	}
	return nil
}

func (m *mockBestiaryRepo) SlugExists(ctx context.Context, slug string) (bool, error) {
	if m.slugExistsFn != nil {
		return m.slugExistsFn(ctx, slug)
	}
	return false, nil
}

// mockUserFetcher stubs the cross-plugin user lookup used by GetCreatorProfile.
type mockUserFetcher struct {
	infoFn func(ctx context.Context, userID string) (*UserInfo, error)
}

func (m *mockUserFetcher) GetUserPublicInfo(ctx context.Context, userID string) (*UserInfo, error) {
	if m.infoFn != nil {
		return m.infoFn(ctx, userID)
	}
	return nil, nil
}

// mockEntityCreator stubs the cross-plugin entity creation used by Import/Fork.
type mockEntityCreator struct {
	createFn func(ctx context.Context, campaignID, userID, name string, statblock json.RawMessage) (string, error)
}

func (m *mockEntityCreator) CreateFromStatblock(ctx context.Context, campaignID, userID, name string, statblock json.RawMessage) (string, error) {
	if m.createFn != nil {
		return m.createFn(ctx, campaignID, userID, name, statblock)
	}
	return "entity-1", nil
}

// mockRoleChecker stubs the cross-plugin campaign role check used by Import/Fork.
type mockRoleChecker struct {
	hasMinRoleFn func(ctx context.Context, campaignID, userID string, minRole int) (bool, error)
}

func (m *mockRoleChecker) HasMinRole(ctx context.Context, campaignID, userID string, minRole int) (bool, error) {
	if m.hasMinRoleFn != nil {
		return m.hasMinRoleFn(ctx, campaignID, userID, minRole)
	}
	return true, nil
}

// newTestBestiaryService builds a bestiaryService directly (bypassing
// NewBestiaryService) so tests can wire only the mocks they need.
func newTestBestiaryService(repo BestiaryRepository) *bestiaryService {
	return &bestiaryService{repo: repo}
}

func validStatblock(name string) json.RawMessage {
	return json.RawMessage(`{"name":"` + name + `"}`)
}

// --- Publish ---

func TestPublish_ValidatesName(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	_, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:          "  ",
		StatblockJSON: validStatblock("Goblin"),
		Visibility:    VisibilityDraft,
	})
	if err == nil {
		t.Fatal("expected validation error for blank name")
	}
}

func TestPublish_ValidatesStatblock(t *testing.T) {
	cases := []struct {
		name      string
		statblock json.RawMessage
	}{
		{"empty", nil},
		{"not json object", json.RawMessage(`not json`)},
		{"missing name field", json.RawMessage(`{"level":1}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestBestiaryService(&mockBestiaryRepo{})
			_, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
				Name:          "Goblin",
				StatblockJSON: tc.statblock,
				Visibility:    VisibilityDraft,
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPublish_ValidatesVisibility(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	_, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:          "Goblin",
		StatblockJSON: validStatblock("Goblin"),
		Visibility:    "not-a-real-state",
	})
	if err == nil {
		t.Fatal("expected validation error for invalid visibility")
	}
}

// TestPublish_Success proves a valid input reaches the repository with a
// sanitized name, a generated slug and denormalized statblock fields.
func TestPublish_Success(t *testing.T) {
	var created *Publication
	repo := &mockBestiaryRepo{
		createPublicationFn: func(_ context.Context, p *Publication) error {
			created = p
			return nil
		},
	}
	svc := newTestBestiaryService(repo)

	pub, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:          "Ashen Wyrm",
		StatblockJSON: json.RawMessage(`{"name":"Ashen Wyrm","organization":"solo","level":5}`),
		Visibility:    VisibilityDraft,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Slug != "ashen-wyrm" {
		t.Errorf("expected slug 'ashen-wyrm', got %q", pub.Slug)
	}
	if pub.Version != 1 {
		t.Errorf("expected version 1, got %d", pub.Version)
	}
	if pub.Organization == nil || *pub.Organization != "solo" {
		t.Errorf("expected organization to be extracted from statblock, got %v", pub.Organization)
	}
	if pub.Level == nil || *pub.Level != 5 {
		t.Errorf("expected level to be extracted from statblock, got %v", pub.Level)
	}
	if created == nil || created.CreatorID != "user-1" {
		t.Fatal("expected repo.CreatePublication to be called with the creator id")
	}
}

// TestPublish_SlugCollisionAppendsSuffix proves the service asks the
// repository whether each candidate slug is taken and appends "-2" rather
// than colliding.
func TestPublish_SlugCollisionAppendsSuffix(t *testing.T) {
	calls := 0
	repo := &mockBestiaryRepo{
		slugExistsFn: func(_ context.Context, slug string) (bool, error) {
			calls++
			return slug == "goblin", nil // only the bare slug is taken
		},
	}
	svc := newTestBestiaryService(repo)

	pub, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:          "Goblin",
		StatblockJSON: validStatblock("Goblin"),
		Visibility:    VisibilityDraft,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Slug != "goblin-2" {
		t.Errorf("expected slug 'goblin-2', got %q", pub.Slug)
	}
	if calls != 2 {
		t.Errorf("expected 2 SlugExists calls, got %d", calls)
	}
}

// TestPublish_SanitizesStatblockBeforeExtraction proves the write-side XSS
// scrub runs before organization/role/level are denormalized, so a hostile
// statblock cannot leave markup in the indexed columns.
func TestPublish_SanitizesStatblockBeforeExtraction(t *testing.T) {
	var created *Publication
	repo := &mockBestiaryRepo{
		createPublicationFn: func(_ context.Context, p *Publication) error {
			created = p
			return nil
		},
	}
	svc := newTestBestiaryService(repo)

	_, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:          "Goblin",
		StatblockJSON: json.RawMessage(`{"name":"Goblin","organization":"<script>x</script>horde"}`),
		Visibility:    VisibilityDraft,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created.Organization == nil {
		t.Fatal("expected organization to be extracted")
	}
	if *created.Organization == "<script>x</script>horde" {
		t.Errorf("organization column carries unsanitized markup: %q", *created.Organization)
	}
}

// TestPublish_ResolvesSystemFromSourceCampaign proves the priority order:
// an explicit SystemID wins, and only an absent one falls back to the
// source campaign's selected system via CampaignSystemFetcher.
func TestPublish_ResolvesSystemFromSourceCampaign(t *testing.T) {
	campID := "camp-1"
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	svc.SetCampaignSystemFetcher(&mockSystemFetcher{
		getFn: func(_ context.Context, _ string) (string, error) { return "drawsteel", nil },
	})

	pub, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:             "Goblin",
		StatblockJSON:    validStatblock("Goblin"),
		Visibility:       VisibilityDraft,
		SourceCampaignID: &campID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.SystemID != "drawsteel" {
		t.Errorf("expected system resolved from campaign, got %q", pub.SystemID)
	}

	// An explicit SystemID must win over the campaign's system.
	pub2, err := svc.Publish(context.Background(), "user-1", CreatePublicationInput{
		Name:             "Orc",
		StatblockJSON:    validStatblock("Orc"),
		Visibility:       VisibilityDraft,
		SourceCampaignID: &campID,
		SystemID:         "dnd5e",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub2.SystemID != "dnd5e" {
		t.Errorf("expected explicit system to win, got %q", pub2.SystemID)
	}
}

// mockSystemFetcher stubs the cross-plugin campaign-system lookup.
type mockSystemFetcher struct {
	getFn func(ctx context.Context, campaignID string) (string, error)
}

func (m *mockSystemFetcher) GetCampaignSystemID(ctx context.Context, campaignID string) (string, error) {
	if m.getFn != nil {
		return m.getFn(ctx, campaignID)
	}
	return "", nil
}

// --- GetBySlug / GetByID ---

func TestGetBySlug_PassesThrough(t *testing.T) {
	want := &Publication{ID: "p1", Slug: "goblin"}
	repo := &mockBestiaryRepo{
		getBySlugFn: func(_ context.Context, slug string) (*Publication, error) {
			if slug != "goblin" {
				t.Errorf("expected slug 'goblin', got %q", slug)
			}
			return want, nil
		},
	}
	svc := newTestBestiaryService(repo)
	got, err := svc.GetBySlug(context.Background(), "goblin")
	if err != nil || got != want {
		t.Fatalf("expected passthrough, got %v, %v", got, err)
	}
}

func TestGetByID_NotFoundPropagates(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return nil, apperror.NewNotFound("publication not found")
		},
	}
	svc := newTestBestiaryService(repo)
	_, err := svc.GetByID(context.Background(), "missing")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "not_found" {
		t.Fatalf("expected not_found error, got %v", err)
	}
}

// --- Update ---

func TestUpdate_RejectsNonCreator(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1"}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	name := "New Name"
	_, err := svc.Update(context.Background(), "someone-else", "p1", UpdatePublicationInput{Name: patch.Of(name)})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

// TestUpdate_PartialUpdateContract pins the absent-preserves / present-replaces
// half of the partial-update contract: sending only {name} must not disturb
// description, flavor text, tags or the statblock — the exact failure shape
// the contract note in CLAUDE.md warns about.
func TestUpdate_PartialUpdateContract(t *testing.T) {
	desc := "original description"
	flavor := "original flavor"
	existing := &Publication{
		ID:            "p1",
		CreatorID:     "user-1",
		Name:          "Old Name",
		Description:   &desc,
		FlavorText:    &flavor,
		StatblockJSON: json.RawMessage(`{"name":"Old Name","level":3}`),
		Tags:          json.RawMessage(`["a","b"]`),
		Version:       1,
	}
	var updated *Publication
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) { return existing, nil },
		updatePublicationFn: func(_ context.Context, p *Publication) error {
			updated = p
			return nil
		},
	}
	svc := newTestBestiaryService(repo)

	newName := "New Name"
	pub, err := svc.Update(context.Background(), "user-1", "p1", UpdatePublicationInput{Name: patch.Of(newName)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Name != "New Name" {
		t.Errorf("expected name updated, got %q", pub.Name)
	}
	if pub.Description == nil || *pub.Description != desc {
		t.Errorf("name-only update must preserve description, got %v", pub.Description)
	}
	if pub.FlavorText == nil || *pub.FlavorText != flavor {
		t.Errorf("name-only update must preserve flavor text, got %v", pub.FlavorText)
	}
	if string(pub.StatblockJSON) != `{"name":"Old Name","level":3}` {
		t.Errorf("name-only update must preserve statblock, got %s", pub.StatblockJSON)
	}
	if string(pub.Tags) != `["a","b"]` {
		t.Errorf("name-only update must preserve tags, got %s", pub.Tags)
	}
	if pub.Version != 2 {
		t.Errorf("expected version bumped to 2, got %d", pub.Version)
	}
	if updated == nil {
		t.Fatal("expected repo.UpdatePublication to be called")
	}
}

// TestUpdate_PartialUpdateWireBodies decodes real JSON bodies, because the
// absent/null distinction only exists at the bind layer: absent preserves,
// explicit null clears a nullable column and preserves a required one, a value
// replaces.
func TestUpdate_PartialUpdateWireBodies(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantName      string
		wantDesc      string // "" with wantDescNil=false means "empty string"
		wantDescNil   bool
		wantFlavor    string
		wantFlavorNil bool
		wantTags      string
		wantStat      string
	}{
		{"empty body preserves everything", `{}`, "Old", "d", false, "f", false, `["a"]`, `{"name":"Old","level":3}`},
		{"explicit null clears description only", `{"description":null}`, "Old", "", true, "f", false, `["a"]`, `{"name":"Old","level":3}`},
		{"explicit null clears flavor only", `{"flavor_text":null}`, "Old", "d", false, "", true, `["a"]`, `{"name":"Old","level":3}`},
		{"null name, tags and statblock preserve", `{"name":null,"tags":null,"statblock_json":null}`, "Old", "d", false, "f", false, `["a"]`, `{"name":"Old","level":3}`},
		{"value replaces description", `{"description":"new"}`, "Old", "new", false, "f", false, `["a"]`, `{"name":"Old","level":3}`},
		{"tags replace", `{"tags":["x","y"]}`, "Old", "d", false, "f", false, `["x","y"]`, `{"name":"Old","level":3}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desc, flavor := "d", "f"
			existing := &Publication{
				ID: "p1", CreatorID: "u", Name: "Old", Slug: "old",
				Description: &desc, FlavorText: &flavor,
				Tags:          json.RawMessage(`["a"]`),
				StatblockJSON: json.RawMessage(`{"name":"Old","level":3}`),
			}
			repo := &mockBestiaryRepo{
				getByIDFn: func(_ context.Context, _ string) (*Publication, error) { return existing, nil },
			}
			var in UpdatePublicationInput
			if err := json.Unmarshal([]byte(tt.body), &in); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			pub, err := newTestBestiaryService(repo).Update(context.Background(), "u", "p1", in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pub.Name != tt.wantName {
				t.Errorf("name = %q, want %q", pub.Name, tt.wantName)
			}
			if tt.wantDescNil {
				if pub.Description != nil {
					t.Errorf("description = %q, want cleared", *pub.Description)
				}
			} else if pub.Description == nil || *pub.Description != tt.wantDesc {
				t.Errorf("description = %v, want %q", pub.Description, tt.wantDesc)
			}
			if tt.wantFlavorNil {
				if pub.FlavorText != nil {
					t.Errorf("flavor = %q, want cleared", *pub.FlavorText)
				}
			} else if pub.FlavorText == nil || *pub.FlavorText != tt.wantFlavor {
				t.Errorf("flavor = %v, want %q", pub.FlavorText, tt.wantFlavor)
			}
			if string(pub.Tags) != tt.wantTags {
				t.Errorf("tags = %s, want %s", pub.Tags, tt.wantTags)
			}
			if string(pub.StatblockJSON) != tt.wantStat {
				t.Errorf("statblock = %s, want %s", pub.StatblockJSON, tt.wantStat)
			}
		})
	}
}

// TestUpdate_NameChangeRegeneratesSlug proves a rename asks the repo for a
// fresh unique slug rather than keeping a now-stale one.
func TestUpdate_NameChangeRegeneratesSlug(t *testing.T) {
	existing := &Publication{ID: "p1", CreatorID: "user-1", Name: "Old", Slug: "old"}
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) { return existing, nil },
	}
	svc := newTestBestiaryService(repo)

	newName := "Brand New"
	pub, err := svc.Update(context.Background(), "user-1", "p1", UpdatePublicationInput{Name: patch.Of(newName)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Slug != "brand-new" {
		t.Errorf("expected regenerated slug 'brand-new', got %q", pub.Slug)
	}
}

// TestUpdate_StatblockChangeRevalidatesAndRedenormalizes proves a statblock
// replacement is validated, sanitized and re-extracted, not just stored raw.
func TestUpdate_StatblockChangeRevalidatesAndRedenormalizes(t *testing.T) {
	existing := &Publication{ID: "p1", CreatorID: "user-1", Name: "Goblin"}
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) { return existing, nil },
	}
	svc := newTestBestiaryService(repo)

	newStatblock := json.RawMessage(`{"name":"Goblin","organization":"<b>horde</b>","level":7}`)
	pub, err := svc.Update(context.Background(), "user-1", "p1", UpdatePublicationInput{StatblockJSON: newStatblock})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pub.Level == nil || *pub.Level != 7 {
		t.Errorf("expected level re-denormalized to 7, got %v", pub.Level)
	}
	if pub.Organization == nil || *pub.Organization == "<b>horde</b>" {
		t.Errorf("expected sanitized organization, got %v", pub.Organization)
	}
}

func TestUpdate_InvalidStatblockRejected(t *testing.T) {
	existing := &Publication{ID: "p1", CreatorID: "user-1"}
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) { return existing, nil },
	}
	svc := newTestBestiaryService(repo)
	_, err := svc.Update(context.Background(), "user-1", "p1", UpdatePublicationInput{
		StatblockJSON: json.RawMessage(`{"level":1}`), // missing required "name"
	})
	if err == nil {
		t.Fatal("expected validation error for statblock missing name")
	}
}

// --- Archive ---

func TestArchive_RejectsNonCreator(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1"}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	err := svc.Archive(context.Background(), "someone-else", "p1")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestArchive_Success(t *testing.T) {
	archived := ""
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "user-1"}, nil
		},
		archivePublicationFn: func(_ context.Context, id string) error {
			archived = id
			return nil
		},
	}
	svc := newTestBestiaryService(repo)
	if err := svc.Archive(context.Background(), "user-1", "p1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if archived != "p1" {
		t.Errorf("expected ArchivePublication called with 'p1', got %q", archived)
	}
}

// --- ChangeVisibility ---

func TestChangeVisibility_RejectsInvalidValue(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	err := svc.ChangeVisibility(context.Background(), "user-1", "p1", "not-a-state")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// TestChangeVisibility_CreatorCannotSetFlagged proves the service rejects a
// creator trying to manually set 'flagged' — that state is moderation-only.
func TestChangeVisibility_CreatorCannotSetFlagged(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	err := svc.ChangeVisibility(context.Background(), "user-1", "p1", VisibilityFlagged)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestChangeVisibility_RejectsNonCreator(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1", Visibility: VisibilityDraft}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	err := svc.ChangeVisibility(context.Background(), "someone-else", "p1", VisibilityPublished)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

// TestChangeVisibility_FlaggedPublicationLocked proves a creator cannot
// change visibility away from 'flagged' themselves — only admin moderation can.
func TestChangeVisibility_FlaggedPublicationLocked(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "user-1", Visibility: VisibilityFlagged}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	err := svc.ChangeVisibility(context.Background(), "user-1", "p1", VisibilityDraft)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestChangeVisibility_Success(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "user-1", Visibility: VisibilityDraft}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	if err := svc.ChangeVisibility(context.Background(), "user-1", "p1", VisibilityPublished); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.updateVisibilityCalls) != 1 || repo.updateVisibilityCalls[0] != "p1:published" {
		t.Errorf("expected UpdateVisibility('p1','published'), got %v", repo.updateVisibilityCalls)
	}
}

// --- Pagination pass-through (representative; clampPagination itself is
// exercised directly by pagination_test.go) ---

func TestListPublished_ClampsPagination(t *testing.T) {
	var gotPage, gotPerPage int
	repo := &mockBestiaryRepo{
		listPublishedFn: func(_ context.Context, page, perPage int) ([]Publication, int, error) {
			gotPage, gotPerPage = page, perPage
			return nil, 0, nil
		},
	}
	svc := newTestBestiaryService(repo)
	if _, err := svc.ListPublished(context.Background(), 0, 9999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPage != 1 {
		t.Errorf("expected page clamped to 1, got %d", gotPage)
	}
	if gotPerPage != maxPerPage {
		t.Errorf("expected perPage clamped to %d, got %d", maxPerPage, gotPerPage)
	}
}

// --- Rate ---

func TestRate_RejectsOutOfRangeRating(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	for _, r := range []int{0, -1, 6, 100} {
		if err := svc.Rate(context.Background(), "user-1", "p1", r, nil); err == nil {
			t.Errorf("rating %d: expected validation error", r)
		}
	}
}

// TestRate_RejectsSelfRating proves a creator cannot rate their own publication.
func TestRate_RejectsSelfRating(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "user-1"}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	err := svc.Rate(context.Background(), "user-1", "p1", 5, nil)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestRate_TruncatesLongReview(t *testing.T) {
	var gotReview *string
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1"}, nil
		},
		ratePublicationFn: func(_ context.Context, _, _ string, _ int, reviewText *string) (bool, error) {
			gotReview = reviewText
			return true, nil
		},
	}
	svc := newTestBestiaryService(repo)
	long := make([]byte, 3000)
	for i := range long {
		long[i] = 'a'
	}
	longStr := string(long)
	if err := svc.Rate(context.Background(), "user-2", "p1", 4, &longStr); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReview == nil || len(*gotReview) > 2000 {
		t.Fatalf("expected review truncated to <=2000 chars, got %d", len(*gotReview))
	}
}

func TestRate_AllowsOtherUsersRating(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1"}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	if err := svc.Rate(context.Background(), "rater-1", "p1", 3, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- ToggleFavorite ---

func TestToggleFavorite_VerifiesPublicationExistsFirst(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return nil, apperror.NewNotFound("publication not found")
		},
		toggleFavoriteWithCountFn: func(_ context.Context, _, _ string) (bool, error) {
			t.Fatal("ToggleFavoriteWithCount must not be called when the publication doesn't exist")
			return false, nil
		},
	}
	svc := newTestBestiaryService(repo)
	_, err := svc.ToggleFavorite(context.Background(), "user-1", "missing")
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestToggleFavorite_Success(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1"}, nil
		},
		toggleFavoriteWithCountFn: func(_ context.Context, _, _ string) (bool, error) {
			return true, nil
		},
	}
	svc := newTestBestiaryService(repo)
	favorited, err := svc.ToggleFavorite(context.Background(), "user-1", "p1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !favorited {
		t.Error("expected favorited=true")
	}
}

// --- Import / Fork ---

func TestImport_RequiresEntityCreatorAndRoleChecker(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	_, err := svc.Import(context.Background(), "user-1", "p1", "camp-1")
	if err == nil {
		t.Fatal("expected internal error when EntityCreator/CampaignRoleChecker are not wired")
	}
}

func TestImport_RejectsInsufficientRole(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	svc.SetEntityCreator(&mockEntityCreator{})
	svc.SetCampaignRoleChecker(&mockRoleChecker{
		hasMinRoleFn: func(_ context.Context, _, _ string, _ int) (bool, error) { return false, nil },
	})
	_, err := svc.Import(context.Background(), "user-1", "p1", "camp-1")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

// TestImport_RejectsUnavailableVisibility proves draft/archived/flagged
// publications cannot be imported — only published/unlisted are eligible.
func TestImport_RejectsUnavailableVisibility(t *testing.T) {
	for _, vis := range []string{VisibilityDraft, VisibilityArchived, VisibilityFlagged} {
		t.Run(vis, func(t *testing.T) {
			repo := &mockBestiaryRepo{
				getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
					return &Publication{ID: "p1", Visibility: vis, StatblockJSON: validStatblock("Goblin")}, nil
				},
			}
			svc := newTestBestiaryService(repo)
			svc.SetEntityCreator(&mockEntityCreator{})
			svc.SetCampaignRoleChecker(&mockRoleChecker{})
			_, err := svc.Import(context.Background(), "user-1", "p1", "camp-1")
			var appErr *apperror.AppError
			if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
				t.Fatalf("expected forbidden error for visibility %q, got %v", vis, err)
			}
		})
	}
}

// TestImport_RejectsDuplicate proves a second import of the same publication
// into the same campaign is a conflict, not a silent duplicate entity.
func TestImport_RejectsDuplicate(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", Visibility: VisibilityPublished, StatblockJSON: validStatblock("Goblin")}, nil
		},
		importExistsFn: func(_ context.Context, _, _ string) (bool, error) { return true, nil },
	}
	svc := newTestBestiaryService(repo)
	svc.SetEntityCreator(&mockEntityCreator{})
	svc.SetCampaignRoleChecker(&mockRoleChecker{})
	_, err := svc.Import(context.Background(), "user-1", "p1", "camp-1")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "conflict" {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

// TestImport_Success proves a valid import creates an entity, records the
// import, and increments the download counter — the entity name carries no
// "(Fork)" suffix for a plain import.
func TestImport_Success(t *testing.T) {
	var recordedImport *Import
	var createdName string
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", Name: "Goblin", Visibility: VisibilityPublished, StatblockJSON: validStatblock("Goblin")}, nil
		},
		createImportFn: func(_ context.Context, imp *Import) error {
			recordedImport = imp
			return nil
		},
	}
	svc := newTestBestiaryService(repo)
	svc.SetEntityCreator(&mockEntityCreator{
		createFn: func(_ context.Context, _, _, name string, _ json.RawMessage) (string, error) {
			createdName = name
			return "entity-42", nil
		},
	})
	svc.SetCampaignRoleChecker(&mockRoleChecker{})

	result, err := svc.Import(context.Background(), "user-1", "p1", "camp-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdName != "Goblin" {
		t.Errorf("expected entity name 'Goblin' (no Fork suffix), got %q", createdName)
	}
	if result.EntityID != "entity-42" {
		t.Errorf("expected entity id propagated, got %q", result.EntityID)
	}
	if recordedImport == nil || recordedImport.EntityID == nil || *recordedImport.EntityID != "entity-42" {
		t.Fatal("expected CreateImport called with the created entity id")
	}
	if repo.incrementDownloadsCalls != 1 {
		t.Errorf("expected download counter incremented once, got %d", repo.incrementDownloadsCalls)
	}
}

// TestFork_AppendsForkSuffixAndSkipsDuplicateCheck proves Fork names the new
// entity "(Fork)" and always creates a new entity, even if the same
// publication was already imported into this campaign.
func TestFork_AppendsForkSuffixAndSkipsDuplicateCheck(t *testing.T) {
	var createdName string
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", Name: "Goblin", Visibility: VisibilityPublished, StatblockJSON: validStatblock("Goblin")}, nil
		},
		importExistsFn: func(_ context.Context, _, _ string) (bool, error) {
			t.Fatal("Fork must not check for a duplicate import")
			return true, nil
		},
	}
	svc := newTestBestiaryService(repo)
	svc.SetEntityCreator(&mockEntityCreator{
		createFn: func(_ context.Context, _, _, name string, _ json.RawMessage) (string, error) {
			createdName = name
			return "entity-99", nil
		},
	})
	svc.SetCampaignRoleChecker(&mockRoleChecker{})

	if _, err := svc.Fork(context.Background(), "user-1", "p1", "camp-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if createdName != "Goblin (Fork)" {
		t.Errorf("expected entity name 'Goblin (Fork)', got %q", createdName)
	}
}

// --- Flag ---

func TestFlag_RejectsSelfFlag(t *testing.T) {
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "user-1"}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	err := svc.Flag(context.Background(), "user-1", "p1", nil)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "forbidden" {
		t.Fatalf("expected forbidden error, got %v", err)
	}
}

func TestFlag_PassesThresholdToRepo(t *testing.T) {
	var gotThreshold int
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1", CreatorID: "owner-1"}, nil
		},
		flagPublicationAtomicFn: func(_ context.Context, _, _ string, _ *string, threshold int) error {
			gotThreshold = threshold
			return nil
		},
	}
	svc := newTestBestiaryService(repo)
	if err := svc.Flag(context.Background(), "flagger-1", "p1", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotThreshold != flagThreshold {
		t.Errorf("expected threshold %d, got %d", flagThreshold, gotThreshold)
	}
}

// --- Moderate ---

func TestModerate_RejectsInvalidAction(t *testing.T) {
	svc := newTestBestiaryService(&mockBestiaryRepo{})
	err := svc.Moderate(context.Background(), "mod-1", "p1", "delete-forever", nil)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Type != "validation_error" {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestModerate_ActionsMapToVisibility(t *testing.T) {
	cases := []struct {
		action      string
		wantVisible string
	}{
		{"approve", VisibilityPublished},
		{"archive", VisibilityArchived},
		{"restore", VisibilityPublished},
	}
	for _, tc := range cases {
		t.Run(tc.action, func(t *testing.T) {
			repo := &mockBestiaryRepo{
				getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
					return &Publication{ID: "p1"}, nil
				},
			}
			svc := newTestBestiaryService(repo)
			if err := svc.Moderate(context.Background(), "mod-1", "p1", tc.action, nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.updateVisibilityCalls) != 1 || repo.updateVisibilityCalls[0] != "p1:"+tc.wantVisible {
				t.Errorf("expected UpdateVisibility('p1', %q), got %v", tc.wantVisible, repo.updateVisibilityCalls)
			}
		})
	}
}

func TestModerate_LogsTheAction(t *testing.T) {
	var logged *ModerationLogEntry
	repo := &mockBestiaryRepo{
		getByIDFn: func(_ context.Context, _ string) (*Publication, error) {
			return &Publication{ID: "p1"}, nil
		},
		createModerationEntryFn: func(_ context.Context, entry *ModerationLogEntry) error {
			logged = entry
			return nil
		},
	}
	svc := newTestBestiaryService(repo)
	reason := "spam"
	if err := svc.Moderate(context.Background(), "mod-1", "p1", "archive", &reason); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if logged == nil {
		t.Fatal("expected a moderation log entry to be created")
	}
	if logged.ModeratorID != "mod-1" || logged.Action != "archive" || logged.Reason == nil || *logged.Reason != "spam" {
		t.Errorf("unexpected log entry: %+v", logged)
	}
}

// --- GetCreatorProfile ---

func TestGetCreatorProfile_EnrichesWithUserInfoWhenWired(t *testing.T) {
	repo := &mockBestiaryRepo{
		getCreatorStatsFn: func(_ context.Context, _ string) (*CreatorStats, error) {
			return &CreatorStats{PublicationCount: 3}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	svc.SetUserFetcher(&mockUserFetcher{
		infoFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{DisplayName: "Alice", AvatarURL: "avatar.png"}, nil
		},
	})
	profile, err := svc.GetCreatorProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.DisplayName != "Alice" || profile.AvatarURL != "avatar.png" {
		t.Errorf("expected enriched profile, got %+v", profile)
	}
	if profile.Stats.PublicationCount != 3 {
		t.Errorf("expected stats propagated, got %+v", profile.Stats)
	}
}

// TestGetCreatorProfile_WorksWithoutUserFetcher proves the profile still
// returns (with stats but no display name) when UserFetcher isn't wired.
func TestGetCreatorProfile_WorksWithoutUserFetcher(t *testing.T) {
	repo := &mockBestiaryRepo{
		getCreatorStatsFn: func(_ context.Context, _ string) (*CreatorStats, error) {
			return &CreatorStats{PublicationCount: 1}, nil
		},
	}
	svc := newTestBestiaryService(repo)
	profile, err := svc.GetCreatorProfile(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.DisplayName != "" {
		t.Errorf("expected no display name without a UserFetcher, got %q", profile.DisplayName)
	}
}
