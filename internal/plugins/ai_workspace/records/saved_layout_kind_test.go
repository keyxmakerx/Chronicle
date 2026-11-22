package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeSavedLayouts is a built-in Standard layout (id 1) and the campaign's own
// Hero sheet (id 2).
type fakeSavedLayouts struct {
	owner   string
	presets []entities.LayoutPreset
	nextID  int
	updates []entities.UpdateLayoutPresetInput
	created []entities.CreateLayoutPresetInput
	deleted []int
}

func presetJSON(rows ...entities.TemplateRow) string {
	b, _ := json.Marshal(entities.EntityTypeLayout{Rows: rows})
	return string(b)
}

func newSavedFake() *fakeSavedLayouts {
	std := presetJSON(entities.TemplateRow{ID: "r", Columns: []entities.TemplateColumn{
		{ID: "c1", Width: 8, Blocks: []entities.TemplateBlock{tblock("a", "title", nil), tblock("b", "entry", nil)}},
		{ID: "c2", Width: 4, Blocks: []entities.TemplateBlock{tblock("c", "image", nil)}},
	}})
	hero := presetJSON(entities.TemplateRow{ID: "hr", Columns: []entities.TemplateColumn{
		{ID: "hc1", Width: 8, Blocks: []entities.TemplateBlock{tblock("h1", "title", nil), tblock("h2", "entry", map[string]any{"x": 1})}},
		{ID: "hc2", Width: 4, Blocks: []entities.TemplateBlock{tblock("h3", "details", map[string]any{"visibility": "dm_only"})}},
	}})
	return &fakeSavedLayouts{owner: "own", nextID: 10, presets: []entities.LayoutPreset{
		{ID: 1, CampaignID: "c", Name: "Standard", Description: "Two columns.", Icon: "fa-table-columns", LayoutJSON: std, IsBuiltin: true},
		{ID: 2, CampaignID: "c", Name: "Hero sheet", Description: "Portrait right.", Icon: "fa-id-card", LayoutJSON: hero},
	}}
}

