package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A visitor who reads a page (a look-around pass, a public campaign) gets no
// Notes tab and no star: both need membership, and would only fail to load.
func TestPageControlsForVisitors(t *testing.T) {
	ctx := layouts.SetUserID(context.Background(), "u1")
	tests := []struct {
		name   string
		role   campaigns.Role
		member bool
	}{
		{"visitor", campaigns.RoleNone, false},
		{"player", campaigns.RolePlayer, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: tt.role}
			hasNotes := false
			for _, tab := range systemPageTabList(ctx, cc, &Entity{ID: "e1"}, nil, "") {
				if tab.Key == "notes" {
					hasNotes = true
				}
			}
			if hasNotes != tt.member {
				t.Errorf("notes tab = %v, want %v", hasNotes, tt.member)
			}
			var buf bytes.Buffer
			if err := entityFavoriteButton(cc, &Entity{ID: "e1", Name: "Hero"}).Render(ctx, &buf); err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(buf.String(), "data-favorite-toggle"); got != tt.member {
				t.Errorf("star drawn = %v, want %v", got, tt.member)
			}
		})
	}
}
