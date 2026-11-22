package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// fakeCatHomes holds a Locations category (id 3) with a saved home and a
// Characters category (id 1) that shows the built-in landing page.
type fakeCatHomes struct {
	owner string
	types []entities.EntityType
	reset []int
}

func (f *fakeCatHomes) IsOwner(_ context.Context, _, u string) (bool, error) {
	return u == f.owner, nil
}
func (f *fakeCatHomes) Categories(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}
func (f *fakeCatHomes) BlockTypes(context.Context, string) []LayoutBlockInfo {
	return []LayoutBlockInfo{
		{"category_header", "Heading", "The category's name"}, {"search_bar", "Search", "Find a page"},
		{"pinned_pages", "Pinned pages", "Pages pinned here"}, {"entity_grid", "All pages", "Every page"},
		{"recent_pages", "Recent pages", "Newest pages"}, {"text_block", "Text block", "Your own words"},
	}
}
func (f *fakeCatHomes) Check(_ context.Context, _ string, _ int, l campaigns.DashboardLayout) error {
	for i, r := range l.Rows {
		if r.Kind == tilegrid.KindTiles {
			continue
		}
		w := 0
		for _, c := range r.Columns {
			w += c.Width
		}
		if w > 12 {
			return badRequestf("Row %d is wider than 12", i+1)
		}
	}
	return nil
}
func (f *fakeCatHomes) Save(_ context.Context, _ string, id int, l campaigns.DashboardLayout) error {
	b, _ := json.Marshal(l)
	for i := range f.types {
		if f.types[i].ID == id {
			s := string(b)
			f.types[i].DashboardLayout = &s
		}
	}
	return nil
}
func (f *fakeCatHomes) Reset(_ context.Context, _ string, id int) error {
	f.reset = append(f.reset, id)
	for i := range f.types {
		if f.types[i].ID == id {
			f.types[i].DashboardLayout = nil
		}
	}
	return nil
}

func (f *fakeCatHomes) layoutOf(id int) campaigns.DashboardLayout {
	for i := range f.types {
		if f.types[i].ID == id {
			if l := f.types[i].ParseCategoryDashboardLayout(); l != nil {
				return *l
			}
		}
	}
	return campaigns.DashboardLayout{}
}

func newCatHomes() *fakeCatHomes {
	home := campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{
		{ID: "r1", Columns: []campaigns.DashboardColumn{{ID: "c1", Width: 12, Blocks: []campaigns.DashboardBlock{{ID: "hd", Type: "category_header"}, {ID: "tx", Type: "text_block", Config: map[string]any{"content": "<p>Start here</p>"}}}}}},
		{ID: "r2", Columns: []campaigns.DashboardColumn{
			{ID: "c2", Width: 8, Blocks: []campaigns.DashboardBlock{{ID: "gr", Type: "entity_grid", Config: map[string]any{"limit": 12}}}},
			{ID: "c3", Width: 4, Blocks: []campaigns.DashboardBlock{{ID: "pn", Type: "pinned_pages"}}},
		}},
		{ID: "t1", Kind: tilegrid.KindTiles, Columns: []campaigns.DashboardColumn{}, Tiles: []tilegrid.Tile{{ID: "tl", Type: "recent_pages", Size: "2x1"}}},
	}}
	b, _ := json.Marshal(home)
	s := string(b)
	return &fakeCatHomes{owner: "own", types: []entities.EntityType{
		{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character"},
		{ID: 3, Name: "Location", NamePlural: "Locations", Slug: "location", DashboardLayout: &s},
	}}
}

func homeOf(action, category string, rows ...any) Record {
	f := map[string]any{"category": category}
	if len(rows) > 0 {
		f["rows"] = rows
	}
	return Record{Kind: "category-home", Action: action, Fields: f, Text: mdText{}}
}

func sameHomeRows() []any {
	return []any{
		row(col(12, "category_header", blk("text_block", "text", "Start here"))),
		row(col(8, "entity_grid"), col(4, "pinned_pages")),
		tilesKept(),
	}
}

