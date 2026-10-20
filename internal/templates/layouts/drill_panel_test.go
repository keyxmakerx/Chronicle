package layouts

import (
	"bytes"
	"strings"
	"testing"
)

// The category row's click only peeks, so the drill panel must lead with a
// link to the category's own page, marked current on that page.
func TestSidebarDrillPanel_LeadsWithTheCategoryPage(t *testing.T) {
	et := SidebarEntityType{ID: 1, Slug: "locations", Name: "Location", NamePlural: "Locations", Color: "#f87171"}
	render := func(path string) string {
		ctx := navStateCase{role: 3, path: path}.ctx()
		ctx = SetEntityTypes(ctx, []SidebarEntityType{et})
		var buf bytes.Buffer
		if err := SidebarDrillPanel("c1", et, 7).Render(ctx, &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	other := render("/campaigns/c1/maps")
	home := strings.Index(other, `class="nav-drill-home"`)
	search := strings.Index(other, `class="nav-drill-search"`)
	if home < 0 || search < 0 || home > search {
		t.Fatalf("the category page link must come before search: %s", other)
	}
	if !strings.Contains(other, `href="/campaigns/c1/locations" class="nav-drill-home"`) {
		t.Errorf("the category page link must go to the category's page: %s", other)
	}
	if strings.Contains(other, `aria-current`) {
		t.Errorf("off the category's page nothing is current: %s", other)
	}
	head := other[strings.Index(other, `class="nav-drill-head"`):]
	if head = head[:strings.Index(head, "</div>")]; strings.Contains(head, `"/campaigns/c1/locations"`) {
		t.Errorf("the header is no longer a second, hidden link to the page: %s", head)
	}

	on := render("/campaigns/c1/locations")
	if !strings.Contains(on, `class="nav-drill-home" aria-current="page"`) {
		t.Errorf("on the category's page its link is current: %s", on)
	}
}
