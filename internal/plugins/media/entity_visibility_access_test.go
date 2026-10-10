// entity_visibility_access_test.go pins ADR-058 decisions 1-3: a media file
// inherits the visibility of the entity pages that reference it, instead of
// being readable by any member of the file's campaign at any role.
//
// Every test drives the real Handler.checkMediaAccess and
// Handler.checkEntityScopedAccess, with FilterViewableEntityIDs faked
// (fakeEntityVisibilityFilter) — it does not prove the entities plugin's
// own visibility SQL, which entity_visibility_access_integration_test.go
// covers against a real MariaDB. Read the two files together.
package media

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- Fakes ---

// fakeAccessMediaService is a MediaService double whose only meaningfully
// configurable behavior is FindReferences — every access-control test
// only needs that one call. Every other method is an unused stub (the
// checkMediaAccess/checkEntityScopedAccess path under test never reaches
// them), mirroring fakeUploadService's approach in upload_authz_test.go.
type fakeAccessMediaService struct {
	findReferencesFn    func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error)
	findReferencesCalls int
}

func (f *fakeAccessMediaService) FindReferences(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
	f.findReferencesCalls++
	if f.findReferencesFn != nil {
		return f.findReferencesFn(ctx, campaignID, mediaID)
	}
	return nil, nil
}

func (f *fakeAccessMediaService) Upload(ctx context.Context, input UploadInput) (*MediaFile, error) {
	return nil, apperror.NewInternal(errors.New("unused in access tests"))
}
func (f *fakeAccessMediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	return nil, apperror.NewNotFound("media file not found")
}
func (f *fakeAccessMediaService) Delete(ctx context.Context, id string) error { return nil }
func (f *fakeAccessMediaService) FilePath(file *MediaFile) string             { return "" }
func (f *fakeAccessMediaService) ThumbnailPath(file *MediaFile, size string) string {
	return ""
}
func (f *fakeAccessMediaService) SetStorageLimiter(limiter StorageLimiter)           {}
func (f *fakeAccessMediaService) SetMemberChecker(checker MemberChecker)             {}
func (f *fakeAccessMediaService) SetEntityVisibilityFilter(v EntityVisibilityFilter) {}
func (f *fakeAccessMediaService) ListCampaignMedia(ctx context.Context, campaignID string, page, perPage int) ([]MediaFile, int, error) {
	return nil, 0, nil
}
func (f *fakeAccessMediaService) GetCampaignStats(ctx context.Context, campaignID string) (*CampaignMediaStats, error) {
	return nil, nil
}
func (f *fakeAccessMediaService) DeleteCampaignMedia(ctx context.Context, campaignID, mediaID string) error {
	return nil
}
func (f *fakeAccessMediaService) DeleteCampaignFiles(ctx context.Context, campaignID string) (int, error) {
	return 0, nil
}
func (f *fakeAccessMediaService) CleanupOrphans(ctx context.Context) (int, error) { return 0, nil }
func (f *fakeAccessMediaService) TrashFiles(ctx context.Context, batchID string, ids []string) (int, int64, error) {
	return 0, 0, nil
}
func (f *fakeAccessMediaService) RestoreTrashedFiles(ctx context.Context, batchID string) (int, error) { return 0, nil }
func (f *fakeAccessMediaService) ListTrashedFileIDs(ctx context.Context, batchID string) ([]string, error) {
	return nil, nil
}
func (f *fakeAccessMediaService) PurgeTrashedFile(ctx context.Context, batchID, fileID string) error { return nil }
func (f *fakeAccessMediaService) BackfillContentHashes(ctx context.Context, batchSize int) (int, error) {
	return 0, nil
}
func (f *fakeAccessMediaService) ValidateMediaPath() error { return nil }

// fakeEntityVisibilityFilter is an EntityVisibilityFilter double. viewableIDs
// names which of the entity IDs passed in are "visible" — every ID NOT in
// the set is filtered out, matching FilterViewableEntityIDs's real
// contract (a map of only the viewable ones, per repository.go). err, when
// set, is returned instead (used to drive the fail-closed error path).
// recordedRole/recordedUserID/callCount let tests assert exactly what the
// handler asked for.
type fakeEntityVisibilityFilter struct {
	viewableIDs map[string]bool
	err         error

	callCount      int
	lastRole       int
	lastUserID     string
	lastEntityIDs  []string
	lastCampaignID string
}

