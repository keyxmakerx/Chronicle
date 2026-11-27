package decks

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// The Table drawer's Hand says the campaign's own word for its game master,
// as the drawer's dice and Tonight do: "Director" only under a system that
// names it so, and the default "GM" otherwise.
func TestHandSaysTheCampaignsWordForTheGameMaster(t *testing.T) {
	invited := HandDraw{DeckID: "d1", DeckName: "Omens", Invited: true, CanDraw: true, Heroes: []Hero{{ID: "h1", Name: "Tam"}}}
	waiting := &OpenedCard{DeckID: "d1", DeckName: "Omens", Copy: PlayerCopy{Waiting: true}}
	tests := []struct {
		name string
		ctx  context.Context
		term string
	}{
		{"no game system", context.Background(), "GM"},
		{"a system that says Director", layouts.SetGMTerm(context.Background(), "Director"), "Director"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var empty, draws, opened strings.Builder
			if err := HandBody(handPage{CampaignID: "c1", View: &HandView{}}).Render(tt.ctx, &empty); err != nil {
				t.Fatal(err)
			}
			if err := HandBody(handPage{CampaignID: "c1", View: &HandView{Draws: []HandDraw{invited}}}).Render(tt.ctx, &draws); err != nil {
				t.Fatal(err)
			}
			if err := HandOpened(openedPage{CampaignID: "c1", Card: waiting}).Render(tt.ctx, &opened); err != nil {
				t.Fatal(err)
			}
			for _, want := range []struct{ got, line string }{
				{empty.String(), "No cards yet. Your " + tt.term + " deals them."},
				{draws.String(), "Your " + tt.term + " invites you to draw."},
				{opened.String(), "Waiting for your " + tt.term},
			} {
				if !strings.Contains(want.got, want.line) {
					t.Errorf("missing %q in %.300s", want.line, want.got)
				}
			}
			other := map[string]string{"GM": "Director", "Director": "GM"}[tt.term]
			for _, got := range []string{empty.String(), draws.String(), opened.String()} {
				if strings.Contains(got, "Your "+other) || strings.Contains(got, "your "+other) {
					t.Errorf("says %q under a campaign whose word is %q", other, tt.term)
				}
			}
		})
	}
}
