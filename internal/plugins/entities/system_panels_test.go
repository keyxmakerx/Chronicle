package entities

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// The mount carries the widget, ids and the viewer's DM capability, and only
// appears when the handler resolved a panel for the page.
func TestEntitySystemPanels_Mount(t *testing.T) {
	entity := &Entity{ID: "e1"}
	panels := []SystemPanel{{Widget: "negotiation", SystemID: "drawsteel"}}
	tests := []struct {
		name   string
		cc     *campaigns.CampaignContext
		panels []SystemPanel
		wantGM string // "" means no mount expected
	}{
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}, panels, "true"},
		{"player with dm access", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true}, panels, "true"},
		{"scribe without dm access", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe}, panels, "false"},
		{"player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer}, panels, "false"},
		{"no panel resolved", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.panels != nil {
				ctx = withSystemPanels(ctx, tt.panels)
			}
			var sb strings.Builder
			if err := entitySystemPanels(tt.cc, entity).Render(ctx, &sb); err != nil {
				t.Fatal(err)
			}
			html := sb.String()
			if tt.wantGM == "" {
				if strings.Contains(html, "data-widget") {
					t.Fatalf("unexpected mount: %s", html)
				}
				return
			}
			for _, want := range []string{
				`data-widget="negotiation"`,
				`data-campaign-id="c1"`,
				`data-entity-id="e1"`,
				`data-system-id="drawsteel"`,
				fmt.Sprintf(`data-is-gm="%s"`, tt.wantGM),
			} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %s in %s", want, html)
				}
			}
			if strings.Contains(html, "<script") {
				t.Errorf("panel mount must not emit a script: %s", html)
			}
		})
	}
}

// The title block carries the panel, so a page using the layout gets it.
func TestBlockTitle_RendersSystemPanels(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	ctx := withSystemPanels(context.Background(), []SystemPanel{{Widget: "negotiation", SystemID: "drawsteel"}})
	var sb strings.Builder
	if err := blockTitle(cc, &Entity{ID: "e1", Name: "Varra"}, "csrf").Render(ctx, &sb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), `data-widget="negotiation"`) {
		t.Errorf("title block lacks the panel mount: %s", sb.String())
	}
}
