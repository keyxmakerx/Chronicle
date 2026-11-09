// signed_url_trust_test.go pins that a valid signed URL is itself proof of
// authorization for checkMediaAccess: the cookie+membership defense-in-depth
// check only runs when no valid signature is present. A cross-origin <img>
// (Foundry) can't carry a session cookie, so this must hold or those
// requests get denied. Expired and tampered signatures must still fail.

package media

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// stubMemberChecker is a tiny MemberChecker for tests. members[campaignID]
// holds the set of userIDs that "are" members. roles optionally overrides
// the role MemberRole reports for a given (campaignID, userID) pair — tests
// that only care about membership (not role granularity) can leave it nil.
// dmGranted optionally marks a (campaignID, userID) pair as co-DM/dm_only
// granted — ADR-058's promotion source; tests that don't care leave it nil
// (IsUserDmGranted then reports false for everyone, same as an unwired
// campaign).
type stubMemberChecker struct {
	members   map[string]map[string]bool
	roles     map[string]map[string]int
	dmGranted map[string]map[string]bool
}

// IsUserDmGranted reports the stubbed co-DM grant for (campaignID, userID).
// Defaults to false (not granted) when dmGranted is nil or the pair is
// absent — mirrors mediaMemberCheckerAdapter's fail-to-false-on-error
// posture in production.
func (s *stubMemberChecker) IsUserDmGranted(campaignID, userID string) bool {
	return s.dmGranted[campaignID][userID]
}

func (s *stubMemberChecker) IsCampaignMember(campaignID, userID string) bool {
	if userID == "" {
		return false
	}
	return s.members[campaignID][userID]
}

// MemberRole returns the stubbed role for (campaignID, userID). Falls back
// to RolePlayer (1) for a plain member (a test only populated `members`)
// and RoleNone (0) otherwise — the roles map is for tests that need a
// specific tier (e.g. Scribe vs Player) rather than a bare yes/no.
func (s *stubMemberChecker) MemberRole(campaignID, userID string) int {
	if userID == "" {
		return 0
	}
	if r, ok := s.roles[campaignID][userID]; ok {
		return r
	}
	if s.members[campaignID][userID] {
		return 1 // RolePlayer
	}
	return 0 // RoleNone
}

// boolPtr is a one-line helper because Go requires named storage for
// the address-of operator on basic types. Used to build *bool fields.
func boolPtr(b bool) *bool { return &b }

// signMediaURL returns the (expires, sig) tuple for a fresh media
// signed URL minted for viewer (ADR-058 decision 6). Wraps URLSigner.Sign
// and parses the query string back so tests don't have to thread the URL
// format manually.
func signMediaURL(t *testing.T, signer *URLSigner, fileID, viewer string, ttl time.Duration) (string, string) {
	t.Helper()
	full := signer.Sign(fileID, viewer, ttl)
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		t.Fatalf("parse signed URL: %v", err)
	}
	q := req.URL.Query()
	return q.Get("expires"), q.Get("sig")
}

// signThumbURL is the thumb-path companion to signMediaURL.
func signThumbURL(t *testing.T, signer *URLSigner, fileID, size, viewer string, ttl time.Duration) (string, string) {
	t.Helper()
	full := signer.SignThumb(fileID, size, viewer, ttl)
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		t.Fatalf("parse thumb signed URL: %v", err)
	}
	q := req.URL.Query()
	return q.Get("expires"), q.Get("sig")
}

// newAccessTestContext builds an Echo context whose request URL carries
// the given query params. Tests then call h.checkMediaAccess against
// it. Optional session is set via auth.SetSession so we exercise the
// real auth helpers (no shadow keys).
func newAccessTestContext(query map[string]string, session *auth.Session) *echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/media/x", nil)
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if session != nil {
		auth.SetSession(c, session)
	}
	return c
}

// privateMediaFile returns a MediaFile attached to a private campaign,
// the trigger for the defense-in-depth block.
func privateMediaFile() *MediaFile {
	campaignID := "camp-1"
	return &MediaFile{
		ID:               "file-1",
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(false),
	}
}

// publicMediaFile returns a MediaFile attached to a public campaign,
// the lenient access path.
func publicMediaFile() *MediaFile {
	campaignID := "camp-public"
	return &MediaFile{
		ID:               "file-2",
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(true),
	}
}

// newTestHandler returns a Handler wired with a signer + member checker
// for the access-control tests. Service is a no-references stub (ADR-058:
// checkMediaAccess now always asks for a file's references before falling
// back to plain membership, so every one of these pre-existing tests —
// which are all decision-3 "no owning entity" scenarios — needs a
// FindReferences that returns empty rather than nil, exactly like an
// avatar or backdrop would). Members map can be customized per test.
func newTestHandler(secret string, members map[string]map[string]bool) *Handler {
	h := &Handler{
		signer:           NewURLSigner(secret),
		memberChecker:    &stubMemberChecker{members: members},
		service:          &fakeAccessMediaService{},
		entityVisibility: &fakeEntityVisibilityFilter{},
	}
	return h
}

