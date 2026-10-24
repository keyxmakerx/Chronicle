package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// advEntitySvc holds an ordinary category and the DM-only Adventures category,
// and a page in each. The page-level gate mirrors what the entities service
// decides (below Scribe, the Adventures page is closed); that rule has its own
// tests next to the service, so what is under test here is that every sync API
// read consults it and the category helpers.
type advEntitySvc struct {
	entities.EntityService
}

func advTypes() []entities.EntityType {
	adv := entities.PresetCategoryAdventures
	return []entities.EntityType{
		{ID: 1, CampaignID: "camp-1", Slug: "npc", Name: "NPC"},
		{ID: 9, CampaignID: "camp-1", Slug: "adventures", Name: "Adventure", PresetCategory: &adv},
	}
}

func (advEntitySvc) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return advTypes(), nil
}

func (advEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	for _, t := range advTypes() {
		if t.ID == id {
			return &t, nil
		}
	}
	return nil, apperror.NewNotFound("entity type not found")
}

func (advEntitySvc) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	switch id {
	case "npc-page":
		return &entities.Entity{ID: id, CampaignID: "camp-1", EntityTypeID: 1}, nil
	case "adv-page":
		return &entities.Entity{ID: id, CampaignID: "camp-1", EntityTypeID: 9}, nil
	}
	return nil, apperror.NewNotFound("entity not found")
}

func (advEntitySvc) CheckEntityAccess(_ context.Context, id string, role int, _ string) (*entities.EffectivePermission, error) {
	if id == "adv-page" && role < int(campaigns.RoleScribe) {
		return &entities.EffectivePermission{}, nil
	}
	return &entities.EffectivePermission{CanView: true}, nil
}

func (advEntitySvc) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, role int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		out[id] = id != "adv-page" || role >= int(campaigns.RoleScribe)
	}
	return out, nil
}

func (advEntitySvc) GetEntityPermissions(context.Context, string) ([]entities.EntityPermission, error) {
	return nil, nil
}

type advRelationSvc struct{ relations.RelationService }

func (advRelationSvc) ListByEntity(_ context.Context, _, entityID string) ([]relations.Relation, error) {
	return []relations.Relation{
		{ID: 1, SourceEntityID: entityID, TargetEntityID: "npc-page", RelationType: "knows"},
		{ID: 2, SourceEntityID: entityID, TargetEntityID: "adv-page", RelationType: "hosts"},
	}, nil
}

func advAPICall(t *testing.T, role campaigns.Role, granted bool, call func(*APIHandler, echo.Context) error, params map[string]string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	h := NewAPIHandler(nil, advEntitySvc{}, &stubCampaignSvcForDmGrant{role: role, granted: granted}, advRelationSvc{})
	e := echo.New()
	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/api/v1/x", nil), rec)
	names, values := []string{"id"}, []string{"camp-1"}
	for k, v := range params {
		names, values = append(names, k), append(values, v)
	}
	c.SetParamNames(names...)
	c.SetParamValues(values...)
	c.Set(apiKeyContextKey, &APIKey{ID: synthKeySessionID, CampaignID: "camp-1", UserID: "u1", IsActive: true})
	return rec, call(h, c)
}

func isNotFound(err error) bool {
	ae, ok := err.(*apperror.AppError)
	return ok && ae.Code == http.StatusNotFound
}

func TestSyncAPI_EntityTypesHideAdventuresFromPlayers(t *testing.T) {
	tests := []struct {
		name    string
		role    campaigns.Role
		granted bool
		wantAdv bool
	}{
		{"player", campaigns.RolePlayer, false, false},
		{"scribe", campaigns.RoleScribe, false, false},
		{"owner", campaigns.RoleOwner, false, true},
		{"player with DM access", campaigns.RolePlayer, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/list", func(t *testing.T) {
			rec, err := advAPICall(t, tt.role, tt.granted, (*APIHandler).ListEntityTypes, nil)
			if err != nil {
				t.Fatal(err)
			}
			var resp struct {
				Total int `json:"total"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &resp)
			if has := strings.Contains(rec.Body.String(), "adventures"); has != tt.wantAdv {
				t.Errorf("Adventures listed = %v, want %v (%s)", has, tt.wantAdv, rec.Body)
			}
			if want := map[bool]int{true: 2, false: 1}[tt.wantAdv]; resp.Total != want {
				t.Errorf("total = %d, want %d", resp.Total, want)
			}
		})
		t.Run(tt.name+"/get", func(t *testing.T) {
			_, err := advAPICall(t, tt.role, tt.granted, (*APIHandler).GetEntityType, map[string]string{"typeID": "9"})
			if tt.wantAdv && err != nil {
				t.Fatalf("DM team should read the category: %v", err)
			}
			if !tt.wantAdv && !isNotFound(err) {
				t.Fatalf("err = %v, want a 404", err)
			}
		})
	}
}

func TestSyncAPI_RelationsAndPermissionsOfAdventuresPage(t *testing.T) {
	t.Run("a player gets 404 for the Adventures page's relations", func(t *testing.T) {
		_, err := advAPICall(t, campaigns.RolePlayer, false, (*APIHandler).ListEntityRelations, map[string]string{"entityID": "adv-page"})
		if !isNotFound(err) {
			t.Fatalf("err = %v, want a 404", err)
		}
	})
	t.Run("a player gets 404 for the Adventures page's permissions", func(t *testing.T) {
		_, err := advAPICall(t, campaigns.RolePlayer, false, (*APIHandler).GetEntityPermissions, map[string]string{"entityID": "adv-page"})
		if !isNotFound(err) {
			t.Fatalf("err = %v, want a 404", err)
		}
	})
	t.Run("a relation to an Adventures page is left out of an ordinary page's list", func(t *testing.T) {
		rec, err := advAPICall(t, campaigns.RolePlayer, false, (*APIHandler).ListEntityRelations, map[string]string{"entityID": "npc-page"})
		if err != nil {
			t.Fatal(err)
		}
		var got []relations.Relation
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].TargetEntityID != "npc-page" {
			t.Errorf("relations = %+v, want only the one to the ordinary page", got)
		}
	})
	t.Run("the DM team still sees both", func(t *testing.T) {
		rec, err := advAPICall(t, campaigns.RoleScribe, false, (*APIHandler).ListEntityRelations, map[string]string{"entityID": "adv-page"})
		if err != nil {
			t.Fatal(err)
		}
		var got []relations.Relation
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if len(got) != 2 {
			t.Errorf("relations = %+v, want 2", got)
		}
	})
}
