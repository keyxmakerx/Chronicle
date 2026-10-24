// dm_grant_egress_test.go pins the second half of the ADR-057 asymmetry:
// visibilityRoleFor promotes only the CheckEntityAccess call in GetEntity, so
// a Co-DM (Player + DM grant) gets in, but `role` stays unpromoted for
// gm_only field stripping (FilterRestrictedFields) and inline-secret
// redaction (stripEntitySecretsForEgress) — those must still strip for them.
package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// stubEntitySvcForDmGrantEgress is stubEntitySvcForDmGrant (dm_grant_visibility
// _test.go) plus a real EntityType.Fields set, so FilterRestrictedFields has
// a gm_only field to strip.
type stubEntitySvcForDmGrantEgress struct {
	entities.EntityService
	entity *entities.Entity
	etype  *entities.EntityType
}

func (s *stubEntitySvcForDmGrantEgress) GetByID(_ context.Context, _ string) (*entities.Entity, error) {
	e := *s.entity
	return &e, nil
}

func (s *stubEntitySvcForDmGrantEgress) GetEntityTypeByID(_ context.Context, _ int) (*entities.EntityType, error) {
	et := *s.etype
	return &et, nil
}

func (s *stubEntitySvcForDmGrantEgress) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*entities.EffectivePermission, error) {
	if s.entity.IsPrivate && role < int(campaigns.RoleOwner) {
		return &entities.EffectivePermission{CanView: false}, nil
	}
	return &entities.EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

// TestGetEntity_GMFieldsAndSecretsFollowDMAccess asserts that hidden parts of
// a page reach the owner's team only, as on the web: a Co-DM (here a Player
// with DM access) reading a dm_only entity gets its gm_only field and inline
// secret, and a Scribe without DM access reading a visible one gets neither.
func TestGetEntity_GMFieldsAndSecretsFollowDMAccess(t *testing.T) {
	tests := []struct {
		name     string
		role     campaigns.Role
		granted  bool
		private  bool
		wantSeen bool
	}{
		{"co-dm reads a dm_only page whole", campaigns.RolePlayer, true, true, true},
		{"scribe reads a visible page without its hidden parts", campaigns.RoleScribe, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secretHTML := `<p>The tavern is quiet.</p><span data-secret="true">The barkeep is a spy.</span>`
			ent := &entities.Entity{
				ID: "e1", CampaignID: "camp-1", EntityTypeID: 7, IsPrivate: tt.private,
				EntryHTML: &secretHTML,
				FieldsData: map[string]any{
					"gm_notes":     "The BBEG is actually the mayor",
					"public_field": "known to all",
				},
			}
			et := &entities.EntityType{ID: 7, CampaignID: "camp-1", Fields: []entities.FieldDefinition{
				{Key: "gm_notes", GMOnly: true},
				{Key: "public_field"},
			}}

			h := NewAPIHandler(nil,
				&stubEntitySvcForDmGrantEgress{entity: ent, etype: et},
				&stubCampaignSvcForDmGrant{role: tt.role, granted: tt.granted},
				nil)

			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "entityID")
			c.SetParamValues("camp-1", "e1")
			c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "member-1", IsActive: true})

			if err := h.GetEntity(c); err != nil {
				t.Fatalf("GetEntity: got %v, want nil (200)", err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("got status %d, want 200", rec.Code)
			}

			if got := strings.Contains(rec.Body.String(), "barkeep is a spy"); got != tt.wantSeen {
				t.Errorf("inline GM secret present = %v, want %v: body=%s", got, tt.wantSeen, rec.Body)
			}
			var resp entities.Entity
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, rec.Body)
			}
			if _, ok := resp.FieldsData["gm_notes"]; ok != tt.wantSeen {
				t.Errorf("gm_only field present = %v, want %v: fields_data=%v", ok, tt.wantSeen, resp.FieldsData)
			}
			if resp.FieldsData["public_field"] != "known to all" {
				t.Errorf("non-restricted field must survive stripping: fields_data=%v", resp.FieldsData)
			}
		})
	}
}
