package records

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/aiplug"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/tilegrid"
)

// fakeLayoutAPI is one Characters category (id 1) with an NPC main layout,
// a tile section and a Hero layout players' characters start on.
type fakeLayoutAPI struct {
	owner      string
	types      []entities.EntityType
	named      entities.PageLayouts
	noNamed    bool
	savedMain  map[int]entities.EntityTypeLayout
	savedNamed map[int]entities.PageLayouts
}

func (f *fakeLayoutAPI) IsOwner(_ context.Context, _, u string) (bool, error) {
	return u == f.owner, nil
}
func (f *fakeLayoutAPI) Categories(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}
func (f *fakeLayoutAPI) Named(_ context.Context, et *entities.EntityType) (entities.PageLayouts, bool) {
	if f.noNamed || et.ID != 1 {
		return entities.PageLayouts{}, !f.noNamed
	}
	return f.named, true
}
func (f *fakeLayoutAPI) BlockTypes(context.Context, string) []LayoutBlockInfo {
	return []LayoutBlockInfo{
		{"title", "Title", "The page's name"}, {"entry", "Entry", "The page's body text"},
		{"image", "Image", "The page's picture"}, {"details", "Details", "The fields"},
		{"trackers", "Trackers", "Counters and clocks"}, {"posts", "Posts", "Messages on the page"},
		{"text_block", "Text block", "Your own words"}, {"ext_widget", "Extension widget", "From an extension"},
	}
}
func (f *fakeLayoutAPI) Check(_ context.Context, _ string, _ int, l entities.EntityTypeLayout) error {
	for i, r := range l.Rows {
		if r.Kind == tilegrid.KindTiles {
			continue
		}
		w := 0
		for _, c := range r.Columns {
			w += c.Width
		}
		if w != 12 {
			return badRequestf("Row %d has columns that don't fit", i+1)
		}
	}
	return nil
}
func (f *fakeLayoutAPI) SaveMain(_ context.Context, _ string, id int, l entities.EntityTypeLayout) error {
	if f.savedMain == nil {
		f.savedMain = map[int]entities.EntityTypeLayout{}
	}
	f.savedMain[id] = l
	return nil
}
func (f *fakeLayoutAPI) SaveNamed(_ context.Context, _ string, id int, p entities.PageLayouts) error {
	if f.savedNamed == nil {
		f.savedNamed = map[int]entities.PageLayouts{}
	}
	f.savedNamed[id] = p
	return nil
}

func tblock(id, typ string, cfg map[string]any) entities.TemplateBlock {
	return entities.TemplateBlock{ID: id, Type: typ, Config: cfg}
}

func newLayoutFake() *fakeLayoutAPI {
	main := entities.EntityTypeLayout{Rows: []entities.TemplateRow{
		{ID: "r1", Columns: []entities.TemplateColumn{
			{ID: "c1", Width: 8, Blocks: []entities.TemplateBlock{tblock("t1", "title", nil), tblock("e1", "entry", map[string]any{"x": 1}), tblock("tr1", "trackers", nil)}},
			{ID: "c2", Width: 4, Blocks: []entities.TemplateBlock{tblock("i1", "image", nil), tblock("d1", "details", map[string]any{"visibility": "dm_only", "cols": 2})}},
		}},
		{ID: "tiles1", Kind: tilegrid.KindTiles, Columns: []entities.TemplateColumn{}, Tiles: []tilegrid.Tile{{ID: "tl1", Type: "posts", Size: "2x1"}}},
	}}
	hero := entities.EntityTypeLayout{Rows: []entities.TemplateRow{{ID: "h1", Columns: []entities.TemplateColumn{
		{ID: "hc", Width: 12, Blocks: []entities.TemplateBlock{tblock("ht", "title", nil), tblock("he", "entry", nil)}},
	}}}}
	return &fakeLayoutAPI{
		owner: "own",
		types: []entities.EntityType{
			{ID: 1, Name: "Character", NamePlural: "Characters", Slug: "character", Layout: main},
			{ID: 2, Name: "Location", NamePlural: "Locations", Slug: "location"},
		},
		named: entities.PageLayouts{MainName: "NPC", Claimed: "hero", Extra: []entities.PageLayout{{Key: "hero", Name: "Hero", Layout: hero}}},
	}
}