func (f *fakeEntityVisibilityFilter) FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	f.callCount++
	f.lastRole = role
	f.lastUserID = userID
	f.lastEntityIDs = entityIDs
	f.lastCampaignID = campaignID
	if f.err != nil {
		return nil, f.err
	}
	out := make(map[string]bool, len(entityIDs))
	for _, id := range entityIDs {
		if f.viewableIDs[id] {
			out[id] = true
		}
	}
	return out, nil
}

// --- Test fixtures ---

const (
	testCampaignID = "camp-adr058"
	testFileID     = "file-adr058"
	testPlayerID   = "user-player"
	testCoDMID     = "user-codm"
)

// hiddenPageFile/visiblePageFile are both PRIVATE-campaign files — the
// only branch checkEntityScopedAccess runs on. The distinction under test
// is which entities reference them, supplied per test via
// fakeAccessMediaService.findReferencesFn.
func adr058File() *MediaFile {
	campaignID := testCampaignID
	return &MediaFile{
		ID:               testFileID,
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(false),
	}
}

// newADR058TestContext builds an authenticated, unsigned request context —
// no expires/sig query params, so signatureValid is always false and
// checkMediaAccess always reaches the defense-in-depth block under test.
func newADR058TestContext(userID string) echo.Context {
	c := newAccessTestContext(nil, &auth.Session{UserID: userID})
	return c
}

// newADR058Handler wires a Handler with no signer (so checkMediaAccess
// never short-circuits on signatureValid) and the fakes under test.
func newADR058Handler(members map[string]map[string]bool, dmGranted map[string]map[string]bool, findRefs func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error), vis EntityVisibilityFilter) (*Handler, *fakeAccessMediaService) {
	svc := &fakeAccessMediaService{findReferencesFn: findRefs}
	h := &Handler{
		memberChecker:    &stubMemberChecker{members: members, dmGranted: dmGranted},
		service:          svc,
		entityVisibility: vis,
	}
	return h, svc
}

func mustDeny(t *testing.T, err error, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: want denial, got nil (access allowed)", label)
	}
	if ae, ok := err.(*apperror.AppError); !ok || apperror.SafeCode(ae) != 404 {
		t.Errorf("%s: want a 404 AppError (never leak WHY access was denied), got %T: %v", label, err, err)
	}
}

func mustAllow(t *testing.T, err error, label string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: want access allowed, got denial: %v", label, err)
	}
}

// --- Decision 1: at least one referencing entity visible ⇒ readable ---

// TestADR058_HiddenPageOnly_PlayerDenied is the core leak this ADR closes:
// a Player who is a campaign member (any role was enough pre-ADR-058) must
// no longer read an image whose ONLY referencing entity is dm_only.
// Would FAIL against the pre-ADR-058 checkMediaAccess, which never looked
// past plain membership.
func TestADR058_HiddenPageOnly_PlayerDenied(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Villain", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}} // nothing visible to this viewer
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "Player reading an image used only by a dm_only page")

	if vis.callCount != 1 {
		t.Errorf("expected FilterViewableEntityIDs to be called once, got %d", vis.callCount)
	}
	if vis.lastRole != int(campaigns.RolePlayer) {
		t.Errorf("expected role RolePlayer(%d) reaching the filter, got %d", campaigns.RolePlayer, vis.lastRole)
	}
}

// TestADR058_VisiblePage_PlayerAllowed is the mirror positive case: the
// SAME Player CAN read an image used by a page they can see.
func TestADR058_VisiblePage_PlayerAllowed(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-visible", EntityName: "Town Square", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustAllow(t, err, "Player reading an image used by a page they can see")
}

// TestADR058_SharedHiddenAndVisible_ReadableAtAll pins ADR-058 decision 1:
// "at least one referencing page is visible", not "hidden if any page is
// hidden". An image referenced by both a dm_only entity and a visible one
// must stay readable — guards against the OR being flipped to an AND.
func TestADR058_SharedHiddenAndVisible_ReadableAtAll(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{
			{EntityID: "ent-hidden", EntityName: "Secret Villain", RefType: "image"},
			{EntityID: "ent-visible", EntityName: "Town Square", RefType: "content"},
		}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustAllow(t, err, "image shared by a hidden AND a visible page")

	if len(vis.lastEntityIDs) != 2 {
		t.Errorf("expected both referencing entity ids to be sent to the filter, got %v", vis.lastEntityIDs)
	}
}

