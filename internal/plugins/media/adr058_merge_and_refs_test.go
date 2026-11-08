// adr058_merge_and_refs_test.go pins ADR-058 decisions 4 and 5.
//
// Decision 5 (the merge): mediaService.Upload's per-campaign content-hash
// dedup must merge a byte-identical upload into an existing row only when
// the uploader can already see every page the matched file is on —
// otherwise attaching their "new" file to a visible page would silently
// publish a hidden page's artwork. canMergeWithExisting (service.go)
// answers that by reusing FindReferences + the same FilterViewableEntityIDs
// seam decision 1 wired into checkMediaAccess.
//
// Decision 4 (where is this used): CampaignMediaRefs is gated inside the
// handler on the promoted VisibilityRole() (>= RoleScribe), and the
// returned list is filtered to entities the viewer may actually see.
//
// Every test drives the real mediaService.Upload, canMergeWithExisting (via
// Upload), and Handler.CampaignMediaRefs / filterViewableRefs, using the
// same fakes entity_visibility_access_test.go and service_test.go use.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// testPNGBytes returns real, decodable PNG bytes (a tiny 4x4 image) — both
// validateMagicBytes AND http.DetectContentType need genuine magic bytes,
// and sanitizeImage genuinely decodes and re-encodes them, so a canned
// byte literal would fail somewhere in that pipeline.
func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

// newMergeTestService builds a *mediaService with the ADR-058 decision 5
// machinery wired directly (newTestMediaService in service_test.go predates
// decision 5 and doesn't set these fields).
func newMergeTestService(repo *mockMediaRepo, checker MemberChecker, vis EntityVisibilityFilter, mediaPath string) *mediaService {
	return &mediaService{
		repo:             repo,
		mediaPath:        mediaPath,
		maxSize:          10 * 1024 * 1024,
		sem:              &uploadSemaphore{slots: make(map[string]int)},
		memberChecker:    checker,
		entityVisibility: vis,
	}
}

// --- Decision 5: the merge rule ---

// TestUpload_MergeSafe_UploaderSeesEveryReferencingPage is the positive
// case: every entity referencing the matched file is visible to the
// uploader, so the merge proceeds exactly as it always has, AND the
// response is annotated with decision 4's "where is this used" data
// (MatchedExisting/UsedBy) so the caller can tell the uploader.
func TestUpload_MergeSafe_UploaderSeesEveryReferencingPage(t *testing.T) {
	const existingID = "existing-safe-merge"
	createCalled := false
	repo := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, campaignID, hash string) (*MediaFile, error) {
			return &MediaFile{ID: existingID, MimeType: "image/png"}, nil
		},
		findReferencesFn: func(_ context.Context, _, mediaID string) ([]MediaRef, error) {
			if mediaID != existingID {
				t.Errorf("expected the merge check to ask about the MATCHED file %q, got %q", existingID, mediaID)
			}
			return []MediaRef{{EntityID: "ent-visible", EntityName: "Town Square", RefType: "image"}}, nil
		},
		createFn: func(_ context.Context, _ *MediaFile) error {
			createCalled = true
			return nil
		},
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	svc := newMergeTestService(repo, &stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"scribe-1": true}}}, vis, t.TempDir())

	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1",
		UploadedBy: "scribe-1",
		MimeType:   "image/png",
		FileSize:   100,
		FileBytes:  testPNGBytes(t),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != existingID {
		t.Errorf("a safe merge must return the existing file %q, got %q", existingID, got.ID)
	}
	if createCalled {
		t.Error("a safe merge must not write a new row")
	}
	if !got.MatchedExisting {
		t.Error("a safe merge must set MatchedExisting so Handler.Upload can tell the uploader")
	}
	if len(got.UsedBy) != 1 || got.UsedBy[0].EntityID != "ent-visible" {
		t.Errorf("expected UsedBy to carry the (fully visible) reference list, got %+v", got.UsedBy)
	}
}

