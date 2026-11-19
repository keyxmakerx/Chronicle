package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// A Scribe's own session reaches the sync API's write routes, so the routes
// that take a bare id must refuse it what the web refuses it: relations it
// cannot see, and retyping pages it cannot edit or into a DM-only category.

// scribeRulesEntitySvc serves pages in camp-1: "hidden" is private, the rest
// are public. Type 9 is the DM-only category.
type scribeRulesEntitySvc struct {
	entities.EntityService
	retyped []string
}

func (s *scribeRulesEntitySvc) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	return &entities.Entity{ID: id, CampaignID: "camp-1", IsPrivate: id == "hidden", EntityTypeID: 1}, nil
}

func (s *scribeRulesEntitySvc) CheckEntityAccess(_ context.Context, id string, role int, _ string) (*entities.EffectivePermission, error) {
	if role >= int(campaigns.RoleOwner) {
		return &entities.EffectivePermission{CanView: true, CanEdit: true}, nil
	}
	if id == "hidden" {
		return &entities.EffectivePermission{}, nil
	}
	return &entities.EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

func (s *scribeRulesEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	et := &entities.EntityType{ID: id, CampaignID: "camp-1"}
	if id == 9 {
		preset := entities.PresetCategoryAdventures
		et.PresetCategory = &preset
	}
	return et, nil
}

func (s *scribeRulesEntitySvc) BulkUpdateType(_ context.Context, _ string, ids []string, _ int) (int, error) {
	s.retyped = append(s.retyped, ids...)
	return len(ids), nil
}

func scribeRulesContext(method, body string, params echo.PathValues) *echo.Context {
	c, _ := gateContext(method, body)
	c.SetPathValues(params)
	return c
}

func TestSyncRelationWrites_FollowWebRules(t *testing.T) {
	cases := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		relation  int
		want      int
	}{
		{"scribe, plain relation", campaigns.RoleScribe, false, 1, http.StatusOK},
		{"scribe, DM-only relation", campaigns.RoleScribe, false, 2, http.StatusNotFound},
		{"scribe, relation to a hidden page", campaigns.RoleScribe, false, 3, http.StatusNotFound},
		{"co-DM, DM-only relation", campaigns.RoleScribe, true, 2, http.StatusOK},
		{"owner, relation to a hidden page", campaigns.RoleOwner, false, 3, http.StatusOK},
	}
	for _, tc := range cases {
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			t.Run(tc.name+" "+method, func(t *testing.T) {
				rels := &stubRelationSvcForScope{rels: map[int]*relations.Relation{
					1: {ID: 1, CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b"},
					2: {ID: 2, CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b", DmOnly: true},
					3: {ID: 3, CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "hidden"},
				}}
				h := NewAPIHandler(&stubSyncSvcForRole{}, &scribeRulesEntitySvc{}, gateCampaignSvc{role: tc.role, dmGranted: tc.dmGranted}, rels)
				c := scribeRulesContext(method, `{"metadata":{"x":1}}`, echo.PathValues{
					{Name: "id", Value: "camp-1"}, {Name: "relationId", Value: string(rune('0' + tc.relation))},
				})
				var err error
				if method == http.MethodPut {
					err = h.UpdateRelation(c)
				} else {
					err = h.DeleteRelation(c)
				}
				got := gateStatus(err)
				if tc.want == http.StatusOK {
					if err != nil {
						t.Fatalf("refused: %v", err)
					}
					return
				}
				if got != tc.want {
					t.Fatalf("status = %d (err %v), want %d", got, err, tc.want)
				}
				if len(rels.deleted)+len(rels.updated) > 0 {
					t.Fatal("refused request still wrote")
				}
			})
		}
	}
}

func TestSyncBulkRetype_FollowsWebRules(t *testing.T) {
	cases := []struct {
		name      string
		role      campaigns.Role
		dmGranted bool
		ids       []string
		typeID    int
		want      int
	}{
		{"scribe retypes visible pages", campaigns.RoleScribe, false, []string{"a", "b"}, 2, http.StatusOK},
		{"scribe retypes a hidden page", campaigns.RoleScribe, false, []string{"a", "hidden"}, 2, http.StatusNotFound},
		{"scribe moves pages into the DM-only category", campaigns.RoleScribe, false, []string{"a"}, 9, http.StatusForbidden},
		{"player retypes a visible page", campaigns.RolePlayer, false, []string{"a"}, 2, http.StatusForbidden},
		{"co-DM retypes a hidden page into the DM-only category", campaigns.RoleScribe, true, []string{"hidden"}, 9, http.StatusOK},
		{"owner retypes a hidden page", campaigns.RoleOwner, false, []string{"hidden"}, 2, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := &scribeRulesEntitySvc{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, gateCampaignSvc{role: tc.role, dmGranted: tc.dmGranted}, nil)
			body, _ := json.Marshal(map[string]any{"entity_ids": tc.ids, "entity_type_id": tc.typeID})
			c := scribeRulesContext(http.MethodPost, string(body), echo.PathValues{{Name: "id", Value: "camp-1"}})
			err := h.BulkUpdateEntityType(c)
			if got := gateStatus(err); got != tc.want {
				t.Fatalf("status = %d (err %v), want %d", got, err, tc.want)
			}
			if tc.want != http.StatusOK && len(ents.retyped) > 0 {
				t.Fatalf("refused request still retyped %s", strings.Join(ents.retyped, ","))
			}
		})
	}
}
