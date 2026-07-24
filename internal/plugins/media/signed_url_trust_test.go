// signed_url_trust_test.go — pins the C-MEDIA-SIGNED-URL-TRUST fix
// for operator Bug #23 (Foundry maps broken via ERR_TOO_MANY_REDIRECTS).
//
// Pre-fix: checkMediaAccess required an authenticated session cookie +
// campaign membership on every private-campaign media access, even when
// a valid signed URL was present. Cross-origin <img> tags from Foundry
// can't carry Chronicle session cookies → userID empty → 404 → Echo's
// framework error handler redirected to /login → ERR_TOO_MANY_REDIRECTS.
//
// Post-fix: a valid signed URL is itself proof of authorization. The
// defense-in-depth cookie+membership check is now gated on
// `!signatureValid`. Expired and tampered signatures still fail (the
// signer rejects them, and the cookie path enforces membership for
// unsigned requests via allowUnsignedAccess).
//
// Seven cases cover the access surface to confirm the fix doesn't
// widen it beyond intent.

package media

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// stubMemberChecker is a tiny MemberChecker for tests. members[campaignID]
// holds the set of userIDs that "are" members.
type stubMemberChecker struct {
	members map[string]map[string]bool
}

func (s *stubMemberChecker) IsCampaignMember(campaignID, userID string) bool {
	if userID == "" {
		return false
	}
	return s.members[campaignID][userID]
}

// boolPtr is a one-line helper because Go requires named storage for
// the address-of operator on basic types. Used to build *bool fields.
func boolPtr(b bool) *bool { return &b }

// signMediaURL returns the (expires, sig) tuple for a fresh media
// signed URL. Wraps URLSigner.Sign and parses the query string back
// so tests don't have to thread the URL format manually.
func signMediaURL(t *testing.T, signer *URLSigner, fileID string, ttl time.Duration) (string, string) {
	t.Helper()
	full := signer.Sign(fileID, ttl)
	req, err := http.NewRequest(http.MethodGet, full, nil)
	if err != nil {
		t.Fatalf("parse signed URL: %v", err)
	}
	q := req.URL.Query()
	return q.Get("expires"), q.Get("sig")
}

// signThumbURL is the thumb-path companion to signMediaURL.
func signThumbURL(t *testing.T, signer *URLSigner, fileID, size string, ttl time.Duration) (string, string) {
	t.Helper()
	full := signer.SignThumb(fileID, size, ttl)
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
func newAccessTestContext(query map[string]string, session *auth.Session) echo.Context {
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
// for the access-control tests. Service is nil — checkMediaAccess
// doesn't touch it. Members map can be customized per test.
func newTestHandler(secret string, members map[string]map[string]bool) *Handler {
	h := &Handler{
		signer:        NewURLSigner(secret),
		memberChecker: &stubMemberChecker{members: members},
	}
	return h
}

// TestCheckMediaAccess_ValidSignedURL_NoCookie_PrivateCampaign is the
// load-bearing positive case: the operator's Foundry cross-origin
// <img> flow. Valid signed URL, no session cookie, private campaign
// → access granted. Pre-fix this returned 404 and Echo redirected to
// /login, producing ERR_TOO_MANY_REDIRECTS in Foundry.
func TestCheckMediaAccess_ValidSignedURL_NoCookie_PrivateCampaign(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	expires, sig := signMediaURL(t, h.signer, "file-1", time.Hour)
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
	expires, sig := signMediaURL(t, h.signer, "file-1", -1*time.Hour)
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
	expires, sig := signMediaURL(t, h.signer, "file-1", time.Hour)
	// Replace the last char with one guaranteed to differ — the previous
	// "flip the first char to '0' instead" fallback was itself a no-op
	// whenever the first AND last chars were both '0' (~1/256 runs, since
	// the sig varies with the expiry timestamp), making the "tampered"
	// sig identical to the real one and this test flake red in CI.
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
	expires, sig := signThumbURL(t, h.signer, "file-1", "300", time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, nil)

	if err := h.checkMediaAccess(c, privateMediaFile(), true, "300"); err != nil {
		t.Errorf("valid thumb signed URL should bypass cookie+membership for private campaigns; got %v", err)
	}
}

// TestCheckMediaAccess_PublicCampaign_NoSignature_NoCookie pins the
// pre-existing public-campaign behavior. The defense-in-depth block
// was already gated on private-only; the fix shouldn't have changed
// this path. Belt-and-braces regression test.
func TestCheckMediaAccess_PublicCampaign_NoSignature_NoCookie(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	if err := h.checkMediaAccess(c, publicMediaFile(), false, ""); err != nil {
		t.Errorf("public-campaign unsigned access must work without auth; got %v", err)
	}
}

// avoid unused-import lint when only one helper actually formats an int.
var _ = strconv.Itoa