func (f *fakeSavedLayouts) IsOwner(_ context.Context, _, u string) (bool, error) {
	return u == f.owner, nil
}
func (f *fakeSavedLayouts) List(context.Context, string) ([]entities.LayoutPreset, error) {
	return append([]entities.LayoutPreset(nil), f.presets...), nil
}
func (f *fakeSavedLayouts) BlockTypes(context.Context, string) []LayoutBlockInfo {
	return []LayoutBlockInfo{{"title", "Title", ""}, {"entry", "Entry", ""}, {"image", "Image", ""}, {"details", "Details", ""}, {"text_block", "Text block", ""}}
}
func (f *fakeSavedLayouts) Check(_ context.Context, _ string, l entities.EntityTypeLayout) error {
	for i, r := range l.Rows {
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
func (f *fakeSavedLayouts) Create(_ context.Context, _ string, in entities.CreateLayoutPresetInput) (*entities.LayoutPreset, error) {
	f.created = append(f.created, in)
	f.nextID++
	p := entities.LayoutPreset{ID: f.nextID, CampaignID: "c", Name: in.Name, Description: in.Description, Icon: in.Icon, LayoutJSON: in.LayoutJSON}
	f.presets = append(f.presets, p)
	return &p, nil
}
func (f *fakeSavedLayouts) Update(_ context.Context, _ string, id int, in entities.UpdateLayoutPresetInput) error {
	f.updates = append(f.updates, in)
	for i := range f.presets {
		if f.presets[i].ID == id {
			p := &f.presets[i]
			p.Name, p.Description = in.Name.Val(p.Name), in.Description.Val(p.Description)
			p.Icon, p.LayoutJSON = in.Icon.Val(p.Icon), in.LayoutJSON.Val(p.LayoutJSON)
		}
	}
	return nil
}
func (f *fakeSavedLayouts) Delete(_ context.Context, _ string, id int) error {
	f.deleted = append(f.deleted, id)
	for i := range f.presets {
		if f.presets[i].ID == id {
			f.presets = append(f.presets[:i], f.presets[i+1:]...)
			break
		}
	}
	return nil
}

func savedRec(action, name string, fields map[string]any, rows ...any) Record {
	f := map[string]any{"name": name}
	for k, v := range fields {
		f[k] = v
	}
	if len(rows) > 0 {
		f["rows"] = rows
	}
	return Record{Kind: "saved-layout", Action: action, Name: name, Fields: f, Text: mdText{}}
}

func TestSavedLayoutKind_Plan(t *testing.T) {
	heroRows := func() []any {
		return []any{row(col(8, "title", blk("entry")), col(4, blk("details", "directors_only", true)))}
	}
	cases := []struct {
		name    string
		a       Actor
		rec     Record
		want    []string
		wantErr string
	}{
		{name: "create", a: layoutOwner, rec: savedRec("create", "Villain", map[string]any{"description": "Menace", "icon": "fa-skull"}, row(col(12, "title", "entry"))),
			want: []string{`adds the saved layout "Villain": title, entry`}},
		{name: "create twice", a: layoutOwner, rec: savedRec("create", "hero SHEET", nil, row(col(12, "title"))), wantErr: "already exists; use action: update"},
		{name: "create without rows", a: layoutOwner, rec: savedRec("create", "Villain", nil), wantErr: "list the saved layout's `rows:`"},
		{name: "create with rename_to", a: layoutOwner, rec: savedRec("create", "Villain", map[string]any{"rename_to": "X"}, row(col(12, "title"))), wantErr: "nothing to rename"},
		{name: "create with a tile section", a: layoutOwner, rec: savedRec("create", "Villain", nil, row(col(12, "title")), tilesKept()), wantErr: "no tile section to keep"},
		{name: "widths that do not add to 12", a: layoutOwner, rec: savedRec("create", "Villain", nil, row(col(5, "title"))), wantErr: "don't fit"},
		{name: "unknown block", a: layoutOwner, rec: savedRec("create", "Villain", nil, row(col(12, "dragon"))), wantErr: `"dragon" is not a block`},
		{name: "bad icon", a: layoutOwner, rec: savedRec("create", "Villain", map[string]any{"icon": "<script>"}, row(col(12, "title"))), wantErr: "Font Awesome"},
		{name: "description too long", a: layoutOwner, rec: savedRec("create", "Villain", map[string]any{"description": strings.Repeat("x", 501)}, row(col(12, "title"))), wantErr: "at most 500"},
		{name: "rearranged", a: layoutOwner, rec: savedRec("update", "Hero sheet", nil, row(col(4, blk("details")), col(8, "title", "entry"))),
			want: []string{`the saved layout "Hero sheet" becomes: details (directors only) | title, entry (now: title, entry | details (directors only))`}},
		{name: "unchanged", a: layoutOwner, rec: savedRec("update", "Hero sheet", nil, heroRows()...), wantErr: "already like this"},
		{name: "rows left out is a metadata change", a: layoutOwner, rec: savedRec("update", "Hero sheet", map[string]any{"description": "New", "rename_to": "Hero page"}),
			want: []string{`is renamed "Hero page"`, `gets the description "New"`}},
		{name: "left out is removed with a warning", a: layoutOwner, rec: savedRec("update", "Hero sheet", nil, row(col(12, "title"))),
			want: []string{"left out, so removed: entry, details"}},
		{name: "rename onto another saved layout", a: layoutOwner, rec: savedRec("update", "Hero sheet", map[string]any{"rename_to": "standard"}), wantErr: "already exists"},
		{name: "update that does not exist", a: layoutOwner, rec: savedRec("update", "Villain", map[string]any{"description": "x"}), wantErr: "use action: create"},
		{name: "built-in update", a: layoutOwner, rec: savedRec("update", "Standard", nil, row(col(12, "title"))), wantErr: "built in and can't be changed or removed"},
		{name: "built-in delete", a: layoutOwner, rec: savedRec("delete", "Standard", nil), wantErr: "built in and can't be changed or removed"},
		{name: "delete", a: layoutOwner, rec: savedRec("delete", "Hero sheet", nil), want: []string{`removes the saved layout "Hero sheet"`, "keep their own layouts"}},
		{name: "delete that does not exist", a: layoutOwner, rec: savedRec("delete", "Villain", nil), wantErr: "no saved layout called"},
		{name: "a co-DM is refused", a: layoutCoDM, rec: savedRec("delete", "Hero sheet", nil), wantErr: "only the campaign's owner"},
		{name: "a player is refused", a: Actor{UserID: "pl", Role: 1}, rec: savedRec("create", "Villain", nil, row(col(12, "title"))), wantErr: "only the campaign's owner"},
		{name: "no name", a: layoutOwner, rec: savedRec("create", "", nil, row(col(12, "title"))), wantErr: "name the saved layout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := SavedLayoutKind{Svc: newSavedFake()}.Plan(context.Background(), "c", tc.a, tc.rec)
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

func TestSavedLayoutKind_ApplyCreate(t *testing.T) {
	f := newSavedFake()
	rec := savedRec("create", "Villain", map[string]any{"description": "Menace", "icon": "fa-skull"}, row(col(12, "title", blk("text_block", "text", "Doom"))))
	if err := (SavedLayoutKind{Svc: f}).Apply(context.Background(), "c", layoutOwner, rec); err != nil {
		t.Fatal(err)
	}
	if len(f.created) != 1 {
		t.Fatalf("created %v", f.created)
	}
	in := f.created[0]
	if in.Name != "Villain" || in.Description != "Menace" || in.Icon != "fa-skull" || in.CampaignID != "c" {
		t.Errorf("input %+v", in)
	}
	var l entities.EntityTypeLayout
	if err := json.Unmarshal([]byte(in.LayoutJSON), &l); err != nil || len(l.Rows) != 1 || l.Rows[0].Columns[0].Blocks[1].Config["content"] != "<p>Doom</p>" {
		t.Errorf("layout %s (%v)", in.LayoutJSON, err)
	}
}

// An update sends only what the record changes: a rename never rewrites the
// arrangement, and a new arrangement never touches the name, and the blocks
// listed again keep their IDs and settings.
func TestSavedLayoutKind_ApplyUpdateSendsOnlyChanges(t *testing.T) {
	ctx := context.Background()
	f := newSavedFake()
	k := SavedLayoutKind{Svc: f}
	if err := k.Apply(ctx, "c", layoutOwner, savedRec("update", "Hero sheet", map[string]any{"rename_to": "Hero page"})); err != nil {
		t.Fatal(err)
	}
	in := f.updates[0]
	if v, _ := in.Name.Get(); v != "Hero page" || in.LayoutJSON.Present() || in.Description.Present() || in.Icon.Present() {
		t.Errorf("rename update = %+v", in)
	}
	if err := k.Apply(ctx, "c", layoutOwner, savedRec("update", "Hero page", nil, row(col(4, blk("details")), col(8, "entry", "title")))); err != nil {
		t.Fatal(err)
	}
	in = f.updates[1]
	if in.Name.Present() || in.Description.Present() || in.Icon.Present() || !in.LayoutJSON.Present() {
		t.Fatalf("rearrange update = %+v", in)
	}
	js, _ := in.LayoutJSON.Get()
	var l entities.EntityTypeLayout
	if err := json.Unmarshal([]byte(js), &l); err != nil {
		t.Fatal(err)
	}
	c0, c1 := l.Rows[0].Columns[0], l.Rows[0].Columns[1]
	if c0.Blocks[0].ID != "h3" || c0.Blocks[0].Config["visibility"] != "dm_only" || c1.Blocks[0].ID != "h2" || c1.Blocks[0].Config["x"] != float64(1) {
		t.Errorf("settings lost: %+v %+v", c0, c1)
	}
}

func TestSavedLayoutKind_ApplyBuiltinNeverChanges(t *testing.T) {
	f := newSavedFake()
	k := SavedLayoutKind{Svc: f}
	for _, rec := range []Record{savedRec("update", "Standard", nil, row(col(12, "title"))), savedRec("delete", "Standard", nil)} {
		if err := k.Apply(context.Background(), "c", layoutOwner, rec); err == nil {
			t.Errorf("%s of a built-in layout went ahead", rec.Action)
		}
	}
	if len(f.updates)+len(f.deleted)+len(f.created) != 0 {
		t.Errorf("the service was called: %+v", f)
	}
}

func TestSavedLayoutKind_Undo(t *testing.T) {
	ctx := context.Background()
	t.Run("a made layout is removed", func(t *testing.T) {
		f := newSavedFake()
		k := SavedLayoutKind{Svc: f}
		rec := savedRec("create", "Villain", nil, row(col(12, "title")))
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || !strings.HasPrefix(snap.Target, "made:") {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if fp, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap); fp == "" {
			t.Fatal("no fingerprint")
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		if presetNamed(f.presets, "Villain") != nil || len(f.deleted) != 1 {
			t.Errorf("presets = %+v, deleted %v", f.presets, f.deleted)
		}
	})
	t.Run("an edited layout is put back", func(t *testing.T) {
		f := newSavedFake()
		k := SavedLayoutKind{Svc: f}
		rec := savedRec("update", "Hero sheet", map[string]any{"rename_to": "Hero page", "description": "Changed"}, row(col(12, "title")))
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "edit:2" {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		fp1, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap)
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if fp2, _ := k.Fingerprint(ctx, "c", layoutOwner, *snap); fp1 == fp2 {
			t.Error("the fingerprint did not move")
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		p := f.presets[1]
		if p.Name != "Hero sheet" || p.Description != "Portrait right." || !strings.Contains(p.LayoutJSON, `"h2"`) {
			t.Errorf("restored %+v", p)
		}
	})
	t.Run("a removed layout is made again", func(t *testing.T) {
		f := newSavedFake()
		k := SavedLayoutKind{Svc: f}
		rec := savedRec("delete", "Hero sheet", nil)
		snap, err := k.Before(ctx, "c", layoutOwner, rec)
		if err != nil || snap == nil || snap.Target != "gone:2" {
			t.Fatalf("before = %+v, %v", snap, err)
		}
		if err := k.Apply(ctx, "c", layoutOwner, rec); err != nil {
			t.Fatal(err)
		}
		if err := k.Restore(ctx, "c", layoutOwner, *snap); err != nil {
			t.Fatal(err)
		}
		p := presetNamed(f.presets, "Hero sheet")
		if p == nil || p.Description != "Portrait right." || p.Icon != "fa-id-card" || !strings.Contains(p.LayoutJSON, `"h3"`) {
			t.Errorf("recreated %+v", p)
		}
	})
	t.Run("a built-in layout has nothing to put back", func(t *testing.T) {
		snap, err := SavedLayoutKind{Svc: newSavedFake()}.Before(ctx, "c", layoutOwner, savedRec("delete", "Standard", nil))
		if snap != nil || err != nil {
			t.Errorf("before = %+v, %v", snap, err)
		}
	})
}

// The export writes the campaign's own layouts, never the built-in ones, and
// what it writes is already as it is when sent back.
func TestSavedLayoutKind_ExportRoundTrips(t *testing.T) {
	ctx := context.Background()
	k := SavedLayoutKind{Svc: newSavedFake()}
	out, err := k.Export(ctx, "c", layoutOwner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: saved-layout", `name: "Hero sheet"`, `icon: "fa-id-card"`, "{type: details, directors_only: true}"} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Standard") {
		t.Errorf("a built-in layout was exported:\n%s", out)
	}
	recs := parseBlocks(out)
	if len(recs) != 1 {
		t.Fatalf("parsed %d records from:\n%s", len(recs), out)
	}
	rec := recs[0]
	rec.Action, rec.Text = ActionUpdate, BodyText(nil)
	if p := k.Plan(ctx, "c", layoutOwner, rec); !strings.Contains(p.Error, "already like this") {
		t.Fatalf("unchanged: plan = %+v\n%s", p, out)
	}
	for _, a := range []Actor{layoutCoDM, {UserID: "pl", Role: 1}} {
		if out, err := k.Export(ctx, "c", a); err != nil || out != "" {
			t.Errorf("%+v: %q, %v", a, out, err)
		}
	}
}
