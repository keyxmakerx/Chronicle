package syncapi

import (
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// A picture that lives in notes belongs to the people who can read the note.
// The API mints links for any caller with a key (a session key included), so
// it must answer for a note picture exactly as for a file that isn't there,
// and an Owner key must not be able to delete one.
func TestMediaAPI_NotePicturesAreOutOfReach(t *testing.T) {
	campID := "camp-1"
	file := &media.MediaFile{ID: "med-1", CampaignID: &campID, UsageType: media.UsageNoteImage}
	key := &APIKey{ID: 7, CampaignID: campID, UserID: "foundry-key-owner"}
	newHandler := func(svc *stubMediaSvcOwnerGate) *MediaAPIHandler {
		h := NewMediaAPIHandler(&stubSyncSvcForRole{}, svc)
		h.SetCampaignService(&stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)})
		return h
	}

	t.Run("get", func(t *testing.T) {
		c, _ := newMediaAPIContext(key)
		err := newHandler(&stubMediaSvcOwnerGate{f: file}).GetMedia(c)
		if apperror.SafeCode(err) != http.StatusNotFound {
			t.Fatalf("want 404, got %v", err)
		}
	})
	t.Run("delete", func(t *testing.T) {
		svc := &stubMediaSvcOwnerGate{f: file}
		c, _ := newMediaAPIContext(key)
		err := newHandler(svc).DeleteMedia(c)
		if apperror.SafeCode(err) != http.StatusNotFound || svc.deleteCalled {
			t.Fatalf("want 404 and no delete, got %v (deleted=%v)", err, svc.deleteCalled)
		}
	})
	t.Run("a page picture is still served", func(t *testing.T) {
		page := &media.MediaFile{ID: "med-1", CampaignID: &campID, UsageType: media.UsageEntityImage}
		c, _ := newMediaAPIContext(key)
		if err := newHandler(&stubMediaSvcOwnerGate{f: page}).GetMedia(c); err != nil {
			t.Fatalf("page picture: %v", err)
		}
	})
}
