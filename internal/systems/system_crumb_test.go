package systems

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestRulesTrail_WithoutPublisher: a category's list and an entry's page
// carry the trail campaign / category / entry, never the system's name.
func TestRulesTrail_WithoutPublisher(t *testing.T) {
	cat := CategoryDef{Slug: "ancestries", Name: "Ancestries"}
	list := &RulesListView{CampaignID: "camp-1", CampaignName: "Therin", SystemID: "drawsteel", Category: cat,
		CategoryURL: "/campaigns/camp-1/systems/drawsteel/ancestries"}
	page := &EntryView{CampaignID: "camp-1", CampaignName: "Therin", SystemID: "drawsteel", Category: cat,
		CategoryURL: list.CategoryURL, Name: "Ashborn", Kind: "Ancestry"}
	for name, c := range map[string]interface {
		Render(context.Context, io.Writer) error
	}{"list": CategoryListContent(list), "page": EntryPageContent(page)} {
		var buf bytes.Buffer
		if err := c.Render(context.Background(), &buf); err != nil {
			t.Fatal(err)
		}
		html := buf.String()
		if !strings.Contains(html, `href="/campaigns/camp-1"`) || !strings.Contains(html, "Therin") {
			t.Errorf("%s: trail lost the campaign:\n%s", name, html)
		}
		if strings.Contains(html, "Draw Steel") || strings.Contains(html, "/browse") {
			t.Errorf("%s: trail names the publisher:\n%s", name, html)
		}
	}
}

// TestSystemBrowseContent_ListsCategoriesAndCrumbsUpToBook: the browser draws
// the category cards even with a book, and its system crumb leads to the book.
func TestSystemBrowseContent_ListsCategoriesAndCrumbsUpToBook(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test Campaign"}}
	m := &SystemManifest{ID: "drawsteel", Name: "Draw Steel"}
	cats := []categoryInfo{{Slug: "rules-glossary", Name: "Rules Glossary", Count: 3}}
	var buf bytes.Buffer
	if err := SystemBrowseContent(cc, m, cats, true).Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	for _, want := range []string{"/campaigns/camp-1/systems/drawsteel/rules-glossary", "Rules Glossary", `href="/campaigns/camp-1/systems/drawsteel"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s in:\n%s", want, html)
		}
	}
	if strings.Contains(html, `data-widget="rulebook"`) {
		t.Errorf("browser must not mount the book:\n%s", html)
	}
}
