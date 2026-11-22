// viewer_binding_test.go pins ADR-058 decision 6 (a signed media URL is
// bound to the viewer it was minted for) and decision 7 (a public
// campaign's unsigned fallback narrows to what an anonymous viewer may
// actually see). Drives the real Handler.checkMediaAccess and
// URLSigner.Sign/Verify/SignThumb/VerifyThumb; FilterViewableEntityIDs's own
// SQL for an anonymous viewer is proven separately in
// entity_visibility_access_integration_test.go.
package media

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// --- Decision 6: a signed URL is bound to the viewer it was minted for ---

// bindingTestFile is a private-campaign file with NO referencing entity
// (decision 3's plain-membership branch), distinct from privateMediaFile's
// id/campaign so its cache entries never collide with that file's.
func bindingTestFile() *MediaFile {
	campaignID := "camp-bind"
	return &MediaFile{
		ID:               "file-bind",
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(false),
	}
}

// TestSignedURL_MintedForA_StillWorksForA is the positive half of decision
// 6: a viewer presenting their OWN signed link is unaffected by binding.
func TestSignedURL_MintedForA_StillWorksForA(t *testing.T) {
	h := newTestHandler("test-secret", map[string]map[string]bool{
		"camp-bind": {"user-a": true},
	})
	expires, sig := signMediaURL(t, h.signer, "file-bind", ViewerSession("user-a"), time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, &auth.Session{UserID: "user-a"})

	if err := accessErr(h.checkMediaAccess(c, bindingTestFile(), false, "")); err != nil {
		t.Errorf("a link minted for user-a must still work for user-a; got %v", err)
	}
}

// TestSignedURL_MintedForA_DoesNotWorkForB pins ADR-058 decision 6: user A's
// link must not work for user B. user-b is deliberately not a campaign
// member, so the only pre-decision-6 reason this would have been denied is
// absent — a copied link must be inert on its own, not a bearer token.
func TestSignedURL_MintedForA_DoesNotWorkForB(t *testing.T) {
	h := newTestHandler("test-secret", map[string]map[string]bool{
		"camp-bind": {"user-a": true}, // user-b is deliberately NOT a member
	})
	expires, sig := signMediaURL(t, h.signer, "file-bind", ViewerSession("user-a"), time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, &auth.Session{UserID: "user-b"})

	err := accessErr(h.checkMediaAccess(c, bindingTestFile(), false, ""))
	if err == nil {
		t.Error("a link minted for user-a must NOT work for user-b: a copied signed URL must be " +
			"inert for anyone else (ADR-058 decision 6)")
	}
}

// TestSignedURL_MintedForA_DoesNotWorkAnonymously confirms the same
// mismatch holds against a cookieless presentation of A's link — an
// anonymous caller cannot borrow a session-bound link either. Only the
// fixed ViewerAPIKey sentinel gets the anonymous-presentation carve-out
// (see Verify's doc comment in signed_url.go), and this link was not
// minted with it.
func TestSignedURL_MintedForA_DoesNotWorkAnonymously(t *testing.T) {
	h := newTestHandler("test-secret", map[string]map[string]bool{
		"camp-bind": {"user-a": true},
	})
	expires, sig := signMediaURL(t, h.signer, "file-bind", ViewerSession("user-a"), time.Hour)
	c := newAccessTestContext(map[string]string{"expires": expires, "sig": sig}, nil)

	err := accessErr(h.checkMediaAccess(c, bindingTestFile(), false, ""))
	if err == nil {
		t.Error("a session-bound link must not work for a cookieless (anonymous) presentation")
	}
}

// TestSignedURL_OldFormat_InvalidatedImmediately pins that a signature
// computed the pre-decision-6 way (HMAC over "fileID:expires", no viewer
// segment) must not verify under the new scheme, even though it is
// otherwise well-formed and unexpired. Reimplements the old payload by hand
// since no code path in this package produces it anymore.
func TestSignedURL_OldFormat_InvalidatedImmediately(t *testing.T) {
	secret := "test-secret"
	signer := NewURLSigner(secret)
	expires := time.Now().Add(time.Hour).Unix()

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%s:%d", "file-old", expires)
	oldFormatSig := hex.EncodeToString(mac.Sum(nil))
	expiresStr := fmt.Sprintf("%d", expires)

	for _, viewer := range []string{ViewerAnonymous, ViewerAPIKey, ViewerSession("user-a")} {
		if signer.Verify("file-old", viewer, expiresStr, oldFormatSig) {
			t.Errorf("an old-format (pre-viewer-binding) signature must not verify under the new "+
				"scheme, presented viewer = %q", viewer)
		}
	}
}

