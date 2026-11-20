package entities

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestBrochurePanels(t *testing.T) {
	tests := []struct {
		name, in string
		want     int
	}{
		{"cover only", "<h1>T</h1><p>x</p>", 1},
		{"a panel per heading", "<h1>T</h1><h2>A</h2><p>a</p><h2 id=\"x\">B</h2><H2>C</H2>", 4},
		{"third level headings are not panels", "<h2>A</h2><h3>B</h3>", 2},
		{"empty", "", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := brochurePanels(tt.in); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBrochureWords(t *testing.T) {
	long := repeatWords("tide", 100)
	got := brochureWords("  Read   this\nfirst. ")
	if got != "Read this first." {
		t.Errorf("got %q", got)
	}
	if r := []rune(brochureWords(long)); len(r) > brochureWordsLen+1 || !strings.HasSuffix(string(r), "…") {
		t.Errorf("long words not cut: %d runes", len(r))
	}
}

func TestBrochurePanelsLabel(t *testing.T) {
	if brochurePanelsLabel(1) != "1 panel inside" || brochurePanelsLabel(12) != "12 panels inside" {
		t.Error("count is not in words")
	}
}

// The box shows the picked page's title and the owner's words (or its first
// line) and carries the page's served HTML in a dialog; players never get
// secrets, a hidden page is an empty answer, and the owner gets the slot.
func TestBrochureHandler(t *testing.T) {
	body := `<h1>Therin</h1><p><em>A fantasy campaign.</em></p><h2>Panel one</h2><p>Hello <span data-secret="true">the cult</span></p><script>alert(1)</script><h2>Panel two</h2>`
	pages := map[string]*Entity{
		"open":   {ID: "open", CampaignID: "c1", EntityTypeID: 3, Name: "New player brochure", Visibility: VisibilityDefault, EntryHTML: &body},
		"hidden": {ID: "hidden", CampaignID: "c1", EntityTypeID: 3, Name: "The Drowned Saint", Visibility: VisibilityDefault, IsPrivate: true},
		"other":  {ID: "other", CampaignID: "c2", EntityTypeID: 3, Name: "Elsewhere", Visibility: VisibilityDefault},
	}
	tests := []struct {
		name     string
		role     campaigns.Role
		page     string
		text     string
		want     []string
		wantNot  []string
		wantNone bool
	}{
		{"player, first line", campaigns.RolePlayer, "open", "",
			[]string{"New player brochure", "A fantasy campaign.", "3 panels inside", "Open the brochure", "data-brochure-dialog", "Panel one"},
			[]string{"the cult", "alert(1)", "data-dm-only"}, false},
		{"player, owner's words win", campaigns.RolePlayer, "open", "Read this first.",
			[]string{"Read this first."}, []string{"A fantasy campaign.</span>"}, false},
		{"owner never sees secrets in it", campaigns.RoleOwner, "open", "", []string{"New player brochure"}, []string{"the cult"}, false},
		{"player, hidden page", campaigns.RolePlayer, "hidden", "", nil, nil, true},
		{"player, nothing picked", campaigns.RolePlayer, "", "", nil, nil, true},
		{"owner, hidden page is marked", campaigns.RoleOwner, "hidden", "", []string{"The Drowned Saint", "data-dm-only"}, nil, false},
		{"owner, nothing picked", campaigns.RoleOwner, "", "", []string{"Pick the page you wrote for new players", "/campaigns/c1/customize/home"}, nil, false},
		{"owner, page deleted", campaigns.RoleOwner, "gone", "", []string{"was deleted"}, nil, false},
		{"owner, another campaign's page", campaigns.RoleOwner, "other", "", []string{"was deleted"}, []string{"Elsewhere"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			et := &EntityType{ID: 3, CampaignID: "c1", Slug: "place", Name: "Place", Enabled: true}
			typeRepo := &mockEntityTypeRepo{findByIDFn: func(context.Context, int) (*EntityType, error) { cp := *et; return &cp, nil }}
			entityRepo := &mockEntityRepo{findByIDFn: func(_ context.Context, eid string) (*Entity, error) {
				e, ok := pages[eid]
				if !ok {
					return nil, apperror.NewNotFound("entity not found")
				}
				cp := *e
				return &cp, nil
			}}
			h := &Handler{service: NewEntityService(entityRepo, typeRepo, &mockPermissionRepo{})}
			q := url.Values{"page": {tt.page}, "text": {tt.text}}
			c, rec := advCtx(tt.role, false, http.MethodGet, "/campaigns/c1/brochure?"+q.Encode())
			if err := h.Brochure(c); err != nil {
				t.Fatalf("Brochure err = %v", err)
			}
			out := rec.Body.String()
			if tt.wantNone != (out == "") {
				t.Fatalf("empty = %v, want %v: %q", out == "", tt.wantNone, out)
			}
			for _, w := range tt.want {
				if !strings.Contains(out, w) {
					t.Errorf("body missing %q:\n%s", w, out)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(out, w) {
					t.Errorf("body should not hold %q:\n%s", w, out)
				}
			}
		})
	}
}