// TestADR058_CoverImage_BehavesLikeMainImage pins that a cover-image-only
// reference is treated exactly like a main-image reference, not fallen
// through to decision 3: checkMediaAccess treats any reference
// FindReferences reports as a reference regardless of ref_type. Whether
// cover_image_path itself reaches FindReferences's SQL is proven separately
// in entity_visibility_access_integration_test.go.
func TestADR058_CoverImage_BehavesLikeMainImage(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		// A cover-image match: same ref_type as a main-image match
		// (repository.go's FindReferences labels both "image").
		return []MediaRef{{EntityID: "ent-hidden-cover", EntityName: "Secret Lair", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "Player reading a cover image used only by a dm_only page")
	if vis.callCount != 1 {
		t.Errorf("cover-image reference must reach the visibility filter same as a main image; calls=%d", vis.callCount)
	}
}

// --- Co-DM promotion ---

// TestADR058_CoDM_SeesWhatDMSees drives the hidden-page fixture from
// TestADR058_HiddenPageOnly_PlayerDenied, but for a co-DM (Player role + a
// DM grant): the viewer must be promoted to Owner (VisibilityRole()'s
// formula) before FilterViewableEntityIDs is asked.
func TestADR058_CoDM_SeesWhatDMSees(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Villain", RefType: "image"}}, nil
	}
	// The fake filter grants visibility ONLY at Owner role, so a pass
	// here can only mean the promotion happened.
	vis := &fakeVisibilityAtOwnerOnly{}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testCoDMID: true}},
		map[string]map[string]bool{testCampaignID: {testCoDMID: true}}, // DM-granted
		findRefs, vis,
	)
	c := newADR058TestContext(testCoDMID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustAllow(t, err, "co-DM reading what the DM can read")
	if vis.lastRole != int(campaigns.RoleOwner) {
		t.Errorf("expected co-DM promoted to RoleOwner(%d) reaching the filter, got %d", campaigns.RoleOwner, vis.lastRole)
	}
}

// fakeVisibilityAtOwnerOnly is a minimal EntityVisibilityFilter that only
// grants visibility when role == RoleOwner, isolating the promotion
// question from viewableIDs bookkeeping.
type fakeVisibilityAtOwnerOnly struct {
	lastRole int
}

func (f *fakeVisibilityAtOwnerOnly) FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error) {
	f.lastRole = role
	out := make(map[string]bool, len(entityIDs))
	if role >= int(campaigns.RoleOwner) {
		for _, id := range entityIDs {
			out[id] = true
		}
	}
	return out, nil
}

// TestADR058_PlainPlayer_NotPromoted is the negative half of the co-DM
// test: the SAME hidden-page fixture, SAME fakeVisibilityAtOwnerOnly, but
// a Player with no DM grant. Must stay denied — proves the promotion is
// conditioned on the grant, not given to every member.
func TestADR058_PlainPlayer_NotPromoted(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Villain", RefType: "image"}}, nil
	}
	vis := &fakeVisibilityAtOwnerOnly{}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, // no DM grant
		findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "plain Player (no DM grant) must not be promoted")
}

// --- Decision 3: unreferenced files are unaffected ---

// TestADR058_Unreferenced_MembershipStillApplies confirms a file NO entity
// references (avatar, backdrop, freshly uploaded) keeps working via plain
// campaign membership, exactly as before this ADR — even for a Player,
// even though the visibility filter would deny everything if it were
// consulted (proven by vis.callCount staying 0).
func TestADR058_Unreferenced_MembershipStillApplies(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return nil, nil // no references at all
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}} // would deny everything
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustAllow(t, err, "unreferenced file (avatar/backdrop/fresh upload) via plain membership")
	if vis.callCount != 0 {
		t.Errorf("visibility filter must not even be consulted for an unreferenced file, got %d calls", vis.callCount)
	}
}

// TestADR058_Unreferenced_NonMemberStillDenied is the membership-side
// mirror: a non-member still can't read an unreferenced file either.
// Guards against a refactor that accidentally makes "no references" mean
// "public".
func TestADR058_Unreferenced_NonMemberStillDenied(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return nil, nil
	}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {"someone-else": true}},
		nil, findRefs, &fakeEntityVisibilityFilter{},
	)
	c := newADR058TestContext(testPlayerID) // authenticated, but not a member

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "non-member reading an unreferenced file")
}

