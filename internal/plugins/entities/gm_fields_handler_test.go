package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// stubSvcForFields embeds EntityService and overrides only the reads
// GetFieldsAPI performs.
type stubSvcForFields struct {
	EntityService
	entity *Entity
	etype  *EntityType
}

func (s *stubSvcForFields) GetByID(_ context.Context, _ string) (*Entity, error) {
	e := *s.entity
	fd := make(map[string]any, len(s.entity.FieldsData))
	for k, v := range s.entity.FieldsData {
		fd[k] = v
	}
	e.FieldsData = fd
	return &e, nil
}

func (s *stubSvcForFields) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	return s.etype, nil
}

// CheckEntityAccess: this fixture's entity is a normal viewable (default,
// non-private) entity — the test exercises GM-field VALUE stripping for
// viewers who can see the entity, so the canonical view gate always admits.
func (s *stubSvcForFields) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*EffectivePermission, error) {
	return &EffectivePermission{CanView: true}, nil
}

// TestGetFieldsAPI_StripsGMFieldsForNonGM pins the r4 second egress path: the
// entities-plugin GetFieldsAPI (consumed by the core attributes widget) must
// omit gm_only field VALUES for a non-GM session and keep them for a GM.
func TestGetFieldsAPI_StripsGMFieldsForNonGM(t *testing.T) {
	et := &EntityType{ID: 7, Fields: []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "gm_notes", Label: "GM Notes", GMOnly: true},
	}}
	ent := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 7, FieldsData: map[string]any{
		"might": 2, "gm_notes": "the-villain-is-his-father",
	}}

	cases := []struct {
		name   string
		role   campaigns.Role
		wantGM bool
	}{
		{"player strips gm_notes", campaigns.RolePlayer, false},
		{"scribe keeps gm_notes", campaigns.RoleScribe, true},
		{"owner keeps gm_notes", campaigns.RoleOwner, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{service: &stubSvcForFields{entity: ent, etype: et}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/fields", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "e1")
			// The campaign-context middleware normally sets this; set it
			// directly by its known key so GetCampaignContext resolves it.
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "c1"},
				MemberRole: tc.role,
			})

			if err := h.GetFieldsAPI(c); err != nil {
				t.Fatalf("GetFieldsAPI: %v", err)
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, rec.Body)
			}
			fd, _ := resp["fields_data"].(map[string]any)
			if _, ok := fd["might"]; !ok {
				t.Errorf("player-visible field 'might' must always be present; body=%s", rec.Body)
			}
			if _, ok := fd["gm_notes"]; ok != tc.wantGM {
				t.Errorf("gm_notes present=%v, want %v; body=%s", ok, tc.wantGM, rec.Body)
			}
		})
	}
}

// TestGetFieldsAPI_StripsOwnerOnlyFieldsForNonOwner pins C-FIELDS-OWNER-FILTER
// at the GetFieldsAPI egress: an owner_only field's VALUE is present for the
// entity's claimed owner and for a GM-tier viewer, but absent for a fellow
// player who is neither.
func TestGetFieldsAPI_StripsOwnerOnlyFieldsForNonOwner(t *testing.T) {
	owner := "player-1"
	et := &EntityType{ID: 7, Fields: []FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "backstory", Label: "Backstory", OwnerOnly: true},
	}}
	ent := &Entity{
		ID: "e1", CampaignID: "c1", EntityTypeID: 7, OwnerUserID: &owner,
		FieldsData: map[string]any{"might": 2, "backstory": "raised by wolves"},
	}

	cases := []struct {
		name          string
		role          campaigns.Role
		viewerID      string
		wantBackstory bool
	}{
		{"another player strips backstory", campaigns.RolePlayer, "player-2", false},
		{"claiming owner keeps backstory", campaigns.RolePlayer, owner, true},
		{"scribe keeps backstory regardless of ownership", campaigns.RoleScribe, "player-2", true},
		{"owner role keeps backstory regardless of ownership", campaigns.RoleOwner, "player-2", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{service: &stubSvcForFields{entity: ent, etype: et}}
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1/fields", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id", "eid")
			c.SetParamValues("c1", "e1")
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:   &campaigns.Campaign{ID: "c1"},
				MemberRole: tc.role,
			})
			auth.SetSession(c, &auth.Session{UserID: tc.viewerID})

			if err := h.GetFieldsAPI(c); err != nil {
				t.Fatalf("GetFieldsAPI: %v", err)
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, rec.Body)
			}
			fd, _ := resp["fields_data"].(map[string]any)
			if _, ok := fd["might"]; !ok {
				t.Errorf("player-visible field 'might' must always be present; body=%s", rec.Body)
			}
			if _, ok := fd["backstory"]; ok != tc.wantBackstory {
				t.Errorf("backstory present=%v, want %v; body=%s", ok, tc.wantBackstory, rec.Body)
			}
		})
	}
}
