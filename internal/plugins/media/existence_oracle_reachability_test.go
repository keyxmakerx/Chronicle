// existence_oracle_reachability_test.go pins that an anonymous,
// unauthenticated caller cannot tell "this id exists but is private" from
// "no such id" via status code alone, for a real GET /media/:id request
// (with a signer configured, as production always has). It drives the
// real, exported Handler.Serve rather than checkMediaAccess directly, so a
// failing assertion is proof the oracle is reachable in the shipped
// configuration.
package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// --- End-to-end Serve() check for the public-campaign carve-out (ADR-058
// decision 7), complementing the checkMediaAccess-level coverage in
// viewer_binding_test.go's TestCheckMediaAccess_AnonymousPublicCampaign_DmOnlyPage_Denied
// — this drives the real exported Handler.Serve instead.

// idKeyedMediaService is a MediaService double whose GetByID resolves from
// a small map and falls back to the same 404 the real service returns for
// an unmapped id. Everything else is inherited from fakeAccessMediaService.
type idKeyedMediaService struct {
	fakeAccessMediaService
	files map[string]*MediaFile
}

func (f *idKeyedMediaService) GetByID(ctx context.Context, id string) (*MediaFile, error) {
	if mf, ok := f.files[id]; ok {
		return mf, nil
	}
	return nil, apperror.NewNotFound("media file not found")
}

// serveRequestAnonymous drives Handler.Serve for a completely anonymous
// caller (no session cookie, no query string) and returns the
// *apperror.AppError Serve hands back. AppError.Code is the exact HTTP
// status a real client would see (error_handler_api_type_test.go).
func serveRequestAnonymous(t *testing.T, h *Handler, id string) *apperror.AppError {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/media/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: id}})

	err := h.Serve(c)
	if err == nil {
		t.Fatalf("Serve(%q) with no session and no signature: expected a denial, got nil (would have served the file)", id)
	}
	appErr, ok := err.(*apperror.AppError)
	if !ok {
		t.Fatalf("Serve(%q): expected *apperror.AppError, got %T: %v", id, err, err)
	}
	return appErr
}

// TestServe_ExistenceOracle_PrivateCampaign_AnonymousCaller wires a real
// signer (matching production) and compares the status Serve returns, for
// an anonymous unsigned request, between a real file in a private campaign
// and an id that does not exist. The two must be indistinguishable.
func TestServe_ExistenceOracle_PrivateCampaign_AnonymousCaller(t *testing.T) {
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &idKeyedMediaService{
			files: map[string]*MediaFile{
				"file-exists-private": privateMediaFile(),
			},
		},
		entityVisibility: &fakeEntityVisibilityFilter{},
	}

	knownPrivate := serveRequestAnonymous(t, h, "file-exists-private")
	unknown := serveRequestAnonymous(t, h, "totally-nonexistent-id")

	t.Logf("known private file, unsigned, anonymous -> %d %s (%q)",
		knownPrivate.Code, knownPrivate.Type, knownPrivate.Message)
	t.Logf("unknown id,         unsigned, anonymous -> %d %s (%q)",
		unknown.Code, unknown.Type, unknown.Message)

	if knownPrivate.Code != unknown.Code {
		t.Errorf("existence oracle reachable: a private campaign's real file answers %d %q while an "+
			"unknown id answers %d %q for the identical anonymous, unsigned request shape. An "+
			"unauthenticated caller can distinguish \"this id exists in a private campaign\" from "+
			"\"no such id anywhere\" with one request. Root cause: checkMediaAccess's "+
			"`if !h.allowUnsignedAccess(c, file) { return apperror.NewForbidden(\"signed URL "+
			"required\") }` (handler.go, inside `if h.signer != nil`) fires and returns BEFORE the "+
			"defense-in-depth branch further down that would otherwise answer not-found for an "+
			"anonymous caller on a private campaign — that branch is unreached here because "+
			"h.signer != nil, which is always true in production.",
			knownPrivate.Code, knownPrivate.Type, unknown.Code, unknown.Type)
	}
}

// TestServe_PublicCampaign_DmOnlyArtwork_AnonymousCaller_Denied drives the
// real Handler.Serve entrypoint (not checkMediaAccess directly): a public
// campaign's file referenced only by a dm_only entity must deny a fully
// anonymous, unsigned request (ADR-058 decision 7).
func TestServe_PublicCampaign_DmOnlyArtwork_AnonymousCaller_Denied(t *testing.T) {
	campaignID := "camp-public-e2e"
	dmOnlyArt := &MediaFile{
		ID:               "file-dm-only-art",
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(true),
	}
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &idKeyedMediaService{
			fakeAccessMediaService: fakeAccessMediaService{
				findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
					return []MediaRef{{EntityID: "ent-dm-only", EntityName: "Secret Villain", RefType: "image"}}, nil
				},
			},
			files: map[string]*MediaFile{"file-dm-only-art": dmOnlyArt},
		},
		// No entity is viewable to an anonymous (RoleNone) visitor.
		entityVisibility: &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}},
	}

	// serveRequestAnonymous itself fails the test (via t.Fatalf) if Serve
	// returns nil, i.e. if the file were actually served — that is the
	// primary assertion. What remains is confirming the denial is a real
	// client error, not an accidental 5xx masking the question.
	got := serveRequestAnonymous(t, h, "file-dm-only-art")
	t.Logf("public campaign, dm-only-referenced file, unsigned, anonymous -> %d %s (%q)",
		got.Code, got.Type, got.Message)

	if got.Code < http.StatusBadRequest || got.Code >= http.StatusInternalServerError {
		t.Errorf("expected a 4xx denial (403 or 404), got %d %s", got.Code, got.Type)
	}
}
