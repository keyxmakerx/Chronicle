// dm_grant_visibility_test.go covers ADR-057: GetEntity's CheckEntityAccess
// call must see a Co-DM's (Player + DM grant) role promoted via
// visibilityRoleFor (checking campaigns.CampaignService.IsUserDmGranted),
// mirroring VisibilityRole() — since api_handler.go is API-key-authenticated
// and has no campaigns.CampaignContext to derive that from. `role` itself
// (from resolveRole()) is left unpromoted for GM-field/secret stripping
// later in GetEntity, matching the entities-plugin GetEntry asymmetry.
package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubEntitySvcForDmGrant is a minimal EntityService stub for a single
// dm_only entity. CheckEntityAccess mirrors the real service's legacy
// default-mode rule (Scribe+ required) so the test exercises the actual
// promotion threshold, not just an assertion on which constant was passed.
type stubEntitySvcForDmGrant struct {
	entities.EntityService
	entity *entities.Entity
}

func (s *stubEntitySvcForDmGrant) GetByID(_ context.Context, _ string) (*entities.Entity, error) {
	e := *s.entity
	return &e, nil
}

// GetEntityTypeByID: GetEntity's role<Scribe branch loads the entity type to
// filter GM-only/owner-only field values. No such fields here.
func (s *stubEntitySvcForDmGrant) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id}, nil
}

func (s *stubEntitySvcForDmGrant) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*entities.EffectivePermission, error) {
	if s.entity.IsPrivate && role < int(campaigns.RoleScribe) {
		return &entities.EffectivePermission{CanView: false}, nil
	}
	return &entities.EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

// stubCampaignSvcForDmGrant reports a Player membership that IS DM-granted —
// the Co-DM shape. GetMember drives resolveRole (unpromoted: Player);
// IsUserDmGranted drives the new visibilityRoleFor promotion.
type stubCampaignSvcForDmGrant struct {
	campaigns.CampaignService
	role    campaigns.Role
	granted bool
}

func (s *stubCampaignSvcForDmGrant) GetMember(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{Role: s.role}, nil
}

func (s *stubCampaignSvcForDmGrant) IsUserDmGranted(_ context.Context, _, _ string) (bool, error) {
	return s.granted, nil
}

// TestGetEntity_CoDmCanReadDmOnlyEntity pins the sync-API half of the Co-DM
// defect: a session-authed Player with a DM grant reading a dm_only entity
// via GET /api/v1/campaigns/:id/entities/:entityID must get the entity, not
// a 404 — matching the promotion the web Show handler now applies via
// VisibilityRole(). A plain Player (no grant) on the same entity must still
// be refused, so the fix doesn't blanket-open dm_only content.
func TestGetEntity_CoDmCanReadDmOnlyEntity(t *testing.T) {
	cases := []struct {
		name    string
		granted bool
		wantOK  bool
	}{
		{"DM-granted player reads dm_only entity", true, true},
		{"ungranted player is still refused", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent := &entities.Entity{ID: "e1", CampaignID: "camp-1", IsPrivate: true}
			h := NewAPIHandler(nil,
				&stubEntitySvcForDmGrant{entity: ent},
				&stubCampaignSvcForDmGrant{role: campaigns.RolePlayer, granted: tc.granted},
				nil)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "entityID", Value: "e1"}})
			c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "codm-1", IsActive: true})

			err := h.GetEntity(c)
			if tc.wantOK {
				if err != nil {
					t.Fatalf("GetEntity: %v, want nil (200)", err)
				}
				if rec.Code != http.StatusOK {
					t.Fatalf("got status %d, want 200", rec.Code)
				}
				var resp map[string]any
				if jerr := json.Unmarshal(rec.Body.Bytes(), &resp); jerr != nil {
					t.Fatalf("decode: %v (body=%s)", jerr, rec.Body)
				}
				if resp["id"] != "e1" {
					t.Errorf("response missing the entity; body=%s", rec.Body)
				}
				return
			}
			if err == nil {
				t.Fatalf("ungranted player: GetEntity returned nil error (status %d), want a not-found error", rec.Code)
			}
		})
	}
}