var layoutOwner = Actor{UserID: "own", Role: 3}
var layoutCoDM = Actor{UserID: "co", Role: 3}

func layoutRec(action string, fields map[string]any, rows ...any) Record {
	f := map[string]any{"category": "Characters", "rows": rows}
	for k, v := range fields {
		f[k] = v
	}
	return Record{Kind: "layout", Action: action, Fields: f, Text: mdText{}}
}

func blk(typ string, kv ...any) map[string]any {
	m := map[string]any{"type": typ}
	for i := 0; i+1 < len(kv); i += 2 {
		m[kv[i].(string)] = kv[i+1]
	}
	return m
}

func tilesKept() map[string]any { return map[string]any{"tiles": "kept"} }

// sameRows is the stored arrangement, written as a record.
func sameRows() []any {
	return []any{
		row(col(8, "title", "entry", "trackers"), col(4, "image", "details")),
		tilesKept(),
	}
}

func TestLayoutKind_Plan(t *testing.T) {
	cases := []struct {
		name    string
		a       Actor
		rec     Record
		fake    func(*fakeLayoutAPI)
		want    []string
		wantErr string
	}{
		{name: "rearranged", a: layoutOwner,
			rec:  layoutRec("update", nil, row(col(4, "image", "details"), col(8, "title", "entry")), tilesKept()),
			want: []string{"the Characters NPC layout becomes: image, details (directors only) | title, entry / [tile section: posts]", "(now: title, entry, trackers | image, details (directors only) / [tile section: posts])"}},
		{name: "left out is removed with a warning", a: layoutOwner,
			rec:  layoutRec("update", nil, row(col(12, "title", "entry"))),
			want: []string{"left out, so removed: trackers, image, details, a tile section (posts)"}},
		{name: "unchanged", a: layoutOwner, rec: layoutRec("update", nil, sameRows()...), wantErr: "already has these blocks"},
		{name: "same blocks on a visibility flip is a change", a: layoutOwner,
			rec:  layoutRec("update", nil, row(col(8, "title", "entry", "trackers"), col(4, "image", blk("details", "directors_only", false))), tilesKept()),
			want: []string{"image, details /"}},
		{name: "named layout", a: layoutOwner, rec: layoutRec("update", map[string]any{"layout": "hero"}, row(col(6, "title"), col(6, "entry"))),
			want: []string{"the Characters Hero layout becomes: title | entry"}},
		{name: "main by word", a: layoutOwner, rec: layoutRec("update", map[string]any{"layout": "main"}, row(col(12, "title")), tilesKept()),
			want: []string{"the Characters NPC layout becomes"}},
		{name: "widths 1 to 12 are fine", a: layoutOwner, rec: layoutRec("update", nil, row(col(1, "title"), col(11, "entry")), tilesKept()),
			want: []string{"title | entry"}},
		{name: "width 0", a: layoutOwner, rec: layoutRec("update", nil, row(col(0, "title"), col(12, "entry"))), wantErr: "row 1, column 1: `width` is a whole number from 1 to 12"},
		{name: "width 13", a: layoutOwner, rec: layoutRec("update", nil, row(col(13, "title"))), wantErr: "`width` is a whole number from 1 to 12"},
		{name: "widths that do not add to 12", a: layoutOwner, rec: layoutRec("update", nil, row(col(5, "title"), col(5, "entry")), tilesKept()), wantErr: "don't fit"},
		{name: "unknown block", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, "title", "dragon"))), wantErr: `row 1, column 1: "dragon" is not a block`},
		{name: "text on a block that takes none", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, blk("entry", "text", "hi")))), wantErr: "only a text_block takes `text`"},
		{name: "too many tile rows", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, "title")), tilesKept(), tilesKept()), wantErr: "no tile section to keep (it has 1)"},
		{name: "tiles written as something else", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, "title")), map[string]any{"tiles": []any{"posts"}}), wantErr: "`tiles: kept`"},
		{name: "directors_only must be a boolean", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, blk("title", "directors_only", "yes")))), wantErr: "`directors_only` is true or false"},
		{name: "an extension widget cannot be added", a: layoutOwner, rec: layoutRec("update", nil, row(col(12, "ext_widget"))), wantErr: "can only be kept"},
		{name: "no rows", a: layoutOwner, rec: layoutRec("update", nil), wantErr: "list the layout's `rows:`"},
		{name: "unknown category", a: layoutOwner, rec: layoutRec("update", map[string]any{"category": "Spaceships"}, row(col(12, "title"))), wantErr: "no category called"},
		{name: "unknown layout on update", a: layoutOwner, rec: layoutRec("update", map[string]any{"layout": "Villain"}, row(col(12, "title"))), wantErr: "use action: create to add one"},
		{name: "delete is refused", a: layoutOwner, rec: layoutRec("delete", nil), wantErr: "use Customize › Categories"},
		{name: "a co-DM is refused", a: layoutCoDM, rec: layoutRec("update", nil, row(col(12, "title"))), wantErr: "only the campaign's owner"},
		{name: "a player is refused", a: Actor{UserID: "pl", Role: 1}, rec: layoutRec("update", nil, row(col(12, "title"))), wantErr: "only the campaign's owner"},
		{name: "new named layout", a: layoutOwner, rec: layoutRec("create", map[string]any{"layout": "Villain"}, row(col(12, "title", "entry"))),
			want: []string{"adds the Characters Villain layout: title, entry"}},
		{name: "new named layout while only one layout per category", a: layoutOwner, fake: func(f *fakeLayoutAPI) { f.noNamed = true },
			rec: layoutRec("create", map[string]any{"layout": "Villain"}, row(col(12, "title"))), wantErr: "one layout each"},
		{name: "a name that starts with no letter", a: layoutOwner, rec: layoutRec("create", map[string]any{"layout": "123"}, row(col(12, "title"))), wantErr: "starts with a letter"},
		{name: "too many named layouts", a: layoutOwner, fake: func(f *fakeLayoutAPI) {
			for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
				f.named.Extra = append(f.named.Extra, entities.PageLayout{Key: n, Name: strings.ToUpper(n)})
			}
		}, rec: layoutRec("create", map[string]any{"layout": "Villain"}, row(col(12, "title"))), wantErr: "the most a category keeps"},
		{name: "create on an existing layout replaces it", a: layoutOwner, rec: layoutRec("create", map[string]any{"layout": "Hero"}, row(col(12, "title"))),
			want: []string{"the Characters Hero layout becomes: title"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLayoutFake()
			if tc.fake != nil {
				tc.fake(f)
			}
			p := LayoutKind{Svc: f}.Plan(context.Background(), "c", tc.a, tc.rec)
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

// Rearranging keeps every block's ID and settings, the tile section comes back
// untouched, and a Text block's words are written as HTML.
func TestLayoutKind_ApplyRearrangesAndKeepsSettings(t *testing.T) {
	f := newLayoutFake()
	k := LayoutKind{Svc: f}
	rec := layoutRec("update", nil,
		row(col(4, "image", blk("details", "directors_only", false)), col(8, blk("entry", "directors_only", true), "title", blk("text_block", "text", "Bring **dice**"))),
		tilesKept())
	if err := k.Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	got := f.savedMain[1]
	if len(got.Rows) != 2 || len(f.savedNamed) != 0 {
		t.Fatalf("saved %+v, named %v", got, f.savedNamed)
	}
	c0, c1 := got.Rows[0].Columns[0], got.Rows[0].Columns[1]
	if c0.Width != 4 || c1.Width != 8 || c0.Blocks[0].ID != "i1" {
		t.Fatalf("columns: %+v", got.Rows[0].Columns)
	}
	if d := c0.Blocks[1]; d.ID != "d1" || d.Config["cols"] != 2 {
		t.Errorf("details lost its settings: %+v", d)
	} else if _, has := d.Config["visibility"]; has {
		t.Errorf("directors_only: false should clear the visibility: %+v", d.Config)
	}
	if e := c1.Blocks[0]; e.ID != "e1" || e.Config["x"] != 1 || e.Config["visibility"] != "dm_only" {
		t.Errorf("entry: %+v", e)
	}
	if ti := c1.Blocks[1]; ti.ID != "t1" {
		t.Errorf("title: %+v", ti)
	}
	txt := c1.Blocks[2]
	if txt.Type != "text_block" || txt.Config["content"] != "<p>Bring **dice**</p>" {
		t.Errorf("text block: %+v", txt)
	} else if _, has := txt.Config["_md"]; has {
		t.Errorf("the markdown stand-in leaked into the saved block: %+v", txt.Config)
	}
	tr := got.Rows[1]
	if tr.Kind != tilegrid.KindTiles || tr.ID != "tiles1" || len(tr.Tiles) != 1 || tr.Tiles[0].ID != "tl1" || tr.Columns == nil {
		t.Errorf("tile section: %+v", tr)
	}
}

// directors_only left out keeps what the block has, on and off.
func TestLayoutKind_DirectorsOnlyOmittedKeeps(t *testing.T) {
	f := newLayoutFake()
	k := LayoutKind{Svc: f}
	rec := layoutRec("update", nil, row(col(8, "title", "trackers"), col(4, "details")), tilesKept())
	if err := k.Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	got := f.savedMain[1].Rows[0].Columns[1].Blocks[0]
	if got.Config["visibility"] != "dm_only" {
		t.Errorf("an omitted directors_only dropped the setting: %+v", got)
	}
	if v := f.savedMain[1].Rows[0].Columns[0].Blocks[0].Config["visibility"]; v != nil {
		t.Errorf("title gained a visibility: %v", v)
	}
}

// A block placed twice takes the existing settings once; the second is new
// and gets an ID that collides with none.
func TestLayoutKind_NewBlockIDsDoNotCollide(t *testing.T) {
	f := newLayoutFake()
	k := LayoutKind{Svc: f}
	rec := layoutRec("update", nil, row(col(6, "entry", "entry", "divider0"), col(6, "title")), tilesKept())
	rec.Fields["rows"] = []any{row(col(6, "entry", "entry"), col(6, "title")), tilesKept()}
	if err := k.Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range f.savedMain[1].Rows {
		for _, t2 := range r.Tiles {
			seen[t2.ID] = true
		}
		for _, c := range r.Columns {
			for _, b := range c.Blocks {
				if seen[b.ID] {
					t.Fatalf("duplicate block id %q", b.ID)
				}
				seen[b.ID] = true
			}
		}
	}
	if !seen["e1"] || len(seen) != 4 {
		t.Errorf("ids = %v", seen)
	}
}

func TestLayoutKind_ApplyNewNamedLayoutAndUndo(t *testing.T) {
	ctx := context.Background()
	f := newLayoutFake()
	k := LayoutKind{Svc: f}
	rec := layoutRec("create", map[string]any{"layout": "Big Villain"}, row(col(12, "title", blk("text_block", "text", "Doom"))))
	snap, err := k.Before(ctx, "c", layoutOwner, rec)
	if err != nil || snap == nil {
		t.Fatalf("before = %v, %v", snap, err)
	}
	if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	pl := f.savedNamed[1]
	if len(pl.Extra) != 2 || pl.Extra[1].Key != "big-villain" || pl.Extra[1].Name != "Big Villain" || pl.MainName != "NPC" || pl.Claimed != "hero" {
		t.Fatalf("saved %+v", pl)
	}
	if err := pl.Check(); err != nil {
		t.Fatal(err)
	}
	// Undo puts the layouts back as they were, which removes the new one.
	f.named = pl
	before, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
	if before == "" {
		t.Fatal("no fingerprint")
	}
	if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
		t.Fatal(err)
	}
	if got := f.savedNamed[1]; len(got.Extra) != 1 || got.Extra[0].Key != "hero" {
		t.Errorf("restored %+v", got)
	}
}

func TestLayoutKind_UndoMain(t *testing.T) {
	ctx := context.Background()
	f := newLayoutFake()
	k := LayoutKind{Svc: f}
	rec := layoutRec("update", nil, row(col(12, "title")))
	snap, err := k.Before(ctx, "c", layoutOwner, rec)
	if err != nil || snap == nil || !strings.HasSuffix(snap.Target, ":main") || !strings.Contains(snap.Name, "NPC") {
		t.Fatalf("before = %+v, %v", snap, err)
	}
	fp1, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
	if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	// Nothing else changed the layout, so its fingerprint is the one a later
	// edit would move.
	f.types[0].Layout = f.savedMain[1]
	fp2, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
	if fp1 == fp2 {
		t.Error("the fingerprint did not move with the layout")
	}
	if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
		t.Fatal(err)
	}
	got := f.savedMain[1]
	if len(got.Rows) != 2 || fmt.Sprint(got.Rows[0].Columns[0].Blocks[1].Config["x"]) != "1" || got.Rows[1].Tiles[0].ID != "tl1" {
		t.Errorf("restored %+v", got)
	}
}