// --- Fail closed ---

// TestADR058_ReferenceLookupError_Denies: FindReferences erroring must
// deny, never fall through to "unreferenced" (which would grant plain
// membership access) nor to "referenced but visible".
func TestADR058_ReferenceLookupError_Denies(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return nil, errors.New("boom: db exploded")
	}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}}, // IS a member
		nil, findRefs, &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"x": true}},
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "reference lookup error, even for a campaign member")
}

// TestADR058_VisibilityFilterError_Denies: FilterViewableEntityIDs
// erroring must also deny outright.
func TestADR058_VisibilityFilterError_Denies(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-1", EntityName: "Something", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{err: errors.New("boom: filter exploded")}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, vis,
	)
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "visibility filter error, even for a campaign member")
}

// TestADR058_NilEntityVisibilityFilter_Denies: a referenced file with NO
// filter wired at all (a wiring bug, not a runtime error) must also fail
// closed rather than silently skipping decision 1.
func TestADR058_NilEntityVisibilityFilter_Denies(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-1", EntityName: "Something", RefType: "image"}}, nil
	}
	h, _ := newADR058Handler(
		map[string]map[string]bool{testCampaignID: {testPlayerID: true}},
		nil, findRefs, nil,
	)
	h.entityVisibility = nil // simulate a missed wiring line
	c := newADR058TestContext(testPlayerID)

	err := h.checkMediaAccess(c, adr058File(), false, "")
	mustDeny(t, err, "referenced file with the visibility seam unwired")
}

// --- Caching (ADR-058 Consequences: "caching the decision per (file, viewer)") ---

// fastFailRedisOptions trims go-redis's default retry/backoff so the
// "cache unreachable" test (which closes miniredis mid-test) fails a
// connection attempt in milliseconds instead of the library's default
// multi-second backoff schedule.
func fastFailRedisOptions(addr string) *redis.Options {
	return &redis.Options{
		Addr:         addr,
		MaxRetries:   -1,
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		PoolTimeout:  50 * time.Millisecond,
	}
}

// newMiniredisHandler builds a Handler like newADR058Handler but also
// wires a real *redis.Client backed by miniredis, so cache hit/miss
// behavior is exercised for real rather than assumed.
func newMiniredisHandler(t *testing.T, findRefs func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error), vis *fakeEntityVisibilityFilter) (*Handler, *fakeAccessMediaService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(fastFailRedisOptions(mr.Addr()))
	svc := &fakeAccessMediaService{findReferencesFn: findRefs}
	h := &Handler{
		memberChecker:    &stubMemberChecker{members: map[string]map[string]bool{testCampaignID: {testPlayerID: true}}},
		service:          svc,
		entityVisibility: vis,
		cache:            rdb,
	}
	return h, svc, mr
}

// TestADR058_Cache_HitSkipsBothLookups proves a cache hit skips BOTH the
// reference lookup and the visibility filter call on the second request
// for the same (file, viewer) — the actual "lookup per image request is
// too expensive uncached" cost the ADR's caching mitigates.
func TestADR058_Cache_HitSkipsBothLookups(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-visible", EntityName: "Town Square", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	h, svc, _ := newMiniredisHandler(t, findRefs, vis)

	c1 := newADR058TestContext(testPlayerID)
	mustAllow(t, h.checkMediaAccess(c1, adr058File(), false, ""), "first request (cache miss)")
	if svc.findReferencesCalls != 1 || vis.callCount != 1 {
		t.Fatalf("first request: want 1 reference lookup + 1 filter call, got %d/%d", svc.findReferencesCalls, vis.callCount)
	}

	c2 := newADR058TestContext(testPlayerID)
	mustAllow(t, h.checkMediaAccess(c2, adr058File(), false, ""), "second request (expected cache hit)")
	if svc.findReferencesCalls != 1 || vis.callCount != 1 {
		t.Errorf("second request: cache hit should skip both lookups; reference lookups=%d filter calls=%d", svc.findReferencesCalls, vis.callCount)
	}
}

