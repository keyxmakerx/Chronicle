// media_owner_gate_test.go pins that DELETE /api/v1/campaigns/:id/media/:mediaID
// requires Owner, matching its web twin (media/routes.go: DELETE
// /campaigns/:id/media/:mid requires RoleOwner — "The media browser
// (browse/delete) is Owner-only" per that file's doc comment).
// RequirePermission(PermWrite) alone — the only gate before this fix —
// also admits a Scribe, whether via a Foundry Bearer key or the Scribe's
// own session cookie (this route group's RequireAuthOrAPIKey accepts
// both), so a Scribe could delete media the web UI itself refuses them.
package syncapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"context"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// stubMediaSvcOwnerGate embeds media.MediaService; only GetByID and Delete
// are reachable from DeleteMedia. Delete always succeeds so a pre-fix test
// call would reach (and would perform) the delete, proving the role floor —
// not a downstream failure — is what blocks it.
type stubMediaSvcOwnerGate struct {
	media.MediaService
	f            *media.MediaFile
	deleteCalled bool
}

func (s *stubMediaSvcOwnerGate) GetByID(context.Context, string) (*media.MediaFile, error) {
	return s.f, nil
}

func (s *stubMediaSvcOwnerGate) Delete(context.Context, string) error {
	s.deleteCalled = true
	return nil
}

// newMediaAPIContext builds an Echo context for DELETE
// /api/v1/campaigns/:id/media/:mediaID with :id and :mediaID bound.
func newMediaAPIContext(key *APIKey) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/camp-1/media/med-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "mediaID", Value: "med-1"}})
	c.Set(apiKeyContextKey, key)
	return c, rec
}

// TestDeleteMedia_OwnerOnly pins that a Scribe-scoped caller (Bearer key or
// session cookie) is refused, and an Owner-scoped caller still succeeds.
func TestDeleteMedia_OwnerOnly(t *testing.T) {
	campID := "camp-1"
	file := &media.MediaFile{ID: "med-1", CampaignID: &campID}

	t.Run("Scribe session cookie rejected", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleScribe)}
		mediaSvc := &stubMediaSvcOwnerGate{f: file}
		h := NewMediaAPIHandler(&stubSyncSvcForRole{}, mediaSvc)
		h.SetCampaignService(campSvc)

		key := &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "scribe-1"}
		c, _ := newMediaAPIContext(key)

		err := h.DeleteMedia(c)
		if err == nil {
			t.Fatalf("Scribe session: want error (403), got nil")
		}
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
			t.Fatalf("Scribe session: want 403 Forbidden, got %v", err)
		}
		if mediaSvc.deleteCalled {
			t.Error("Delete must not be called when the role check refuses the request")
		}
	})

	t.Run("Owner session cookie allowed", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)}
		mediaSvc := &stubMediaSvcOwnerGate{f: file}
		h := NewMediaAPIHandler(&stubSyncSvcForRole{}, mediaSvc)
		h.SetCampaignService(campSvc)

		key := &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "owner-1"}
		c, _ := newMediaAPIContext(key)

		if err := h.DeleteMedia(c); err != nil {
			t.Fatalf("Owner session: want no error, got %v", err)
		}
		if !mediaSvc.deleteCalled {
			t.Error("Delete must be called for an Owner-scoped caller")
		}
	})

	t.Run("stored Bearer key allowed (Owner-minted)", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)}
		mediaSvc := &stubMediaSvcOwnerGate{f: file}
		h := NewMediaAPIHandler(&stubSyncSvcForRole{}, mediaSvc)
		h.SetCampaignService(campSvc)

		key := &APIKey{ID: 7, CampaignID: "camp-1", UserID: "foundry-key-owner"}
		c, _ := newMediaAPIContext(key)

		if err := h.DeleteMedia(c); err != nil {
			t.Fatalf("stored Bearer key: want no error, got %v", err)
		}
		if !mediaSvc.deleteCalled {
			t.Error("Delete must be called for a stored Bearer key")
		}
	})
}
