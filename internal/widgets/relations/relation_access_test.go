// relation_access_test.go pins that delete and metadata edits, which take a
// bare relation ID, still require the caller to see both ends of the relation
// and to be allowed to see a dm_only one.
package relations

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type relWriteService struct {
	RelationService
	rel   *Relation
	calls int
}

func (s *relWriteService) GetByID(context.Context, int) (*Relation, error) { return s.rel, nil }
func (s *relWriteService) Delete(context.Context, int) error               { s.calls++; return nil }
func (s *relWriteService) UpdateMetadata(context.Context, int, json.RawMessage) error {
	s.calls++
	return nil
}

func TestRelationWrites_RequireAccess(t *testing.T) {
	gate := relDmGrantGate{
		campaignOf: map[string]string{"a": "camp-1", "b": "camp-1", "hidden": "camp-1"},
		private:    map[string]bool{"hidden": true},
	}
	player := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	scribe := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleScribe, IsMember: true}
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner, IsMember: true}
	granted := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true, IsDmGranted: true}

	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		rel       Relation
		wantCalls int
	}{
		{"visible relation", player, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b"}, 1},
		{"other campaign", player, Relation{CampaignID: "camp-2", SourceEntityID: "a", TargetEntityID: "b"}, 0},
		{"hidden target", player, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "hidden"}, 0},
		{"hidden source", player, Relation{CampaignID: "camp-1", SourceEntityID: "hidden", TargetEntityID: "a"}, 0},
		{"dm_only as player", player, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b", DmOnly: true}, 0},
		{"dm_only as scribe", scribe, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b", DmOnly: true}, 0},
		{"dm_only as owner", owner, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b", DmOnly: true}, 1},
		{"dm_only as dm-granted", granted, Relation{CampaignID: "camp-1", SourceEntityID: "a", TargetEntityID: "b", DmOnly: true}, 1},
	}
	ops := map[string]func(*Handler, echo.Context) error{
		"delete":   (*Handler).DeleteRelation,
		"metadata": (*Handler).UpdateRelationMetadata,
	}
	for opName, op := range ops {
		for _, tc := range tests {
			t.Run(opName+"/"+tc.name, func(t *testing.T) {
				rel := tc.rel
				svc := &relWriteService{rel: &rel}
				h := NewHandler(svc)
				h.SetEntityGate(gate)
				e := echo.New()
				req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"metadata":{}}`))
				c := e.NewContext(req, httptest.NewRecorder())
				c.SetParamNames("id", "eid", "rid")
				c.SetParamValues("camp-1", "a", "7")
				c.Set("campaign_context", tc.cc)
				auth.SetSession(c, &auth.Session{UserID: "u1"})

				err := op(h, c)
				if tc.wantCalls == 0 {
					var ae *apperror.AppError
					if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
						t.Fatalf("err = %v, want 404", err)
					}
				} else if err != nil {
					t.Fatalf("err = %v, want success", err)
				}
				if svc.calls != tc.wantCalls {
					t.Errorf("mutations = %d, want %d", svc.calls, tc.wantCalls)
				}
			})
		}
	}
}