// What the export writes, sent back unchanged, is already as written; changing
// a block's visibility or words is the change that is saved.
func TestLayoutKind_ExportRoundTrips(t *testing.T) {
	ctx := context.Background()
	f := newLayoutFake()
	f.types[0].Layout.Rows[0].Columns[0].Blocks = append(f.types[0].Layout.Rows[0].Columns[0].Blocks,
		tblock("tx", "text_block", map[string]any{"content": "<p>Bring dice</p>"}))
	k := LayoutKind{Svc: f}
	out, err := k.Export(ctx, "c", layoutOwner)
	if err != nil {
		t.Fatal(err)
	}
	recs := parseBlocks(out)
	if len(recs) != 2 {
		t.Fatalf("parsed %d records from:\n%s", len(recs), out)
	}
	for _, want := range []string{"kind: layout", "category: \"Character\"", "layout: \"Hero\"", "- tiles: kept", "{type: details, directors_only: true}", "{type: text_block, text: \"Bring dice\"}"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Locations") {
		t.Errorf("a category with no layout was exported:\n%s", out)
	}
	for _, rec := range recs {
		rec.Action = ActionUpdate
		rec.Text = BodyText(nil)
		if p := k.Plan(ctx, "c", layoutOwner, rec); !strings.Contains(p.Error, "already has these blocks") {
			t.Fatalf("unchanged %v: plan = %+v\n%s", rec.Fields["layout"], p, out)
		}
	}
	main := recs[0]
	main.Action = ActionUpdate
	main.Text = BodyText(nil)
	rows := main.List("rows")
	rows[0].(map[string]any)["columns"].([]any)[1].(map[string]any)["blocks"] = []any{"image", map[string]any{"type": "details", "directors_only": false}}
	if p := k.Plan(ctx, "c", layoutOwner, main); p.Error != "" {
		t.Fatalf("a visibility change: %+v", p)
	}
	if err := k.Apply(ctx, "c", layoutOwner, main); err != nil {
		t.Fatal(err)
	}
	if _, has := f.savedMain[1].Rows[0].Columns[1].Blocks[1].Config["visibility"]; has {
		t.Error("the visibility was not cleared")
	}
}