// TestUpload_MergeRefused_SeparateRowStored is decision 5's core: the
// uploader can NOT see the one page already using this exact content, so
// the merge must be refused and a genuinely separate row written — never
// the existing (partially hidden) file's id.
func TestUpload_MergeRefused_SeparateRowStored(t *testing.T) {
	const existingID = "existing-hidden-merge"
	var createdFile *MediaFile
	repo := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, _, _ string) (*MediaFile, error) {
			return &MediaFile{ID: existingID, MimeType: "image/png"}, nil
		},
		findReferencesFn: func(_ context.Context, _, _ string) ([]MediaRef, error) {
			return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Lair", RefType: "image"}}, nil
		},
		createFn: func(_ context.Context, f *MediaFile) error {
			createdFile = f
			return nil
		},
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}} // uploader sees nothing
	svc := newMergeTestService(repo, &stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player-1": true}}}, vis, t.TempDir())

	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1",
		UploadedBy: "player-1",
		MimeType:   "image/png",
		FileSize:   100,
		FileBytes:  testPNGBytes(t),
	})

	if err != nil {
		t.Fatalf("a refused merge must still succeed as an ordinary upload, got error: %v", err)
	}
	if createdFile == nil {
		t.Fatal("a refused merge must write a genuinely separate row — Create was never called")
	}
	if got.ID == existingID {
		t.Errorf("a refused merge must not return the existing (partially hidden) file's id; got %q", got.ID)
	}
	if got.MatchedExisting {
		t.Error("a refused merge must never set MatchedExisting — that would tell the uploader a hidden match exists")
	}
	if got.UsedBy != nil {
		t.Error("a refused merge must never populate UsedBy")
	}
}

// TestUpload_MergeCheckErrors_RefusesLikeANo pins the fail-closed direction:
// an error resolving mergeability (here, the visibility filter itself
// erroring) must refuse the merge exactly like a genuine "no" — never risk
// a merge on an answer that could not actually be computed.
func TestUpload_MergeCheckErrors_RefusesLikeANo(t *testing.T) {
	const existingID = "existing-error-merge"
	var createdFile *MediaFile
	repo := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, _, _ string) (*MediaFile, error) {
			return &MediaFile{ID: existingID, MimeType: "image/png"}, nil
		},
		findReferencesFn: func(_ context.Context, _, _ string) ([]MediaRef, error) {
			return []MediaRef{{EntityID: "ent-1", EntityName: "Something", RefType: "image"}}, nil
		},
		createFn: func(_ context.Context, f *MediaFile) error {
			createdFile = f
			return nil
		},
	}
	vis := &fakeEntityVisibilityFilter{err: context.DeadlineExceeded}
	svc := newMergeTestService(repo, &stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player-1": true}}}, vis, t.TempDir())

	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1",
		UploadedBy: "player-1",
		MimeType:   "image/png",
		FileSize:   100,
		FileBytes:  testPNGBytes(t),
	})

	if err != nil {
		t.Fatalf("a merge-check error must fall through to an ordinary upload, not fail it: %v", err)
	}
	if createdFile == nil || got.ID == existingID {
		t.Error("a merge-check error must refuse the merge (separate row), same as an explicit denial")
	}
}