func TestCategoryHomeKind_Plan(t *testing.T) {
	cases := []struct {
		name    string
		a       Actor
		rec     Record
		want    []string
		wantErr string
	}{
		{name: "rearranged", a: layoutOwner,
			rec:  homeOf("update", "Locations", row(col(12, "category_header", "search_bar")), row(col(4, "pinned_pages"), col(8, "entity_grid")), tilesKept()),
			want: []string{"the Locations home becomes: category_header, search_bar / pinned_pages | entity_grid / [tile section: recent_pages]", "left out, so removed: text_block"}},
		{name: "unchanged", a: layoutOwner, rec: homeOf("update", "Locations", sameHomeRows()...), wantErr: "already has these blocks"},
		{name: "new words are a change", a: layoutOwner,
			rec:  homeOf("update", "Locations", row(col(12, "category_header", blk("text_block", "text", "Begin in the capital"))), row(col(8, "entity_grid"), col(4, "pinned_pages")), tilesKept()),
			want: []string{"text_block words: \"Begin in the capital\" (was \"Start here\")"}},
		{name: "a category with no home gets its own", a: layoutOwner,
			rec:  homeOf("create", "Character", row(col(12, "category_header", "entity_grid"))),
			want: []string{"the Characters home gets its own arrangement: category_header, entity_grid", "the built-in landing page"}},
		{name: "delete", a: layoutOwner, rec: homeOf("delete", "Locations"), want: []string{"goes back to Chronicle's built-in landing page"}},
		{name: "delete with no home of its own", a: layoutOwner, rec: homeOf("delete", "Character"), wantErr: "no arrangement of its own"},
		{name: "a block the editor does not offer", a: layoutOwner, rec: homeOf("update", "Locations", row(col(12, "welcome_banner"))), wantErr: `"welcome_banner" is not a block a category home can hold`},
		{name: "a widths error from the service", a: layoutOwner, rec: homeOf("update", "Locations", row(col(12, "category_header"), col(4, "entity_grid")), tilesKept()), wantErr: "wider than 12"},
		{name: "text on a block that takes none", a: layoutOwner, rec: homeOf("update", "Locations", row(col(12, blk("entity_grid", "text", "x")))), wantErr: "only a text_block takes `text`"},
		{name: "a tile section cannot be added", a: layoutOwner, rec: homeOf("create", "Character", row(col(12, "entity_grid")), tilesKept()), wantErr: "no tile section to keep (it has 0)"},
		{name: "no rows", a: layoutOwner, rec: homeOf("update", "Locations"), wantErr: "list the home's `rows:`"},
		{name: "no category", a: layoutOwner, rec: homeOf("update", "", row(col(12, "entity_grid"))), wantErr: "name the category"},
		{name: "unknown category", a: layoutOwner, rec: homeOf("update", "Ships", row(col(12, "entity_grid"))), wantErr: "no category called"},
		{name: "a co-DM is refused", a: layoutCoDM, rec: homeOf("update", "Locations", row(col(12, "entity_grid"))), wantErr: "only the campaign's owner"},
		{name: "a player is refused", a: Actor{UserID: "pl", Role: 1}, rec: homeOf("delete", "Locations"), wantErr: "only the campaign's owner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := CategoryHomeKind{Svc: newCatHomes()}.Plan(context.Background(), "c", tc.a, tc.rec)
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("error = %q, want %q", p.Error, tc.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatal(p.Error)
			}
			all := p.Summary + " " + strings.Join(p.Warnings, " ")
			for _, w := range tc.want {
				if !strings.Contains(all, w) {
					t.Errorf("missing %q in %q", w, all)
				}
			}
		})
	}
}

// A rearrangement keeps each block's ID and settings and the tile section, and
// new words are written as HTML.
func TestCategoryHomeKind_ApplyKeepsSettings(t *testing.T) {
	f := newCatHomes()
	k := CategoryHomeKind{Svc: f}
	rec := homeOf("update", "Locations", row(col(4, "pinned_pages"), col(8, "entity_grid", blk("text_block", "text", "Begin in the **capital**"))), row(col(12, "category_header")), tilesKept())
	if err := k.Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	got := f.layoutOf(3)
	if len(got.Rows) != 3 {
		t.Fatalf("saved %+v", got)
	}
	c0, c1 := got.Rows[0].Columns[0], got.Rows[0].Columns[1]
	if c0.Blocks[0].ID != "pn" || c1.Blocks[0].ID != "gr" || c1.Blocks[0].Config["limit"] != float64(12) {
		t.Errorf("settings lost: %+v %+v", c0, c1)
	}
	if tx := c1.Blocks[1]; tx.ID != "tx" || tx.Config["content"] != "<p>Begin in the **capital**</p>" {
		t.Errorf("text block: %+v", tx)
	} else if _, has := tx.Config["_md"]; has {
		t.Errorf("the markdown stand-in leaked: %+v", tx.Config)
	}
	if tr := got.Rows[2]; tr.Kind != tilegrid.KindTiles || tr.ID != "t1" || len(tr.Tiles) != 1 {
		t.Errorf("tile section: %+v", tr)
	}
}