// TestCheckMediaAccess_ValidSignedURL_NoCookie_PrivateCampaign pins the
// Foundry cross-origin <img> flow: a valid signed URL with no session
// cookie must grant access to private-campaign media. Every signed URL is
// viewer-bound (ADR-058 decision 6) and a cookieless request presents as
// ViewerAnonymous, so this only passes via Verify's ViewerAPIKey carve-out —
// the link must be minted with media.ViewerAPIKey, matching production.
func TestCheckMediaAccess_ValidSignedURL_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	expires, sig := signMediaURL(t, h.signer, "file-1", ViewerAPIKey, time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, nil)

	if err := h.checkMediaAccess(c, privateMediaFile(), false, ""); err != nil {
		t.Errorf("valid signed URL should bypass cookie+membership for private campaigns; got %v. C-MEDIA-SIGNED-URL-TRUST regressed.", err)
	}
}

// TestCheckMediaAccess_ExpiredSignature_NoCookie_PrivateCampaign pins
// that expiry still rejects. Without this guarantee, the fix would
// turn a signed URL into a permanent credential.
func TestCheckMediaAccess_ExpiredSignature_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	// Sign with a -1h TTL → expired before the verifier sees it.
	expires, sig := signMediaURL(t, h.signer, "file-1", ViewerAPIKey, -1*time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, nil)

	err := h.checkMediaAccess(c, privateMediaFile(), false, "")
	if err == nil {
		t.Errorf("expired signature must be rejected; got nil")
	}
}

// TestCheckMediaAccess_TamperedSignature_NoCookie_PrivateCampaign pins
// that signature forgery still rejects. Flips one byte of the sig.
func TestCheckMediaAccess_TamperedSignature_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	expires, sig := signMediaURL(t, h.signer, "file-1", ViewerAPIKey, time.Hour)
	// Replace the last char with one guaranteed to differ, so the
	// "tampered" sig can never collide with the real one.
	repl := "0"
	if sig[len(sig)-1] == '0' {
		repl = "1"
	}
	tampered := sig[:len(sig)-1] + repl
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": tampered}, nil)

	err := h.checkMediaAccess(c, privateMediaFile(), false, "")
	if err == nil {
		t.Errorf("tampered signature must be rejected; got nil")
	}
}

// TestCheckMediaAccess_NoSignature_NoCookie_PrivateCampaign pins that
// the allowUnsignedAccess fallback path still requires cookie+
// membership for private campaigns. The fix does NOT remove the
// gating — it only carves out the valid-signature shortcut.
func TestCheckMediaAccess_NoSignature_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	err := h.checkMediaAccess(c, privateMediaFile(), false, "")
	if err == nil {
		t.Errorf("unsigned access to private campaign without cookie must be rejected; got nil")
	}
}

// TestCheckMediaAccess_NoSignature_Cookie_NonMember_PrivateCampaign
// pins membership-enforcement on the cookie-auth path: a logged-in
// user who isn't a member of the campaign still can't reach private
// media without a valid signed URL.
func TestCheckMediaAccess_NoSignature_Cookie_NonMember_PrivateCampaign(t *testing.T) {
	// Member map has user-x belonging to a DIFFERENT campaign — they
	// are authenticated but not a member of camp-1.
	h := newTestHandler("test-secret", map[string]map[string]bool{
		"camp-other": {"user-x": true},
	})
	session := &auth.Session{UserID: "user-x"}
	c := newAccessTestContext(nil, session)

	err := h.checkMediaAccess(c, privateMediaFile(), false, "")
	if err == nil {
		t.Errorf("non-member cookie auth on private campaign must be rejected; got nil")
	}
	// Echo's framework error handler will render 404 → /login redirect
	// for non-AppErrors. Confirm we returned a typed AppError so the
	// safety net isn't load-bearing.
	if _, ok := err.(*apperror.AppError); !ok {
		t.Errorf("expected AppError, got %T", err)
	}
}

// TestCheckMediaAccess_ValidThumbSignedURL_NoCookie_PrivateCampaign is
// the mirror of the positive case for the thumb path. Thumbs include
// size in the signature; the verifier checks the same way.
func TestCheckMediaAccess_ValidThumbSignedURL_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	expires, sig := signThumbURL(t, h.signer, "file-1", "300", ViewerAPIKey, time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, nil)

	if err := h.checkMediaAccess(c, privateMediaFile(), true, "300"); err != nil {
		t.Errorf("valid thumb signed URL should bypass cookie+membership for private campaigns; got %v", err)
	}
}

// TestCheckMediaAccess_PublicCampaign_NoSignature_NoCookie pins ADR-058
// decision 7: a public campaign no longer grants unsigned access to every
// file unconditionally. An anonymous, unsigned request for an unreferenced
// public-campaign file (decision 3's membership question) must be denied. A
// real page load still sees the file via a freshly-minted signed URL
// (decision 6); this exercises only the legacy bare-URL fallback. See
// TestCheckMediaAccess_AnonymousPublicCampaign_DmOnlyPage_Denied and
// TestCheckMediaAccess_AnonymousPublicCampaign_VisiblePage_Allowed
// (viewer_binding_test.go) for decision 7's has-references cases.
func TestCheckMediaAccess_PublicCampaign_NoSignature_NoCookie(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	err := h.checkMediaAccess(c, publicMediaFile(), false, "")
	if err == nil {
		t.Error("ADR-058 decision 7: an anonymous, unsigned request for an unreferenced public-campaign " +
			"file with no membership must now be denied, not granted unconditionally")
	}
}

// avoid unused-import lint when only one helper actually formats an int.
var _ = strconv.Itoa
