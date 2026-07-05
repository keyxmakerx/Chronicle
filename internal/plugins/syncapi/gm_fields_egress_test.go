package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// GM-field egress tests (audit M-1 / C-FIELDS-GM-FILTER). Pin that a gm_only
// field's VALUE is stripped from fields_data for non-GM SESSION callers on
// both GetEntity and ListEntities, while GM/owner sessions and Foundry Bearer
// callers keep full data.

type stubEntityServiceForGM struct {
	entities.EntityService // embed: unimplemented methods panic if hit
	entity                 *entities.Entity
	etype                  *entities.EntityType
}

func gmCloneMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// freshEntity returns a per-call copy (like a DB scan) so a strip can't leak
// across calls by mutating a shared map.
func (s *stubEntityServiceForGM) freshEntity() entities.Entity {
	e := *s.entity
	e.FieldsData = gmCloneMap(s.entity.FieldsData)
	return e
}

func (s *stubEntityServiceForGM) GetByID(_ context.Context, _ string) (*entities.Entity, error) {
	e := s.freshEntity()
	return &e, nil
}
func (s *stubEntityServiceForGM) GetEntityTypeByID(_ context.Context, _ int) (*entities.EntityType, error) {
	return s.etype, nil
}
func (s *stubEntityServiceForGM) GetEntityTypes(_ context.Context, _ string) ([]entities.EntityType, error) {
	return []entities.EntityType{*s.etype}, nil
}
func (s *stubEntityServiceForGM) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true}, nil
}
func (s *stubEntityServiceForGM) List(_ context.Context, _ string, _ int, _ int, _ string, _ entities.ListOptions) ([]entities.Entity, int, error) {
	e := s.freshEntity()
	return []entities.Entity{e}, 1, nil
}

type stubCampaignSvcForGM struct {
	campaigns.CampaignService
	role campaigns.Role
}

func (s *stubCampaignSvcForGM) GetMember(_ context.Context, _, _ string) (*campaigns.CampaignMember, error) {
	return &campaigns.CampaignMember{Role: s.role}, nil
}

func gmTestFixtures() (*entities.Entity, *entities.EntityType) {
	et := &entities.EntityType{ID: 7, Fields: []entities.FieldDefinition{
		{Key: "might", Label: "Might"},
		{Key: "gm_notes", Label: "GM Notes", GMOnly: true},
	}}
	ent := &entities.Entity{
		ID: "e1", CampaignID: "camp-1", EntityTypeID: 7,
		FieldsData: map[string]any{"might": 2, "gm_notes": "the villain is his father"},
	}
	return ent, et
}

func gmContext(method, path, entityID string, bearer bool) (echo.Context, *httptest.ResponseRecorder) {
	e := echo.New()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if entityID != "" {
		c.SetParamNames("id", "entityID")
		c.SetParamValues("camp-1", entityID)
	} else {
		c.SetParamNames("id")
		c.SetParamValues("camp-1")
	}
	keyID := synthKeySessionID // session-authed → role from GetMember
	if bearer {
		keyID = 1 // stored Bearer key → resolveRole returns Owner
	}
	c.Set(apiKeyContextKey, &APIKey{ID: keyID, CampaignID: "camp-1", UserID: "u1", IsActive: true})
	return c, rec
}

var gmViewerCases = []struct {
	name   string
	role   campaigns.Role
	bearer bool
	wantGM bool // gm_notes value present in fields_data?
}{
	{"player session strips gm_notes", campaigns.RolePlayer, false, false},
	{"scribe session keeps gm_notes", campaigns.RoleScribe, false, true},
	{"owner session keeps gm_notes", campaigns.RoleOwner, false, true},
	{"foundry bearer keeps gm_notes", campaigns.RoleOwner, true, true},
}

func TestGetEntity_StripsGMFieldsForNonGM(t *testing.T) {
	for _, tc := range gmViewerCases {
		t.Run(tc.name, func(t *testing.T) {
			ent, et := gmTestFixtures()
			h := NewAPIHandler(nil, &stubEntityServiceForGM{entity: ent, etype: et}, &stubCampaignSvcForGM{role: tc.role}, nil)
			c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1", "e1", tc.bearer)

			if err := h.GetEntity(c); err != nil {
				t.Fatalf("GetEntity: %v", err)
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

func TestListEntities_StripsGMFieldsForNonGM(t *testing.T) {
	for _, tc := range gmViewerCases {
		t.Run(tc.name, func(t *testing.T) {
			ent, et := gmTestFixtures()
			h := NewAPIHandler(nil, &stubEntityServiceForGM{entity: ent, etype: et}, &stubCampaignSvcForGM{role: tc.role}, nil)
			c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities", "", tc.bearer)

			if err := h.ListEntities(c); err != nil {
				t.Fatalf("ListEntities: %v", err)
			}
			var resp map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v (body=%s)", err, rec.Body)
			}
			data, _ := resp["data"].([]any)
			if len(data) != 1 {
				t.Fatalf("want 1 entity in list, got %d; body=%s", len(data), rec.Body)
			}
			item, _ := data[0].(map[string]any)
			fd, _ := item["fields_data"].(map[string]any)
			if _, ok := fd["might"]; !ok {
				t.Errorf("player-visible field 'might' must always be present; body=%s", rec.Body)
			}
			if _, ok := fd["gm_notes"]; ok != tc.wantGM {
				t.Errorf("gm_notes present=%v, want %v; body=%s", ok, tc.wantGM, rec.Body)
			}
		})
	}
}
