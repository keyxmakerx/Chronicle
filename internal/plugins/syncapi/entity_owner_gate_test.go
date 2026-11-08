// entity_owner_gate_test.go pins that DELETE /api/v1/campaigns/:id/entities/:entityID
// requires Owner, matching its web twin (entities/routes.go: DELETE
// /entities/:eid requires RoleOwner — "Owner can delete" per that file's
// header comment). RequirePermission(PermWrite) alone — the only gate
// before this fix — also admits a Scribe, whether via a Foundry Bearer key
// or the Scribe's own session cookie (this route group's
// RequireAuthOrAPIKey accepts both), so a Scribe could delete entities the
// web UI itself refuses them.
package syncapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubEntitySvcOwnerGate embeds entities.EntityService; only GetByID and
// Delete are reachable from DeleteEntity. Delete always succeeds so a
// pre-fix test call would reach (and would perform) the delete, proving the
// role floor — not a downstream failure — is what blocks it.
type stubEntitySvcOwnerGate struct {
	entities.EntityService
	e            *entities.Entity
	deleteCalled bool
}

func (s *stubEntitySvcOwnerGate) GetByID(context.Context, string) (*entities.Entity, error) {
	return s.e, nil
}

func (s *stubEntitySvcOwnerGate) Delete(context.Context, string) error {
	s.deleteCalled = true
	return nil
}

// newEntityAPIContext builds an Echo context for DELETE
// /api/v1/campaigns/:id/entities/:entityID with :id and :entityID bound.
func newEntityAPIContext(key *APIKey) (*echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/campaigns/camp-1/entities/ent-1", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "entityID", Value: "ent-1"}})
	c.Set(apiKeyContextKey, key)
	return c, rec
}

// TestDeleteEntity_OwnerOnly pins that a Scribe-scoped caller (Bearer key or
// session cookie) is refused, and an Owner-scoped caller still succeeds.
func TestDeleteEntity_OwnerOnly(t *testing.T) {
	ent := &entities.Entity{ID: "ent-1", CampaignID: "camp-1"}

	t.Run("Scribe session cookie rejected", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleScribe)}
		entitySvc := &stubEntitySvcOwnerGate{e: ent}
		h := NewAPIHandler(&stubSyncSvcForRole{}, entitySvc, campSvc, nil)

		// Session-authed caller: the synthetic key ID, same auth path a
		// Scribe's own browser session uses.
		key := &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "scribe-1"}
		c, _ := newEntityAPIContext(key)

		err := h.DeleteEntity(c)
		if err == nil {
			t.Fatalf("Scribe session: want error (403), got nil")
		}
		var ae *apperror.AppError
		if !errors.As(err, &ae) || ae.Code != http.StatusForbidden {
			t.Fatalf("Scribe session: want 403 Forbidden, got %v", err)
		}
		if entitySvc.deleteCalled {
			t.Error("Delete must not be called when the role check refuses the request")
		}
	})

	t.Run("Owner session cookie allowed", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)}
		entitySvc := &stubEntitySvcOwnerGate{e: ent}
		h := NewAPIHandler(&stubSyncSvcForRole{}, entitySvc, campSvc, nil)

		key := &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "owner-1"}
		c, _ := newEntityAPIContext(key)

		if err := h.DeleteEntity(c); err != nil {
			t.Fatalf("Owner session: want no error, got %v", err)
		}
		if !entitySvc.deleteCalled {
			t.Error("Delete must be called for an Owner-scoped caller")
		}
	})

	t.Run("stored Bearer key allowed (Owner-minted)", func(t *testing.T) {
		campSvc := &stubCampaignSvcForRole{getMemberFn: memberWithRole(campaigns.RoleOwner)}
		entitySvc := &stubEntitySvcOwnerGate{e: ent}
		h := NewAPIHandler(&stubSyncSvcForRole{}, entitySvc, campSvc, nil)

		// ID != synthKeySessionID marks this as a real stored key, which
		// resolveRole always resolves to Owner.
		key := &APIKey{ID: 7, CampaignID: "camp-1", UserID: "foundry-key-owner"}
		c, _ := newEntityAPIContext(key)

		if err := h.DeleteEntity(c); err != nil {
			t.Fatalf("stored Bearer key: want no error, got %v", err)
		}
		if !entitySvc.deleteCalled {
			t.Error("Delete must be called for a stored Bearer key")
		}
	})
}
