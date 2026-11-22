package campaigns

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

type fakeCategoryLister struct{ cats []CustomizeCategory }

func (f fakeCategoryLister) ListCustomizeCategories(context.Context, string) ([]CustomizeCategory, error) {
	return f.cats, nil
}

type pageReply struct {
	code     int
	body     string
	location string
}

// getPage runs a Customize page handler as the campaign's owner.
func getPage(t *testing.T, h *Handler, handler echo.HandlerFunc, target string, params ...string) pageReply {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	var names, values []string
	for i := 0; i+1 < len(params); i += 2 {
		names, values = append(names, params[i]), append(values, params[i+1])
	}
	c.SetPathValues(pathValuesOf(names, values))
	c.Set(contextKeyCampaign, &CampaignContext{Campaign: &Campaign{ID: "camp-1", Name: "Ashfall"}, MemberRole: RoleOwner})
	auth.SetSession(c, &auth.Session{UserID: "owner-1"})
	err := handler(c)
	r := pageReply{code: rec.Code, body: rec.Body.String(), location: rec.Header().Get("Location")}
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		r.code = ae.Code
	} else if err != nil {
		t.Fatalf("handler: %v", err)
	}
	return r
}

func pagesHandler() *Handler {
	h := &Handler{}
	h.SetCustomizeCategoryLister(fakeCategoryLister{cats: []CustomizeCategory{
		{ID: 7, Slug: "npcs", Name: "NPC", NamePlural: "NPCs", Icon: "fa-user"},
	}})
	return h
}

// The wall draws the frame: Find, the save bar and the draft's version, so the
// shell script has what it needs to start, and a picture for the shared parts
// and each category. It has no bar of tabs.
func TestCustomizeWall_RendersFrame(t *testing.T) {
	h := pagesHandler()
	r := getPage(t, h, h.Customize, "/campaigns/camp-1/customize")
	if r.code != http.StatusOK {
		t.Fatalf("code = %d", r.code)
	}
	for _, want := range []string{
		`data-widget="customize-shell"`, `data-czs-savebar`, `data-version="0"`, `data-czw`, "Customize Ashfall",
		`href="/campaigns/camp-1/customize/look#frame"`, `href="/campaigns/camp-1/customize/look#style"`,
		`href="/campaigns/camp-1/customize/categories/7/layout"`, "NPCs", `data-czs-find`,
		`href="/campaigns/camp-1/customize/categories"`, "All categories",
		`data-share="frame"`, `data-czw-shares="colours"`,
	} {
		if !strings.Contains(r.body, want) {
			t.Errorf("wall missing %q", want)
		}
	}
	for _, not := range []string{"data-czs-top", `data-czs-area="overview"`} {
		if strings.Contains(r.body, not) {
			t.Errorf("wall should not contain %q", not)
		}
	}
}

// A plugin's area opens at its own address; an unknown one is a 404.
func TestCustomizePluginArea(t *testing.T) {
	h := pagesHandler()
	h.RegisterCustomizeTab(func(*CampaignContext) CustomizeTab {
		return CustomizeTab{ID: "maps", Label: "Map frame", Icon: "fa-solid fa-map", Content: tabBody("MAPS-BODY")}
	})
	r := getPage(t, h, h.CustomizePluginArea, "/campaigns/camp-1/customize/x/maps", "tab", "maps")
	if r.code != http.StatusOK || !strings.Contains(r.body, "MAPS-BODY") {
		t.Fatalf("maps area: code %d, body has area: %v", r.code, strings.Contains(r.body, "MAPS-BODY"))
	}
	if !strings.Contains(r.body, `href="/campaigns/camp-1/customize/"`) && !strings.Contains(r.body, `href="/campaigns/camp-1/customize"`) {
		t.Error("the part's bar has no way back to the wall")
	}
	if r := getPage(t, h, h.CustomizePluginArea, "/campaigns/camp-1/customize/x/nope", "tab", "nope"); r.code != http.StatusNotFound {
		t.Errorf("unknown area: code %d, want 404", r.code)
	}
}

