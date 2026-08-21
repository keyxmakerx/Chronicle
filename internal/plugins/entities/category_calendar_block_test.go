// category_calendar_block_test.go — regression for the operator bug: a
// calendar_preview block added to an entity type's category dashboard renders
// nothing. This exercises the SERVER round-trip (stored layout JSON → parse →
// render) so we can localize the bug: if this passes, the Go path is sound and
// the drop is client-side (the layout editor not persisting the block).
package entities

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestCategoryDashboard_CalendarBlockRenders(t *testing.T) {
	layout := `{"rows":[{"id":"r1","columns":[{"id":"c1","width":12,"blocks":[` +
		`{"id":"b1","type":"calendar_preview","config":{}}]}]}]}`
	et := &EntityType{
		ID: 7, CampaignID: "camp-1", Slug: "npc", Name: "NPC", NamePlural: "NPCs",
		DashboardLayout: &layout,
	}

	// 1) Parse: the stored layout must yield the calendar block.
	parsed := et.ParseCategoryDashboardLayout()
	if parsed == nil {
		t.Fatal("ParseCategoryDashboardLayout returned nil for a valid calendar layout")
	}
	if len(parsed.Rows) != 1 || len(parsed.Rows[0].Columns) != 1 || len(parsed.Rows[0].Columns[0].Blocks) != 1 {
		t.Fatalf("layout shape lost in parse: %+v", parsed)
	}
	if got := parsed.Rows[0].Columns[0].Blocks[0].Type; got != "calendar_preview" {
		t.Fatalf("block type = %q, want calendar_preview", got)
	}

	// 2) Render: the custom dashboard must show the calendar card.
	//
	// CALV5-PLACEHOLDER: this asserted the card's own header ("Upcoming
	// Events"). While the calendar is rebuilt the block renders the shared
	// rebuild notice, so the marker moves — but the THING BEING TESTED does
	// not: that a custom dashboard renders its configured calendar block
	// instead of silently falling back to the default dashboard. Restore the
	// header assertion when V5 restores the card.
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "C"}, MemberRole: campaigns.RoleOwner}
	var buf bytes.Buffer
	if err := CategoryDashboardContent(cc, et, nil, nil, 0, ListOptions{}, "", nil).Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := buf.String()
	if !strings.Contains(html, "is being rebuilt") {
		t.Errorf("calendar block did not render at all; custom dashboard likely fell back to default.\nHTML:\n%s", html)
	}
}