// TestUpload_MergeSkipped_NoReferences pins that decision 3 carries through
// to the merge rule: a match with nothing referencing it is vacuously safe
// to merge, and the visibility filter is never consulted.
func TestUpload_MergeSkipped_NoReferences(t *testing.T) {
	const existingID = "existing-unreferenced"
	createCalled := false
	repo := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, _, _ string) (*MediaFile, error) {
			return &MediaFile{ID: existingID, MimeType: "image/png"}, nil
		},
		findReferencesFn: func(_ context.Context, _, _ string) ([]MediaRef, error) {
			return nil, nil
		},
		createFn: func(_ context.Context, _ *MediaFile) error {
			createCalled = true
			return nil
		},
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}} // would deny everything if asked
	svc := newMergeTestService(repo, &stubMemberChecker{members: map[string]map[string]bool{"camp-1": {"player-1": true}}}, vis, t.TempDir())

	got, err := svc.Upload(context.Background(), UploadInput{
		CampaignID: "camp-1",
		UploadedBy: "player-1",
		MimeType:   "image/png",
		FileSize:   100,
		FileBytes:  testPNGBytes(t),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != existingID {
		t.Errorf("expected the unreferenced file to merge as before, got %q", got.ID)
	}
	if createCalled {
		t.Error("Create should not be called when an unreferenced file dedups")
	}
	if vis.callCount != 0 {
		t.Errorf("the visibility filter must not be consulted for an unreferenced match, got %d calls", vis.callCount)
	}
}

// newUploadTestContextWithFile is upload_authz_test.go's
// newUploadTestContext, parameterized on the file bytes — needed here
// because these tests run the real mediaService.Upload's full
// magic-byte/sanitize/dedup pipeline, which requires genuine image bytes.
func newUploadTestContextWithFile(t *testing.T, userID, campaignID string, fileBytes []byte) (*echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "test.png")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(fileBytes); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if campaignID != "" {
		if err := mw.WriteField("campaign_id", campaignID); err != nil {
			t.Fatalf("write campaign_id field: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/media/upload", &body)
	req.Header.Set(echo.HeaderContentType, mw.FormDataContentType())
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if userID != "" {
		auth.SetSession(c, &auth.Session{UserID: userID})
	}
	return c, rec
}

// TestUpload_MergeRefused_ResponseIndistinguishableFromOrdinaryUpload pins
// that telling an uploader "this matched a file you can't see" would let
// them fingerprint a hidden page's artwork. Drives the real Handler.Upload
// for a fresh upload and a refused-merge upload, and asserts the JSON
// response shapes are identical: same status, same top-level keys, and
// neither ever carries "deduplicated" or "used_by".
func TestUpload_MergeRefused_ResponseIndistinguishableFromOrdinaryUpload(t *testing.T) {
	pngBytes := testPNGBytes(t)
	checker := &stubMemberChecker{roles: map[string]map[string]int{"camp-1": {"uploader-1": int(campaigns.RoleScribe)}}}

	// Scenario A: an ordinary upload — genuinely no dedup match at all.
	repoNoMatch := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, _, _ string) (*MediaFile, error) { return nil, nil },
	}
	hNoMatch := &Handler{
		service:       newMergeTestService(repoNoMatch, checker, &fakeEntityVisibilityFilter{}, t.TempDir()),
		memberChecker: checker,
	}

	// Scenario B: a match exists, but the uploader can't see everything
	// already using it — the merge must be refused.
	repoRefused := &mockMediaRepo{
		findByContentHashFn: func(_ context.Context, _, _ string) (*MediaFile, error) {
			return &MediaFile{ID: "hidden-match", MimeType: "image/png"}, nil
		},
		findReferencesFn: func(_ context.Context, _, _ string) ([]MediaRef, error) {
			return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Lair", RefType: "image"}}, nil
		},
	}
	hRefused := &Handler{
		service:       newMergeTestService(repoRefused, checker, &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}}, t.TempDir()),
		memberChecker: checker,
	}

	cA, recA := newUploadTestContextWithFile(t, "uploader-1", "camp-1", pngBytes)
	if err := hNoMatch.Upload(cA); err != nil {
		t.Fatalf("ordinary upload failed: %v", err)
	}
	cB, recB := newUploadTestContextWithFile(t, "uploader-1", "camp-1", pngBytes)
	if err := hRefused.Upload(cB); err != nil {
		t.Fatalf("refused-merge upload must still succeed as an ordinary upload: %v", err)
	}

	if recA.Code != recB.Code {
		t.Errorf("status codes must match: ordinary=%d refused-merge=%d", recA.Code, recB.Code)
	}

	var mapA, mapB map[string]any
	if err := json.Unmarshal(recA.Body.Bytes(), &mapA); err != nil {
		t.Fatalf("decode ordinary-upload response: %v (body=%s)", err, recA.Body.String())
	}
	if err := json.Unmarshal(recB.Body.Bytes(), &mapB); err != nil {
		t.Fatalf("decode refused-merge response: %v (body=%s)", err, recB.Body.String())
	}

	for _, leak := range []string{"deduplicated", "used_by"} {
		if _, ok := mapB[leak]; ok {
			t.Errorf("a refused merge's response must not carry %q — that field IS the oracle ADR-058 decision 5 forbids", leak)
		}
	}

	keysA := sortedKeys(mapA)
	keysB := sortedKeys(mapB)
	if !reflect.DeepEqual(keysA, keysB) {
		t.Errorf("a refused merge's response shape must be identical to an ordinary upload's; ordinary keys=%v refused-merge keys=%v", keysA, keysB)
	}
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Decision 4: "where is this used" joins the DM-team permissions story ---

// campaignRefsTestMediaID is a fixture media id shaped like the real
// server-generated UUID production always sees (generateUUID in
// service.go) — CampaignMediaRefs now rejects a :mid that doesn't parse
// as one, so a placeholder like "file-1" would fail before ever reaching
// the fake service these tests assert against.
const campaignRefsTestMediaID = "11111111-1111-4111-8111-111111111111"

// newCampaignRefsTestContext builds an Echo GET context for
// /campaigns/:id/media/:mid/refs carrying cc as the campaign context, using
// the same "campaign_context" key campaigns/middleware.go sets.
func newCampaignRefsTestContext(cc *campaigns.CampaignContext, userID string) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/"+cc.Campaign.ID+"/media/"+campaignRefsTestMediaID+"/refs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: cc.Campaign.ID}, {Name: "mid", Value: campaignRefsTestMediaID}})
	c.Set("campaign_context", cc)
	if userID != "" {
		auth.SetSession(c, &auth.Session{UserID: userID})
	}
	return c, rec
}

