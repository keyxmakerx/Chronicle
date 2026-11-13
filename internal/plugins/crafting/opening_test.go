package crafting

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// The page opens on one line with the sentence for whoever is looking, and a
// player is offered no control to make anything.
func TestCraftingPage_OpeningByViewer(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "The Ashen Reach"}}
	tests := []struct {
		name     string
		ctx      context.Context
		director bool
		want     string
	}{
		{"player", context.Background(), false, "The recipes your characters know."},
		{"co-DM", layouts.SetIsDmGranted(context.Background(), true), true, "Recipes, the materials they take"},
		{"owner", layouts.SetIsOwner(context.Background(), true), true, "Recipes, the materials they take"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := CraftingPage(cc, tt.director, "").Render(tt.ctx, &buf); err != nil {
				t.Fatal(err)
			}
			out := buf.String()
			if !strings.Contains(out, tt.want) || !strings.Contains(out, `data-app-opening="full"`) {
				t.Errorf("missing %q or the one-line opening in:\n%s", tt.want, out)
			}
			// Only the opening itself: the layout shell has its own "New" links.
			i := strings.Index(out, `data-app-opening="full"`)
			j := strings.Index(out[i:], `data-widget="crafting"`)
			opening := out[i : i+j]
			if !tt.director && (strings.Contains(opening, "Create") || strings.Contains(opening, "Add ") || strings.Contains(opening, "New ")) {
				t.Errorf("a player is offered a way to make something:\n%s", opening)
			}
		})
	}
}
