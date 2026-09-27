// npcs_sidebar_dedup_test.go pins that the sidebar no longer renders a
// standalone "NPCs" addon link (redundant with Characters, which already
// lists the party and NPCs together) even when the "npcs" addon is enabled
// and configured into the top nav — and that Characters carries the small
// "Party & NPCs" caption in its place.

package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSidebar_NPCsLinkGoneCharactersCarriesCaption(t *testing.T) {
	ctx := context.Background()
	ctx = SetCampaignID(ctx, "camp-1")
	ctx = SetEnabledAddons(ctx, map[string]bool{"npcs": true})
	ctx = SetSidebarItems(ctx, []SidebarItemView{
		{Type: "dashboard"},
		{Type: "addon", Slug: "npcs"},
		{Type: "all_pages"},
	})

	var buf bytes.Buffer
	if err := Sidebar().Render(ctx, &buf); err != nil {
		t.Fatalf("render Sidebar: %v", err)
	}
	html := buf.String()

	if strings.Contains(html, `href="/campaigns/camp-1/npcs"`) {
		t.Errorf("sidebar must not render a link to /npcs; the addon item is redundant with Characters: %s", html)
	}
	if !strings.Contains(html, `href="/campaigns/camp-1/characters"`) {
		t.Errorf("sidebar must still render the Characters link: %s", html)
	}
	if !strings.Contains(html, "Party &amp; NPCs") {
		t.Errorf("Characters must carry the \"Party & NPCs\" caption: %s", html)
	}
}