// TestCampaignMediaRefs_PlayerGetsNothing pins decision 4: a Player must
// never receive the usage list at all, not merely a filtered/empty one —
// proven by asserting FindReferences is never called.
func TestCampaignMediaRefs_PlayerGetsNothing(t *testing.T) {
	findRefsCalled := false
	svc := &fakeAccessMediaService{
		findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
			findRefsCalled = true
			return []MediaRef{{EntityID: "ent-1", EntityName: "Anything", RefType: "image"}}, nil
		},
	}
	h := &Handler{service: svc, entityVisibility: &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-1": true}}}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer}
	c, _ := newCampaignRefsTestContext(cc, "player-1")

	err := h.CampaignMediaRefs(c)

	if err == nil {
		t.Fatal("expected the usage list to be refused for a Player, got nil (success)")
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok || appErr.Code != http.StatusForbidden {
		t.Errorf("expected a Forbidden *apperror.AppError, got %T: %v", err, err)
	}
	if findRefsCalled {
		t.Error("a Player must never reach FindReferences — the usage list must be absent entirely, not merely filtered to empty")
	}
}

// TestCampaignMediaRefs_ScribeOmitsPagesScribeCannotSee is decision 4's
// filtering half: a Scribe past the VisibilityRole gate must still not see
// the name of a page they specifically cannot see (a Scribe is not
// automatically the DM).
func TestCampaignMediaRefs_ScribeOmitsPagesScribeCannotSee(t *testing.T) {
	svc := &fakeAccessMediaService{
		findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
			return []MediaRef{
				{EntityID: "ent-hidden", EntityName: "Secret Lair", EntitySlug: "secret-lair", RefType: "image"},
				{EntityID: "ent-visible", EntityName: "Town Square", EntitySlug: "town-square", RefType: "content"},
			}, nil
		},
	}
	vis := &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-visible": true}}
	h := &Handler{service: svc, entityVisibility: vis}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}
	c, rec := newCampaignRefsTestContext(cc, "scribe-1")

	if err := h.CampaignMediaRefs(c); err != nil {
		t.Fatalf("expected a Scribe to reach the usage list, got error: %v", err)
	}

	body := rec.Body.String()
	if strings.Contains(body, "Secret Lair") {
		t.Errorf("a Scribe who cannot see the hidden page must not see its name in the usage list; body=%s", body)
	}
	if !strings.Contains(body, "Town Square") {
		t.Errorf("a Scribe must still see pages they CAN see; body=%s", body)
	}
	if vis.lastRole != int(campaigns.RoleScribe) {
		t.Errorf("expected the viewer's own VisibilityRole (Scribe=%d) reaching the filter, got %d", campaigns.RoleScribe, vis.lastRole)
	}
}

// TestCampaignMediaRefs_CoDM_Promoted_SeesFullList proves the gate is
// "VisibilityRole() >= RoleScribe", not a raw-MemberRole check: a co-DM
// (raw role Player + a DM grant) must see the full list.
func TestCampaignMediaRefs_CoDM_Promoted_SeesFullList(t *testing.T) {
	svc := &fakeAccessMediaService{
		findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
			return []MediaRef{{EntityID: "ent-hidden", EntityName: "Secret Lair", RefType: "image"}}, nil
		},
	}
	vis := &fakeVisibilityAtOwnerOnly{}
	h := &Handler{service: svc, entityVisibility: vis}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true}
	c, rec := newCampaignRefsTestContext(cc, "codm-1")

	if err := h.CampaignMediaRefs(c); err != nil {
		t.Fatalf("expected a co-DM (promoted VisibilityRole) to reach the usage list, got error: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "Secret Lair") {
		t.Errorf("a co-DM promoted to Owner must see everything the DM sees; body=%s", rec.Body.String())
	}
}

// TestCampaignMediaRefs_NonUUIDMediaID_Rejected pins the fix for a
// LIKE-wildcard widening: FindReferences' query trusts mediaID to be a
// server-generated UUID with no LIKE metacharacters, but :mid is the raw,
// unvalidated URL segment. A value like "%" would otherwise reach the
// LIKE arm and match any path-form reference in the campaign; it must be
// rejected before FindReferences is ever called.
func TestCampaignMediaRefs_NonUUIDMediaID_Rejected(t *testing.T) {
	findRefsCalled := false
	svc := &fakeAccessMediaService{
		findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
			findRefsCalled = true
			return nil, nil
		},
	}
	h := &Handler{service: svc, entityVisibility: &fakeEntityVisibilityFilter{}}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/media/%25/refs", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: cc.Campaign.ID}, {Name: "mid", Value: "%"}})
	c.Set("campaign_context", cc)
	auth.SetSession(c, &auth.Session{UserID: "scribe-1"})

	err := h.CampaignMediaRefs(c)

	if err == nil {
		t.Fatal("expected a non-UUID :mid to be refused, got nil (success)")
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok || appErr.Code != http.StatusNotFound {
		t.Errorf("expected a NotFound *apperror.AppError, got %T: %v", err, err)
	}
	if findRefsCalled {
		t.Error("a non-UUID :mid must be rejected before FindReferences is called, not passed through to the LIKE query")
	}
}
