package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// A bare category opens large with the sentence for the viewer; a player is
// offered no way to add a page, and a scribe is. The frame sits inside
// #entity-list so a search swap does not nest a second frame.
func TestCategoryDashboard_Opening(t *testing.T) {
	et := &EntityType{ID: 4, Name: "Player Character", NamePlural: "Player Characters", Slug: "player-character"}
	render := func(role campaigns.Role, ctx context.Context, total int, opts ListOptions) string {
		cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1", Name: "Ashen"}, MemberRole: role}
		var buf bytes.Buffer
		if err := CategoryDashboardContent(cc, et, nil, nil, total, opts, "").Render(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	player := render(campaigns.RolePlayer, context.Background(), 0, ListOptions{})
	if !strings.Contains(player, `data-app-opening="empty"`) || !strings.Contains(player, "shares under Player Characters") ||
		strings.Contains(player, "New page") || strings.Contains(player, "Create ") {
		t.Errorf("a player should see the sharing sentence and no way to add:\n%s", player)
	}
	owner := render(campaigns.RoleOwner, layouts.SetIsOwner(context.Background(), true), 0, ListOptions{})
	if !strings.Contains(owner, "New page") || !strings.Contains(owner, "Add a page and it will show here") {
		t.Errorf("the owner should be offered New page:\n%s", owner)
	}
	if i := strings.Index(owner, `id="entity-list"`); i < 0 || strings.Index(owner, "data-app-frame") < i {
		t.Errorf("the frame must sit inside #entity-list")
	}
	filtered := render(campaigns.RolePlayer, context.Background(), 0, ListOptions{Query: "zz"})
	if !strings.Contains(filtered, `data-app-opening="full"`) || !strings.Contains(filtered, "Nothing matches") {
		t.Errorf("a search with no result keeps the small opening and the search box:\n%s", filtered)
	}
}
