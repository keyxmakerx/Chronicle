// orphaned_media_access_test.go pins that checkMediaAccess only treats a
// nil-campaign file as public when it was uploaded that way on purpose
// (avatar/backdrop). A campaign-scoped file (attachment/entity_image)
// whose campaign_id has been nulled — by a partial or unwired cleanup on
// campaign delete, an import, or a restored backup — must deny like any
// other unknown file, not fall through to "public".
//
// An avatar is a narrower case than a backdrop: it is public to any
// signed-in user but denied to an anonymous visitor (#730 decision 2).

package media

import (
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// nilCampaignMediaFile returns a MediaFile with no campaign, tagged with
// the given usage type — the shape both an intentional avatar/backdrop
// upload and an orphaned attachment/entity_image row share.
func nilCampaignMediaFile(usageType string) *MediaFile {
	return &MediaFile{
		ID:        "file-orphan",
		UsageType: usageType,
	}
}

func TestCheckMediaAccess_NilCampaign_Avatar_SignedIn_Allowed(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, &auth.Session{UserID: "user-1"})

	if err := h.checkMediaAccess(c, nilCampaignMediaFile(UsageAvatar), false, ""); err != nil {
		t.Errorf("a signed-in viewer must be able to see a profile picture; got %v", err)
	}
}

func TestCheckMediaAccess_NilCampaign_Avatar_Anonymous_Denied(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	err := h.checkMediaAccess(c, nilCampaignMediaFile(UsageAvatar), false, "")
	mustDeny(t, err, "an anonymous visitor must not see a profile picture")
}

func TestCheckMediaAccess_NilCampaign_Backdrop_Allowed(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	if err := h.checkMediaAccess(c, nilCampaignMediaFile(UsageBackdrop), false, ""); err != nil {
		t.Errorf("backdrop upload with no campaign must stay public; got %v", err)
	}
}

func TestCheckMediaAccess_NilCampaign_Attachment_Denied(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	err := h.checkMediaAccess(c, nilCampaignMediaFile(UsageAttachment), false, "")
	mustDeny(t, err, "orphaned attachment (nil campaign_id) must not become public")
}

func TestCheckMediaAccess_NilCampaign_EntityImage_Denied(t *testing.T) {
	h := newTestHandler("test-secret", nil)
	c := newAccessTestContext(nil, nil)

	err := h.checkMediaAccess(c, nilCampaignMediaFile(UsageEntityImage), false, "")
	mustDeny(t, err, "orphaned entity image (nil campaign_id) must not become public")
}
