// layouts_test.go covers the `layout:` front-matter key: a page the AI writes
// can say which of its category's layouts it wears.

package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeLayoutAccess struct {
	sets map[string]string // entity id -> layout key written
}

func (f *fakeLayoutAccess) ForType(_ context.Context, et *entities.EntityType) entities.PageLayouts {
	if et.ID != 1 {
		return entities.PageLayouts{}
	}
	return entities.PageLayouts{MainName: "NPC", Extra: []entities.PageLayout{{Key: "hero", Name: "Hero"}}}
}

func (f *fakeLayoutAccess) SetPageLayout(_ context.Context, e *entities.Entity, _ *entities.EntityType, key string) error {
	if f.sets == nil {
		f.sets = map[string]string{}
	}
	f.sets[e.ID] = key
	return nil
}

func withLayout(name, layout, action string) ParsedPage {
	p := page(name, "character", "# "+name+"\nBody.")
	p.FrontMatter.Layout = layout
	p.FrontMatter.Action = action
	return p
}

func TestParse_LayoutKeyIsKnown(t *testing.T) {
	pages := Parse("---\nname: Aric\ntype: character\nlayout: Hero\n---\n# Aric\nBody.\n")
	if len(pages) != 1 || pages[0].FrontMatter.Layout != "Hero" {
		t.Fatalf("pages = %+v", pages)
	}
	for _, w := range pages[0].Warnings {
		if strings.Contains(w, "layout") {
			t.Errorf("layout: reads as an unknown key: %q", w)
		}
	}
}

func TestCommit_Layout(t *testing.T) {
	types := []entities.EntityType{{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character", Enabled: true}}
	tests := []struct {
		name       string
		pages      []ParsedPage
		decisions  []RowDecision
		unwired    bool
		wantStatus RowStatus
		wantReason string
		wantSets   map[string]string
		wantCreate int
	}{
		{"a new hero wears the Hero layout", []ParsedPage{withLayout("Aric", "hero", "")},
			[]RowDecision{decision(true, "Aric", "character", "private", "rename")}, false, StatusCreated, "", map[string]string{"ent-aric": "hero"}, 1},
		{"the main layout by its name", []ParsedPage{withLayout("Bram", "NPC", "")},
			[]RowDecision{decision(true, "Bram", "character", "private", "rename")}, false, StatusCreated, "", map[string]string{"ent-bram": entities.MainLayoutKey}, 1},
		{"an update changes the layout", []ParsedPage{withLayout("Cass", "Hero", ActionUpdate)},
			[]RowDecision{decisionWithAction(true, "Cass", "character", "", "rename", ActionUpdate, false)}, false, StatusUpdated, "", map[string]string{"ent-cass": "hero"}, 0},
		{"no layout key writes nothing about layouts", []ParsedPage{withLayout("Dov", "", "")},
			[]RowDecision{decision(true, "Dov", "character", "private", "rename")}, false, StatusCreated, "", nil, 1},
		{"a layout the category lacks saves nothing", []ParsedPage{withLayout("Eda", "Villain", "")},
			[]RowDecision{decision(true, "Eda", "character", "private", "rename")}, false, StatusFailed, `Characters has no layout called "Villain". Its layouts: NPC, Hero.`, nil, 0},
		{"no layout service refuses the row", []ParsedPage{withLayout("Fen", "Hero", "")},
			[]RowDecision{decision(true, "Fen", "character", "private", "rename")}, true, StatusFailed, "can't set a page's layout", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeCreator{types: types, existing: map[string]*entities.Entity{
				"cass": {ID: "ent-cass", Name: "Cass", Slug: "cass", EntityTypeID: 1, CampaignID: "camp-1"},
			}}
			la := &fakeLayoutAccess{}
			c := NewCommitter(f)
			if !tt.unwired {
				c.SetLayouts(la)
			}
			res, err := c.Commit(context.Background(), "camp-1", CommitInput{OwnerID: "u-1", Pages: tt.pages, Decisions: tt.decisions})
			if err != nil {
				t.Fatal(err)
			}
			if got := res.Rows[0]; got.Status != tt.wantStatus || !strings.Contains(got.Reason, tt.wantReason) {
				t.Fatalf("status %q (%s), want %q containing %q", got.Status, got.Reason, tt.wantStatus, tt.wantReason)
			}
			if len(la.sets) != len(tt.wantSets) {
				t.Errorf("layout writes = %v, want %v", la.sets, tt.wantSets)
			}
			for k, v := range tt.wantSets {
				if la.sets[k] != v {
					t.Errorf("layout of %s = %q, want %q", k, la.sets[k], v)
				}
			}
			if len(f.createCalls) != tt.wantCreate {
				t.Errorf("creates = %d, want %d", len(f.createCalls), tt.wantCreate)
			}
		})
	}
}