// TestSignedURLTTL_ShorterThanTheOldOneHour pins the audit's "shorten the
// TTL" item: every Sign/SignThumb call site used to hardcode a 1-hour TTL;
// SignedURLTTL must be a real, positive, meaningfully shorter reduction,
// not a cosmetic rename of the same hour.
func TestSignedURLTTL_ShorterThanTheOldOneHour(t *testing.T) {
	if SignedURLTTL <= 0 {
		t.Fatalf("SignedURLTTL = %v, must be positive", SignedURLTTL)
	}
	if SignedURLTTL >= time.Hour {
		t.Errorf("SignedURLTTL = %v, want less than the old 1h TTL", SignedURLTTL)
	}
}

// --- Decision 7: a public campaign's unsigned fallback narrows to what an
// anonymous viewer may actually see ---

// publicMediaFileReferenced is a public-campaign file referenced by exactly
// one entity ("ent-1" in each test below) — the fakeEntityVisibilityFilter's
// viewableIDs decide whether that entity, and so the file, is visible.
func publicMediaFileReferenced() *MediaFile {
	campaignID := "camp-public-refs"
	return &MediaFile{
		ID:               "file-public-refs",
		CampaignID:       &campaignID,
		CampaignIsPublic: boolPtr(true),
	}
}

// TestCheckMediaAccess_AnonymousPublicCampaign_DmOnlyPage_Denied is the
// central decision-7 case: an anonymous caller with no signature must not
// read a public campaign's file used only by a dm_only page.
func TestCheckMediaAccess_AnonymousPublicCampaign_DmOnlyPage_Denied(t *testing.T) {
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &fakeAccessMediaService{
			findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
				return []MediaRef{{EntityID: "ent-dm-only", EntityName: "Secret Villain", RefType: "image"}}, nil
			},
		},
		// An anonymous (RoleNone, "") viewer cannot see this entity — the
		// fake simply excludes it from viewableIDs, exactly as
		// FilterViewableEntityIDs would for a dm_only page.
		entityVisibility: &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{}},
	}
	c := newAccessTestContext(nil, nil) // no signature, no cookie: true anonymous

	err := accessErr(h.checkMediaAccess(c, publicMediaFileReferenced(), false, ""))
	if err == nil {
		t.Error("an anonymous caller must not read a public campaign's file used only by a " +
			"dm_only page (ADR-058 decision 7)")
	}
}

// TestCheckMediaAccess_AnonymousPublicCampaign_VisiblePage_Allowed is the
// companion positive case: the SAME public campaign and the same anonymous,
// unsigned request shape, but the file is used by a page an anonymous
// viewer may see. Decision 7 must narrow access, not remove it.
func TestCheckMediaAccess_AnonymousPublicCampaign_VisiblePage_Allowed(t *testing.T) {
	h := &Handler{
		signer:        NewURLSigner("test-secret"),
		memberChecker: &stubMemberChecker{},
		service: &fakeAccessMediaService{
			findReferencesFn: func(ctx context.Context, campaignID, mediaID string) ([]MediaRef, error) {
				return []MediaRef{{EntityID: "ent-public", EntityName: "Town Square", RefType: "image"}}, nil
			},
		},
		entityVisibility: &fakeEntityVisibilityFilter{viewableIDs: map[string]bool{"ent-public": true}},
	}
	c := newAccessTestContext(nil, nil)

	if err := accessErr(h.checkMediaAccess(c, publicMediaFileReferenced(), false, "")); err != nil {
		t.Errorf("an anonymous caller must be able to read a public campaign's file used by a page "+
			"anonymous viewers can see; got %v", err)
	}
}
