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

// The panels are a block of their own: the title no longer carries them, and
// the Game System Panels block mounts them where the layout places it.
func TestSystemPanels_RenderOnlyAsTheirOwnBlock(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner}
	ctx := withSystemPanels(context.Background(), []SystemPanel{{Widget: "negotiation", SystemID: "drawsteel"}})
	ent := &Entity{ID: "e1", Name: "Varra"}

	var title strings.Builder
	if err := blockTitle(cc, ent, "csrf").Render(ctx, &title); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(title.String(), `data-widget="negotiation"`) {
		t.Errorf("title block must not mount the panels on its own: %s", title.String())
	}

	reg := NewBlockRegistry()
	RegisterCoreBlocks(reg)
	SetGlobalBlockRegistry(reg)
	t.Cleanup(func() { SetGlobalBlockRegistry(nil) })
	var block strings.Builder
	if err := RenderBlock(ctx, TemplateBlock{ID: "b", Type: BlockSystemPanels}, cc, ent, &EntityType{ID: 7}, "csrf").Render(ctx, &block); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block.String(), `data-widget="negotiation"`) {
		t.Errorf("Game System Panels block lacks the panel mount: %s", block.String())
	}
}