func TestLayoutKind_ExportOwnerOnly(t *testing.T) {
	for _, a := range []Actor{layoutCoDM, {UserID: "pl", Role: 1}} {
		out, err := LayoutKind{Svc: newLayoutFake()}.Export(context.Background(), "c", a)
		if err != nil || out != "" {
			t.Errorf("%+v: %q, %v", a, out, err)
		}
	}
}

func TestLayoutsLookup(t *testing.T) {
	cases := []struct {
		name    string
		a       Actor
		fields  map[string]any
		want    []string
		lacks   []string
		wantErr string
	}{
		{name: "the owner sees everything", a: layoutOwner,
			want: []string{
				"## Page layouts", "### Characters", "Pages wear the NPC layout (the main one)", "A player's character starts on the Hero layout.",
				"**NPC layout (main)**", "**Hero layout**: title, entry.", "kind: layout\ncategory: \"Character\"\nrows:", "layout: \"Hero\"",
				"Its tile section holds posts", "### Locations", "no blocks yet", "### Blocks a layout can place",
				"- `trackers` — Trackers: Counters and clocks", "- `posts` — Posts: Messages on the page",
			}},
		{name: "one category", a: layoutOwner, fields: map[string]any{"category": "character"},
			want: []string{"### Characters"}, lacks: []string{"### Locations"}},
		{name: "one layout", a: layoutOwner, fields: map[string]any{"category": "Characters", "layout": "hero"},
			want: []string{"**Hero layout**"}, lacks: []string{"**NPC layout"}},
		{name: "the main one by name", a: layoutOwner, fields: map[string]any{"category": "Characters", "layout": "NPC"},
			want: []string{"**NPC layout (main)**"}, lacks: []string{"Hero layout**"}},
		{name: "unknown layout", a: layoutOwner, fields: map[string]any{"category": "Characters", "layout": "Villain"}, wantErr: "no layout called"},
		{name: "layout without category", a: layoutOwner, fields: map[string]any{"layout": "Hero"}, wantErr: "name the `category`"},
		{name: "unknown category", a: layoutOwner, fields: map[string]any{"category": "Ships"}, wantErr: "no category called"},
		{name: "a player", a: Actor{UserID: "pl", Role: 1}, wantErr: "campaign owner's"},
		{name: "a co-DM", a: layoutCoDM, wantErr: "campaign owner's"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Record{Kind: KindLookup, Fields: tc.fields}
			if r.Fields == nil {
				r.Fields = map[string]any{}
			}
			chip, text, err := LayoutsLookup{Svc: newLayoutFake()}.Answer(context.Background(), "c", tc.a, r)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) || text != "" {
					t.Fatalf("got %q, %v", text, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if chip == "" {
				t.Error("no chip")
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("missing %q in:\n%s", w, text)
				}
			}
			for _, w := range tc.lacks {
				if strings.Contains(text, w) {
					t.Errorf("unexpected %q in:\n%s", w, text)
				}
			}
		})
	}
}

// The lookup is found by its aliases and shows up as an owner-only tool.
func TestLayoutsLookup_PluggedInfo(t *testing.T) {
	in := LayoutsLookup{}.LookupInfo()
	if in.What != "layouts" || in.Tool.Name != "templates" || !in.Tool.Owner || len(in.Tool.Args) != 4 {
		t.Fatalf("%+v", in)
	}
	l := &Lookups{Plugged: []aiplug.Lookup{LayoutsLookup{Svc: newLayoutFake()}}}
	for _, what := range []string{"layouts", "layout", "page-layouts", "templates"} {
		if l.pluggedFor(what) == nil {
			t.Errorf("what: %s is not answered", what)
		}
	}
}