// TestADR058_Cache_DifferentViewersNotConflated: the cache is keyed by
// (file, viewer) — a denied viewer's cached "no" must not leak into an
// allowed viewer's request for the same file, and vice versa.
func TestADR058_Cache_DifferentViewersNotConflated(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-visible", EntityName: "Town Square", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(fastFailRedisOptions(mr.Addr()))
	svc := &fakeAccessMediaService{findReferencesFn: findRefs}
	h := &Handler{
		memberChecker: &stubMemberChecker{members: map[string]map[string]bool{
			testCampaignID: {testPlayerID: true, "user-outsider": false},
		}},
		service:          svc,
		entityVisibility: vis,
		cache:            rdb,
	}

	// Allowed viewer first — populates the cache with "allow" for THIS
	// viewer's key.
	mustAllow(t, h.checkMediaAccess(newADR058TestContext(testPlayerID), adr058File(), false, ""), "allowed viewer")

	// A denied viewer for the SAME file must still be denied, not read
	// back the other viewer's cached "allow".
	vis.viewableIDs = map[string]bool{} // this viewer can't see anything
	mustDeny(t, h.checkMediaAccess(newADR058TestContext("user-outsider"), adr058File(), false, ""), "different viewer, same file, must not share the cache entry")
}

// TestADR058_Cache_UnreferencedFile_NeverCached: decision 3's branch must
// not populate the cache at all — caching it would add a staleness window
// (a revoked campaign member keeping access) this ADR never asked for.
func TestADR058_Cache_UnreferencedFile_NeverCached(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return nil, nil
	}
	h, svc, mr := newMiniredisHandler(t, findRefs, &fakeEntityVisibilityFilter{})

	mustAllow(t, h.checkMediaAccess(newADR058TestContext(testPlayerID), adr058File(), false, ""), "unreferenced file")
	if svc.findReferencesCalls != 1 {
		t.Fatalf("expected exactly 1 reference lookup, got %d", svc.findReferencesCalls)
	}
	if keys := mr.Keys(); len(keys) != 0 {
		t.Errorf("decision-3 (no references) branch must not write to the cache; found keys %v", keys)
	}
}

// TestADR058_Cache_Unavailable_FailsSafe is the load-bearing caching
// guarantee: when the cache is unreachable, the rule must still apply —
// slower, never laxer. Simulated by closing miniredis before the request,
// so every Get/Set on h.cache returns a connection error.
func TestADR058_Cache_Unavailable_FailsSafe(t *testing.T) {
	findRefs := func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
		return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Villain", RefType: "image"}}, nil
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}} // deny
	h, svc, mr := newMiniredisHandler(t, findRefs, vis)
	mr.Close() // cache is now unreachable

	err := h.checkMediaAccess(newADR058TestContext(testPlayerID), adr058File(), false, "")
	mustDeny(t, err, "hidden-page image with the cache unreachable — must still deny")
	if svc.findReferencesCalls != 1 || vis.callCount != 1 {
		t.Errorf("cache being down must not skip the real lookup: reference lookups=%d filter calls=%d", svc.findReferencesCalls, vis.callCount)
	}

	// And the positive direction: a visible-page image must still be
	// ALLOWED with the cache down — proving "slower, never laxer" cuts
	// both ways (a cache outage can't turn into a denial-of-service on
	// legitimate access either).
	vis.viewableIDs = map[string]bool{"ent-hidden": true}
	err = h.checkMediaAccess(newADR058TestContext(testPlayerID), adr058File(), false, "")
	mustAllow(t, err, "visible-page image with the cache unreachable — must still allow")
}

// TestADR058_NilMemberChecker_Denies pins the unreferenced-file path's
// fail-closed default: an unwired MemberChecker must deny, not grant, every
// caller access to a file no entity references. It asserts an error, not
// merely false, since a misconfiguration must be loud in the logs rather
// than indistinguishable from an ordinary refusal.
func TestADR058_NilMemberChecker_Denies(t *testing.T) {
	h := &Handler{
		signer:           NewURLSigner("test-secret"),
		memberChecker:    nil, // the whole point
		service:          &fakeAccessMediaService{},
		entityVisibility: &fakeEntityVisibilityFilter{},
	}

	campaignID := "camp-1"
	allowed, err := h.checkEntityScopedAccess(
		context.Background(),
		&MediaFile{ID: "file-with-no-referencing-entity", CampaignID: &campaignID},
		"user-1",
	)

	if allowed {
		t.Error("an unwired member checker must never grant access to an unreferenced file; " +
			"a control that no-ops when its dependency is missing is not a control")
	}
	if err == nil {
		t.Error("a missing member checker is a misconfiguration and must surface as an error, " +
			"not as a silent ordinary refusal")
	}
}
