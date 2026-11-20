package armory

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A hidden line is labelled with the campaign's own word for its GM, the
// way the rest of the page names it.
func TestHeroLogEntryUsesCampaignGMTerm(t *testing.T) {
	tests := []struct {
		name string
		term string
		want string
	}{
		{"DM campaign", "DM", "DM only"},
		{"Director campaign", "Director", "Director only"},
		{"no system", "", "GM only"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := layouts.SetGMTerm(context.Background(), tt.term)
			var sb strings.Builder
			e := LogEntry{Note: true, Before: "x", DirectorOnly: true}
			if err := heroLogEntry(&CharacterPanelView{}, e).Render(ctx, &sb); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(sb.String(), tt.want) {
				t.Errorf("got %q, want it to contain %q", sb.String(), tt.want)
			}
			if tt.term != "Director" && strings.Contains(sb.String(), "Director only") {
				t.Errorf("hard-coded Director label in %q", sb.String())
			}
		})
	}
}
