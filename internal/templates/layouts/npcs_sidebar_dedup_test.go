// npcs_sidebar_dedup_test.go pins how the sidebar draws the Characters app:
// one row, carrying the small "Party & NPCs" caption, since it lists the party
// and NPCs together (the app catalog has no separate NPCs app; see
// internal/app/nav_layout_test.go).

package layouts

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestSidebar_CharactersRowCarriesCaption(t *testing.T) {
	ctx := context.Background()
	ctx = SetCampaignID(ctx, "camp-1")
	ctx = SetNavSections(ctx, []NavSectionView{
		{ID: "pinned", Kind: "pinned", Label: "Pinned"},
		{ID: "apps", Kind: "apps", Label: "Apps", Rows: []NavRowView{
			{Key: "app:characters", Kind: "app", Label: "Characters", Icon: "fa-masks-theater",
				URL: "/campaigns/camp-1/characters", Caption: "Party & NPCs"},
		}},
		{ID: "categories", Kind: "categories", Label: "Categories"},
	})

	var buf bytes.Buffer
	if err := Sidebar().Render(ctx, &buf); err != nil {
		t.Fatalf("render Sidebar: %v", err)
	}
	html := buf.String()

	if strings.Count(html, `href="/campaigns/camp-1/characters"`) != 1 {
		t.Errorf("sidebar must render exactly one Characters link: %s", html)
	}
	if !strings.Contains(html, "Party &amp; NPCs") {
		t.Errorf("Characters must carry the \"Party & NPCs\" caption: %s", html)
	}
}
