package entities

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// page_editor_test.go pins where the page editor is offered and what the
// header part draws in it.

func annalsCtx(userID string) context.Context {
	ctx := layouts.SetAppearance(context.Background(), &layouts.AppearanceData{PageLook: "annals"})
	return layouts.SetUserID(ctx, userID)
}

func TestPageEditorOn(t *testing.T) {
	prev := globalEntityShowRendererRegistry.Load()
	reg := NewEntityShowRendererRegistry()
	reg.Register("hero", MakeWidgetMountRenderer("hero-sheet"))
	globalEntityShowRendererRegistry.Store(reg)
	t.Cleanup(func() { globalEntityShowRendererRegistry.Store(prev) })

	owner := "u-owner"
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Bren", OwnerUserID: &owner}
	plain, drawn := &EntityType{Slug: "npc"}, &EntityType{Slug: "hero"}
	annals := &layouts.AppearanceData{PageLook: "annals"}
	standard := &layouts.AppearanceData{PageLook: "standard"}
	version := &eraView{ShownID: 2, Version: &PageEraVersion{}}
	picked := &eraView{Selected: 2}

	cases := []struct {
		name string
		role campaigns.Role
		user string
		look *layouts.AppearanceData
		full bool
		et   *EntityType
		era  *eraView
		want bool
	}{
		{"scribe on an Annals page", campaigns.RoleScribe, "u-s", annals, true, plain, nil, true},
		{"owner on an Annals page", campaigns.RoleOwner, "u-o", annals, true, plain, nil, true},
		{"the player who claimed it", campaigns.RolePlayer, owner, annals, true, plain, nil, true},
		{"another player", campaigns.RolePlayer, "u-x", annals, true, plain, nil, false},
		{"the Standard look", campaigns.RoleScribe, "u-s", standard, true, plain, nil, false},
		{"no look chosen", campaigns.RoleScribe, "u-s", nil, true, plain, nil, false},
		{"the DM Screen's sheet", campaigns.RoleOwner, "u-o", annals, false, plain, nil, false},
		{"a page a game system draws", campaigns.RoleOwner, "u-o", annals, true, drawn, nil, false},
		{"an era's version on the page", campaigns.RoleOwner, "u-o", annals, true, plain, version, false},
		{"an era picked with no version", campaigns.RoleOwner, "u-o", annals, true, plain, picked, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := layouts.SetUserID(context.Background(), tc.user)
			if tc.look != nil {
				ctx = layouts.SetAppearance(ctx, tc.look)
			}
			if tc.era != nil {
				ctx = withEraView(ctx, tc.era)
			}
			var got bool
			probe := templ.ComponentFunc(func(ctx context.Context, _ io.Writer) error {
				got = pageEditorOn(ctx, foldTestCC(tc.role), ent, tc.et)
				return nil
			})
			c := templ.Component(probe)
			if tc.full {
				c = withFullPage(c)
			}
			if err := c.Render(ctx, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("pageEditorOn = %v, want %v", got, tc.want)
			}
		})
	}
}