func TestCategoryHomeKind_ApplyDeleteResets(t *testing.T) {
	f := newCatHomes()
	if err := (CategoryHomeKind{Svc: f}).Apply(context.Background(), "c", layoutOwner, homeOf("delete", "Locations")); err != nil {
		t.Fatal(err)
	}
	if len(f.reset) != 1 || f.reset[0] != 3 {
		t.Errorf("reset = %v", f.reset)
	}
}

func TestCategoryHomeKind_ApplyRefusesNonOwner(t *testing.T) {
	f := newCatHomes()
	if err := (CategoryHomeKind{Svc: f}).Apply(context.Background(), "c", layoutCoDM, homeOf("delete", "Locations")); err == nil || len(f.reset) != 0 {
		t.Errorf("a co-DM reset a home: %v %v", err, f.reset)
	}
}

func TestCategoryHomeKind_Undo(t *testing.T) {
	ctx := context.Background()
	t.Run("an edited home is put back", func(t *testing.T) {
		f := newCatHomes()
		k := CategoryHomeKind{Svc: f}
		rec := homeOf("update", "Locations", row(col(12, "entity_grid")))
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "3" || !strings.Contains(snap.Name, "Locations") {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		fp1, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if fp2, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap); fp1 == fp2 {
			t.Error("the fingerprint did not move with the home")
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		got := f.layoutOf(3)
		if len(got.Rows) != 3 || got.Rows[1].Columns[0].Blocks[0].ID != "gr" || got.Rows[2].Tiles[0].ID != "tl" {
			t.Errorf("restored %+v", got)
		}
	})
	t.Run("a new home is taken away again", func(t *testing.T) {
		f := newCatHomes()
		k := CategoryHomeKind{Svc: f}
		rec := homeOf("create", "Character", row(col(12, "entity_grid")))
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		if len(f.reset) != 1 || f.reset[0] != 1 || f.types[0].DashboardLayout != nil {
			t.Errorf("reset = %v, layout %v", f.reset, f.types[0].DashboardLayout)
		}
	})
	t.Run("a removed home is brought back", func(t *testing.T) {
		f := newCatHomes()
		k := CategoryHomeKind{Svc: f}
		rec := homeOf("delete", "Locations")
		snap, _ := k.Before(ctx, "c", layoutOwner, rec)
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		if got := f.layoutOf(3); len(got.Rows) != 3 {
			t.Errorf("restored %+v", got)
		}
	})
}

// What the export writes, sent back unchanged, is already as written.
func TestCategoryHomeKind_ExportRoundTrips(t *testing.T) {
	ctx := context.Background()
	f := newCatHomes()
	k := CategoryHomeKind{Svc: f}
	out, err := k.Export(ctx, "c", layoutOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: category-home", `category: "Location"`, "- tiles: kept", `{type: text_block, text: "Start here"}`} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Character") {
		t.Errorf("a category with no home of its own was exported:\n%s", out)
	}
	recs := parseBlocks(out)
	if len(recs) != 1 {
		t.Fatalf("parsed %d records from:\n%s", len(recs), out)
	}
	rec := recs[0]
	rec.Action, rec.Text = ActionUpdate, BodyText(nil)
	if p := k.Plan(ctx, "c", layoutOwner, rec); !strings.Contains(p.Error, "already has these blocks") {
		t.Fatalf("unchanged: plan = %+v\n%s", p, out)
	}
	for _, a := range []Actor{layoutCoDM, {UserID: "pl", Role: 1}} {
		if out, err := k.Export(ctx, "c", a); err != nil || out != "" {
			t.Errorf("%+v: %q, %v", a, out, err)
		}
	}
}
