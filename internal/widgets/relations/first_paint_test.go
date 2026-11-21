// first_paint_test.go pins that the connections the entity page sends with
// itself are exactly what the list endpoint answers for the same viewer, so
// sending them early can never show a reader more than fetching them would.
package relations

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

type firstPaintService struct {
	RelationService
	rels []Relation
}

func (s firstPaintService) ListByEntity(context.Context, string, string) ([]Relation, error) {
	return s.rels, nil
}

func (s firstPaintService) GetCommonTypes() []RelationTypePair { return []RelationTypePair{{Forward: "ruled by", Reverse: "rules"}} }

func TestFirstPaintMatchesTheListEndpoint(t *testing.T) {
	gate := relDmGrantGate{
		campaignOf: map[string]string{"src": "camp-1"},
		private:    map[string]bool{"hidden-target": true},
	}
	rels := []Relation{
		{ID: 1, SourceEntityID: "src", TargetEntityID: "open-target", TargetEntityName: "Open Target"},
		{ID: 2, SourceEntityID: "src", TargetEntityID: "open-target", TargetEntityName: "Secret Link", DmOnly: true},
		{ID: 3, SourceEntityID: "src", TargetEntityID: "hidden-target", TargetEntityName: "Hidden Target"},
	}
	player := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	owner := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}, MemberRole: campaigns.RoleOwner, IsMember: true}

	tests := []struct {
		name      string
		cc        *campaigns.CampaignContext
		withTypes bool
		wantIDs   []int
	}{
		{"a player loses dm_only links and private targets", player, false, []int{1}},
		{"an owner keeps everything and gets the types to add with", owner, true, []int{1, 2, 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(firstPaintService{rels: rels})
			h.SetEntityGate(gate)

			got, types, err := h.FirstPaint(context.Background(), tt.cc, "src", "u1", tt.withTypes)
			if err != nil {
				t.Fatalf("FirstPaint: %v", err)
			}
			if (types != nil) != tt.withTypes {
				t.Errorf("types sent = %v, want %v", types != nil, tt.withTypes)
			}

			// The same viewer through the endpoint.
			e := echo.New()
			rec := httptest.NewRecorder()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "camp-1"}, {Name: "eid", Value: "src"}})
			c.Set("campaign_context", tt.cc)
			auth.SetSession(c, &auth.Session{UserID: "u1"})
			if err := h.ListRelations(c); err != nil {
				t.Fatalf("ListRelations: %v", err)
			}
			var served []Relation
			if err := json.Unmarshal(rec.Body.Bytes(), &served); err != nil {
				t.Fatal(err)
			}

			ids := func(rs []Relation) []int {
				out := []int{}
				for _, r := range rs {
					out = append(out, r.ID)
				}
				return out
			}
			if g, s := ids(got), ids(served); len(g) != len(s) {
				t.Fatalf("page sent %v, endpoint answers %v", g, s)
			}
			g := ids(got)
			if len(g) != len(tt.wantIDs) {
				t.Fatalf("page sent %v, want %v", g, tt.wantIDs)
			}
			for i := range g {
				if g[i] != tt.wantIDs[i] {
					t.Errorf("page sent %v, want %v", g, tt.wantIDs)
				}
			}
		})
	}
}

func TestFirstPaintFailsClosedWithoutAGate(t *testing.T) {
	h := NewHandler(firstPaintService{})
	if _, _, err := h.FirstPaint(context.Background(), &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c"}}, "e", "u", false); err == nil {
		t.Error("a missing gate must never serve ungated relations")
	}
}