func renderEditorTitle(t *testing.T, role campaigns.Role, userID string, entity *Entity) string {
	t.Helper()
	ctx := withPageParent(annalsCtx(userID), pageParent{ID: "p1", Name: "Highwater"})
	var buf bytes.Buffer
	c := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		return withPageEditor(true).Render(templ.WithChildren(ctx, blockTitle(foldTestCC(role), entity, "tok")), w)
	})
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// In the page editor the header is a part with a pen; its old actions row
// and Edit name fold give way to the sheet, which only Scribes and up get.
func TestBlockTitle_InPageEditor(t *testing.T) {
	owner := "u-owner"
	label := "Captain"
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Mara Vell", TypeLabel: &label, OwnerUserID: &owner}

	cases := []struct {
		name       string
		role       campaigns.Role
		userID     string
		wantSheet  bool
		wantDelete bool
	}{
		{"scribe gets the sheet", campaigns.RoleScribe, "u-s", true, false},
		{"owner also gets Delete page", campaigns.RoleOwner, "u-o", true, true},
		{"claimed player edits the name alone", campaigns.RolePlayer, owner, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderEditorTitle(t, tc.role, tc.userID, ent)
			for _, want := range []string{`data-pe-part="header"`, `data-pe-pen="header"`, `data-editable="false"`} {
				if !strings.Contains(html, want) {
					t.Errorf("missing %s", want)
				}
			}
			for _, gone := range []string{"data-page-actions", `id="page-name-fold"`, "data-edit-name"} {
				if strings.Contains(html, gone) {
					t.Errorf("page editor header still draws %s", gone)
				}
			}
			if got := strings.Contains(html, "data-pe-drape"); got != tc.wantSheet {
				t.Errorf("sheet = %v, want %v", got, tc.wantSheet)
			}
			if got := strings.Contains(html, "Delete page…"); got != tc.wantDelete {
				t.Errorf("Delete page = %v, want %v", got, tc.wantDelete)
			}
			if !tc.wantSheet {
				return
			}
			for _, want := range []string{
				`data-pe-f="aka"`,
				`data-pe-f="label"`, `value="Captain"`,
				`data-pe-f="parent"`, `data-id="p1"`, `value="Highwater"`, `role="combobox"`,
				"/campaigns/c1/entities/e1/clone", "/campaigns/c1/entities/e1/history",
			} {
				if !strings.Contains(html, want) {
					t.Errorf("sheet missing %s", want)
				}
			}
			// The sheet starts shut, out of the tab order.
			if !strings.Contains(html, `data-pe-drape inert`) {
				t.Errorf("sheet must start inert")
			}
		})
	}
}

// The bar carries Edit page and the addresses the script saves to, and is
// drawn on the page itself only, never on the DM Screen's sheet.
func TestPageEditorBar_OnlyOnThePage(t *testing.T) {
	ent := &Entity{ID: "e1", CampaignID: "c1", Name: "Mara Vell"}
	et := &EntityType{Slug: "npc"}
	cc := foldTestCC(campaigns.RoleOwner)
	render := func(c templ.Component) string {
		var buf bytes.Buffer
		if err := c.Render(annalsCtx("u-o"), &buf); err != nil {
			t.Fatalf("render: %v", err)
		}
		return buf.String()
	}

	page := render(withFullPage(entityPageBody(cc, ent, et, nil, nil, false, false, false, "", "tok", "u-o")))
	for _, want := range []string{
		`data-widget="page-editor"`, "data-pe-page", "data-pe-edit", ">Edit page<",
		`data-metadata-endpoint="/campaigns/c1/entities/e1/metadata"`,
		`data-aliases-endpoint="/campaigns/c1/entities/e1/aliases"`,
		`data-search-endpoint="/campaigns/c1/entities/search"`,
		`data-pe-part="header"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page missing %s", want)
		}
	}

	sheet := render(EntitySheet(cc, ent, et, nil, nil, false, false, false, "", "tok", "u-o"))
	for _, gone := range []string{"page-editor", "data-pe-"} {
		if strings.Contains(sheet, gone) {
			t.Errorf("DM Screen sheet draws %s", gone)
		}
	}
	if !strings.Contains(sheet, "data-page-actions") {
		t.Errorf("DM Screen sheet lost the page's own actions")
	}
}

// The descriptor is always drawn, so a descriptor saved in the page editor
// can appear without a reload.
func TestBlockDetails_LabelAlwaysDrawn(t *testing.T) {
	label := "Captain"
	cases := []struct {
		name   string
		label  *string
		hidden bool
	}{
		{"with a descriptor", &label, false},
		{"without one", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			html := renderComponent(t, blockDetails(foldTestCC(campaigns.RoleScribe), &Entity{ID: "e1", CampaignID: "c1", TypeLabel: tc.label}))
			i := strings.Index(html, "data-page-label")
			if i < 0 {
				t.Fatalf("no data-page-label")
			}
			tag := html[i:strings.Index(html[i:], ">")+i]
			if got := strings.Contains(tag, "hidden"); got != tc.hidden {
				t.Errorf("hidden = %v, want %v (%s)", got, tc.hidden, tag)
			}
		})
	}
}