func TestCustomizeCategory_Addresses(t *testing.T) {
	h := pagesHandler()
	base := "/campaigns/camp-1/customize/categories/"
	tests := []struct {
		name, key, tab string
		code           int
		location       string
	}{
		{"no tab opens the layout", "7", "", http.StatusSeeOther, base + "7/layout"},
		{"own category opens", "7", "settings", http.StatusOK, ""},
		{"another campaign's category is not found", "99", "settings", http.StatusNotFound, ""},
		{"a key that is not an id is not found", "abc", "settings", http.StatusNotFound, ""},
		{"an unknown tab is not found", "7", "secrets", http.StatusNotFound, ""},
		{"a new category opens its settings", "new-tavern", "layout", http.StatusSeeOther, base + "new-tavern/settings"},
		{"a new category's settings open", "new-tavern", "settings", http.StatusOK, ""},
		{"a malformed new key is not found", "new-Bad_Key", "settings", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := getPage(t, h, h.CustomizeCategory, base+tt.key+"/"+tt.tab, "key", tt.key, "tab", tt.tab)
			if r.code != tt.code {
				t.Fatalf("code = %d, want %d", r.code, tt.code)
			}
			if r.location != tt.location {
				t.Errorf("location = %q, want %q", r.location, tt.location)
			}
		})
	}
}

// The bar inside a part names where you are: "Everything" first, the part's
// name, then its own tabs (a category's five, Look's rooms, the homes). There
// is no top-level row of areas, no rail and no drawer for Look or the homes.
func TestCustomizeTopBar(t *testing.T) {
	h := &Handler{}
	town := 7
	h.SetCustomizeCategoryLister(fakeCategoryLister{cats: []CustomizeCategory{
		{ID: 7, Name: "Location", NamePlural: "Locations", Color: "#ef4444", Pages: 21},
		{ID: 8, Name: "Town", NamePlural: "Towns", Color: "#ef4444", Pages: 9, ParentID: &town},
		{ID: 9, Name: "Character", NamePlural: "Characters", Color: "red;background:url(x)", Pages: 34},
	}})
	tests := []struct {
		name    string
		handler echo.HandlerFunc
		target  string
		params  []string
		want    []string
		wantNot []string
	}{
		{"a category open", h.CustomizeCategory, "/campaigns/camp-1/customize/categories/8/layout", []string{"key", "8", "tab", "layout"},
			[]string{
				`data-czs-catbar`,
				`href="/campaigns/camp-1/customize" data-czs-back`, `<span>Everything</span>`,
				`<span data-czs-catname>Towns</span>`,
				`href="/campaigns/camp-1/customize/categories/8/layout" data-czs-cattab="layout" aria-current="page"`,
				`data-czs-cattab="settings"`,
				`data-czs-bar-slot`,
				`id="czs-dr-categories"`,
				`href="/campaigns/camp-1/customize/categories/8/layout" class="czs-pt is-kind" aria-current="page"`,
				`<span class="czs-sw" style="background:#ef4444;"`,
				`<span class="czs-cnt">34</span>`,
				`All categories`, `New category`,
			},
			[]string{"czs-rail", "czs-crumbs", "background:red", `data-czs-part`, `id="czs-dr-look"`, `id="czs-dr-home"`, `data-czs-area="look"`}},
		{"a home open", h.CustomizeHome, "/campaigns/camp-1/customize/home/player", []string{"who", "player"},
			[]string{`<span>Everything</span>`, `<span class="czs-catname is-static" tabindex="-1">Home pages</span>`,
				`href="/campaigns/camp-1/customize/home/player" aria-current="page"`,
				`href="/campaigns/camp-1/customize/home/guest"`, `href="/campaigns/camp-1/customize/home/default"`},
			// The retired owner dashboard has no tab.
			[]string{`aria-label="Which home"`, `customize/home/dm`, `data-czs-part`}},
		{"Look open", h.CustomizeLook, "/campaigns/camp-1/customize/look", nil,
			[]string{`<span>Everything</span>`, `<span class="czs-catname is-static" tabindex="-1">Look</span>`,
				`href="/campaigns/camp-1/customize/look#frame" data-czs-room="frame"`,
				`href="/campaigns/camp-1/customize/look#style" data-czs-room="style"`,
				`data-czs-room="pages"`, `data-czs-room="people"`},
			[]string{`id="czs-dr-look"`}},
		{"the wall", h.Customize, "/campaigns/camp-1/customize", nil,
			[]string{`czs-is-wall`, `data-czw`},
			[]string{`data-czs-part`, `data-czs-top`, `data-czs-back`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := getPage(t, h, tt.handler, tt.target, tt.params...)
			if r.code != http.StatusOK {
				t.Fatalf("code = %d", r.code)
			}
			for _, w := range tt.want {
				if !strings.Contains(r.body, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tt.wantNot {
				if strings.Contains(r.body, w) {
					t.Errorf("should not contain %q", w)
				}
			}
		})
	}
}
