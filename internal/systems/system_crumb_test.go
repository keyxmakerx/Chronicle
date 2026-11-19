package systems

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TestSystemCrumb_PointsAtBrowserWhenBookExists: with a book, the system crumb
// on a category list and an item page goes to the category browser, not to the
// book the system's own address shows.
func TestSystemCrumb_PointsAtBrowserWhenBookExists(t *testing.T) {
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Test Campaign"}}
	m := &SystemManifest{ID: "drawsteel", Name: "Draw Steel"}
	cat := &CategoryDef{Slug: "rules-glossary", Name: "Rules Glossary"}
	item := &ReferenceItem{ID: "grabbed", Name: "Grabbed"}
	book := `href="/campaigns/camp-1/systems/drawsteel/browse"`
	plain := `href="/campaigns/camp-1/systems/drawsteel"`

	tests := []struct {
		name    string
		hasBook bool
		want    string
		notWant string
	}{
		{"book", true, book, ""},
		{"no book", false, plain, "/browse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for page, render := range map[string]func(*bytes.Buffer) error{
				"category": func(b *bytes.Buffer) error {
					return CategoryListContent(cc, m, cat, nil, tt.hasBook).Render(context.Background(), b)
				},
				"item": func(b *bytes.Buffer) error {
					return ItemDetailContent(cc, m, cat, item, tt.hasBook).Render(context.Background(), b)
				},
			} {
				var buf bytes.Buffer
				if err := render(&buf); err != nil {
					t.Fatal(err)
				}
				html := buf.String()
				if !strings.Contains(html, tt.want) {
					t.Errorf("%s: missing %s in:\n%s", page, tt.want, html)
				}
				if tt.notWant != "" && strings.Contains(html, tt.notWant) {
					t.Errorf("%s: unexpected %s", page, tt.notWant)
				}
			}
		})
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
