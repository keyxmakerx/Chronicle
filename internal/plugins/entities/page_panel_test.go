package entities

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestShow_ItemsPanelAskedForByMembersOnly pins that a character page asks
// for the items-and-money panel only for a member: its route is for players
// and up, and the load a signed-out visitor's page sent made htmx move the
// whole tab to the sign-in page.
func TestShow_ItemsPanelAskedForByMembersOnly(t *testing.T) {
	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	reg.Register(BlockMeta{Type: BlockCharacterItems, Label: "Items and money", Contexts: []string{"template"}}, RenderCharacterItemsBlock)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })

	player := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}
	cases := []struct {
		name  string
		cc    *campaigns.CampaignContext
		draws func(role int) bool
		want  bool
	}{
		{"player", player, nil, true},
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true}, nil, true},
		{"signed-out visitor on a public campaign", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", IsPublic: true}, MemberRole: campaigns.RoleNone}, nil, false},
		// A player who can't act for the hero gets an empty panel, so the
		// page leaves it out and a system sheet keeps its own item list.
		{"player the panel won't draw for", player, func(int) bool { return false }, false},
		{"player the panel draws for", player, func(role int) bool { return role == int(campaigns.RolePlayer) }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent, et := dmOnlyFixture()
			ent.IsPrivate = false
			et.Layout = EntityTypeLayout{Rows: []TemplateRow{fullWidthRow("r",
				TemplateBlock{ID: "t", Type: "title"}, TemplateBlock{ID: "i", Type: BlockCharacterItems})}}
			h := &Handler{service: &dmGrantEntitySvc{entity: ent, etype: et}}
			panel := PagePanel{Addon: "items-panel", URL: func(cid, eid string) string {
				return "/campaigns/" + cid + "/items-panel/" + eid
			}}
			if tc.draws != nil {
				draws := tc.draws
				panel.Draws = func(_ context.Context, cid, eid, uid string, role int) bool {
					return cid == "c1" && eid == "e1" && uid == "u1" && draws(role)
				}
			}
			h.SetCharacterPagePanel(panel)

			rec := httptest.NewRecorder()
			c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/campaigns/c1/entities/e1", nil), rec)
			c.SetPathValues(echo.PathValues{{Name: "id", Value: "c1"}, {Name: "eid", Value: "e1"}})
			c.Set("campaign_context", tc.cc)
			if tc.cc.IsMember {
				auth.SetSession(c, &auth.Session{UserID: "u1"})
			}
			if err := h.Show(c); err != nil {
				t.Fatalf("Show: %v", err)
			}
			if got := strings.Contains(rec.Body.String(), "/items-panel/e1"); got != tc.want {
				t.Errorf("page asks for the items panel = %v, want %v", got, tc.want)
			}
		})
	}
}
