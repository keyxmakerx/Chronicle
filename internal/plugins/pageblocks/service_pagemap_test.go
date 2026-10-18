package pageblocks

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/mapsketch"
)

func pageMapEnv() (Service, *fakeRepo) {
	svc, repo, _ := pageMapEnvEnts()
	return svc, repo
}

func pageMapEnvEnts() (Service, *fakeRepo, *fakeEntities) {
	ents := &fakeEntities{pages: map[string]fakePage{
		page:     {campaign: camp, editable: true, mapBlock: "blk-adv-map"},
		"locked": {campaign: camp, editable: false},
		"keep":   {campaign: camp, name: "The Keep", editable: true},
		"vault":  {campaign: camp, name: "The Vault", hidden: true, editable: true},
		"far":    {campaign: other, name: "Faraway", editable: true},
	}}
	repo := newFakeRepo()
	return NewService(repo, ents), repo, ents
}

func putMap(svc Service, v Viewer, doc string, version int) (*View, error) {
	return svc.Put(context.Background(), camp, page, blk, v, SaveInput{Kind: KindPageMap, Data: json.RawMessage(doc), Version: ver(version)})
}

func viewMap(t *testing.T, svc Service, v Viewer) *View {
	t.Helper()
	out, err := svc.Get(context.Background(), camp, page, blk, KindPageMap, v)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return out
}

func mapOf(t *testing.T, v *View) mapsketch.Doc {
	t.Helper()
	d, ok := v.Data.(mapsketch.Doc)
	if !ok {
		t.Fatalf("data is %T", v.Data)
	}
	return d
}

const mapDoc = `{"grid":{"w":40,"h":26},"background":"grid",
"shapes":[{"id":"s1","t":"room","x":1,"y":1,"w":4,"h":3,"name":"Hall","color":"rose"},
          {"id":"s2","t":"wall","x":0,"y":0,"pts":[[0,0],[5,0],[5,5]]}],
"pins":[{"id":"p1","x":2,"y":2,"name":"Gate","style":"icon","icon":"door-open","entityId":"keep","text":"Terms."},
        {"id":"p2","x":3,"y":3,"name":"Trap","style":"number","dm":true,"text":"Spikes"},
        {"id":"p3","x":4,"y":4,"style":"icon","icon":"gem","entityId":"vault"}]}`

func TestPageMapGetEmpty(t *testing.T) {
	svc, _ := pageMapEnv()
	v := viewMap(t, svc, player)
	b, _ := json.Marshal(v.Data)
	if string(b) != `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[]}` || v.Version != 0 || v.Kind != KindPageMap {
		t.Fatalf("got %s %+v", b, v)
	}
}

func TestPageMapViewerFiltering(t *testing.T) {
	svc, _ := pageMapEnv()
	if _, err := putMap(svc, dm, mapDoc, 0); err != nil {
		t.Fatalf("dm save: %v", err)
	}
	tests := []struct {
		name     string
		v        Viewer
		wantPins []string
		linked   map[string]string // pin id -> entity id kept
		canEdit  bool
	}{
		{"dm sees all", dm, []string{"p1", "p2", "p3"}, map[string]string{"p1": "keep", "p3": "vault"}, true},
		{"co-DM sees all", coDM, []string{"p1", "p2", "p3"}, map[string]string{"p1": "keep", "p3": "vault"}, true},
		{"scribe loses dm pin and hidden link", scribe, []string{"p1", "p3"}, map[string]string{"p1": "keep"}, false},
		{"player", player, []string{"p1", "p3"}, map[string]string{"p1": "keep"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The page is editable only for scribe and above; the player's
			// canEdit is false for that reason too.
			out := viewMap(t, svc, tc.v)
			d := mapOf(t, out)
			var ids []string
			for _, p := range d.Pins {
				ids = append(ids, p.ID)
				if p.EntityID != tc.linked[p.ID] {
					t.Errorf("pin %s entityId = %q, want %q", p.ID, p.EntityID, tc.linked[p.ID])
				}
				if (p.EntityID != "") != (p.EntityName != "" && p.EntityURL != "") {
					t.Errorf("pin %s enrichment mismatch: %+v", p.ID, p)
				}
			}
			if strings.Join(ids, ",") != strings.Join(tc.wantPins, ",") {
				t.Errorf("pins = %v, want %v", ids, tc.wantPins)
			}
			if out.CanEdit != tc.canEdit {
				t.Errorf("canEdit = %v, want %v", out.CanEdit, tc.canEdit)
			}
		})
	}
	// A pin that loses its link keeps its note and position.
	d := mapOf(t, viewMap(t, svc, scribe))
	if d.Pins[0].Text != "Terms." || d.Pins[0].EntityName != "The Keep" || d.Pins[0].EntityURL != "/campaigns/camp-1/entities/keep" {
		t.Errorf("pin = %+v", d.Pins[0])
	}
}

func TestPageMapPutRules(t *testing.T) {
	plain := `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[{"id":"p1","x":1,"y":1,"style":"number"}]}`
	dmPin := `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[{"id":"p1","x":1,"y":1,"style":"number","dm":true}]}`
	hiddenLink := `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[{"id":"p1","x":1,"y":1,"style":"number","entityId":"vault"}]}`
	t.Run("scribe saves a plain map and reads it back", func(t *testing.T) {
		svc, _ := pageMapEnv()
		out, err := putMap(svc, scribe, plain, 0)
		if err != nil || out.Version != 1 || !out.CanEdit {
			t.Fatalf("out = %+v, err = %v", out, err)
		}
	})
	t.Run("scribe cannot mark a pin DM-only", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := putMap(svc, scribe, dmPin, 0); code(err) != 403 {
			t.Fatalf("code = %d (%v), want 403", code(err), err)
		}
	})
	t.Run("dm may", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := putMap(svc, dm, dmPin, 0); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("scribe cannot overwrite a map holding DM-only pins", func(t *testing.T) {
		svc, repo := pageMapEnv()
		if _, err := putMap(svc, dm, dmPin, 0); err != nil {
			t.Fatal(err)
		}
		saves := repo.saves
		if _, err := putMap(svc, scribe, plain, 1); code(err) != 403 {
			t.Fatalf("code = %d (%v), want 403", code(err), err)
		}
		if repo.saves != saves {
			t.Errorf("a refused save wrote")
		}
	})
	t.Run("scribe cannot erase a link they cannot see", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := putMap(svc, dm, hiddenLink, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := putMap(svc, scribe, plain, 1); code(err) != 403 {
			t.Fatalf("code = %d (%v), want 403", code(err), err)
		}
	})
	t.Run("link to a page the saver cannot see is refused", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := putMap(svc, scribe, hiddenLink, 0); code(err) != 422 {
			t.Fatalf("code = %d (%v), want 422", code(err), err)
		}
	})
	t.Run("link to another campaign's page is refused", func(t *testing.T) {
		svc, _ := pageMapEnv()
		doc := strings.Replace(hiddenLink, "vault", "far", 1)
		if _, err := putMap(svc, dm, doc, 0); code(err) != 422 {
			t.Fatalf("code = %d (%v), want 422", code(err), err)
		}
	})
	t.Run("version conflict", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := putMap(svc, dm, plain, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := putMap(svc, dm, plain, 0); code(err) != 409 {
			t.Fatalf("code = %d, want 409", code(err))
		}
	})
	t.Run("readonly enrichment is ignored on save", func(t *testing.T) {
		svc, repo := pageMapEnv()
		doc := `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[{"id":"p1","x":1,"y":1,"style":"number","entityId":"keep","entityName":"Hacked","entityUrl":"javascript:x","page":"Foo"}]}`
		if _, err := putMap(svc, dm, doc, 0); err != nil {
			t.Fatal(err)
		}
		raw := string(repo.rows[key(camp, page, blk)].Data)
		if strings.Contains(raw, "Hacked") || strings.Contains(raw, "javascript") || strings.Contains(raw, "Foo") {
			t.Errorf("stored %s", raw)
		}
	})
	t.Run("locked page", func(t *testing.T) {
		svc, _ := pageMapEnv()
		_, err := svc.Put(context.Background(), camp, "locked", blk, scribe, SaveInput{Kind: KindPageMap, Data: json.RawMessage(plain), Version: ver(0)})
		if code(err) != 403 {
			t.Fatalf("code = %d", code(err))
		}
	})
}

func TestValidatePageMap(t *testing.T) {
	base := func(shapes, pins string) string {
		return `{"grid":{"w":40,"h":26},"background":"none","shapes":[` + shapes + `],"pins":[` + pins + `]}`
	}
	long := strings.Repeat("x", mapsketch.MaxName+1)
	var manyShapes, manyPins []string
	for i := 0; i <= mapsketch.MaxShapes; i++ {
		manyShapes = append(manyShapes, `{"id":"s`+itoa(i)+`","t":"room","x":1,"y":1,"w":1,"h":1}`)
	}
	for i := 0; i <= mapsketch.MaxPins; i++ {
		manyPins = append(manyPins, `{"id":"p`+itoa(i)+`","x":1,"y":1,"style":"number"}`)
	}
	tests := []struct {
		name string
		doc  string
		ok   bool
	}{
		{"empty", base("", ""), true},
		{"no grid is the default", `{"background":"hex","shapes":[],"pins":[]}`, true},
		{"wrong grid", `{"grid":{"w":10,"h":10},"background":"none","shapes":[],"pins":[]}`, false},
		{"bad background", `{"grid":{"w":40,"h":26},"background":"lava","shapes":[],"pins":[]}`, false},
		{"unknown field", `{"grid":{"w":40,"h":26},"background":"none","shapes":[],"pins":[],"extra":1}`, false},
		{"unknown shape field", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"z":1}`, ""), false},
		{"unknown shape kind", base(`{"id":"s1","t":"tower","x":1,"y":1,"w":1,"h":1}`, ""), false},
		{"box off map", base(`{"id":"s1","t":"room","x":39,"y":1,"w":2,"h":1}`, ""), false},
		{"box no size", base(`{"id":"s1","t":"room","x":1,"y":1,"w":0,"h":1}`, ""), false},
		{"box exactly fits", base(`{"id":"s1","t":"area","x":38,"y":24,"w":2,"h":2}`, ""), true},
		{"negative", base(`{"id":"s1","t":"room","x":-1,"y":1,"w":2,"h":1}`, ""), false},
		{"wall one point", base(`{"id":"s1","t":"wall","pts":[[1,1]]}`, ""), false},
		{"wall off map", base(`{"id":"s1","t":"wall","pts":[[1,1],[41,1]]}`, ""), false},
		{"path", base(`{"id":"s1","t":"path","pts":[[1,1],[4,4]],"color":"#3366cc"}`, ""), true},
		{"label ok", base(`{"id":"s1","t":"label","x":3,"y":3,"name":"Hi"}`, ""), true},
		{"label no words", base(`{"id":"s1","t":"label","x":3,"y":3}`, ""), false},
		{"box with pts", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"pts":[[1,1],[2,2]]}`, ""), false},
		{"bad colour", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"color":"red"}`, ""), false},
		{"amber colour", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"color":"#e0a030"}`, ""), false},
		{"long shape name", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"name":"`+long+`"}`, ""), false},
		{"newline in name", base(`{"id":"s1","t":"room","x":1,"y":1,"w":1,"h":1,"name":"a\nb"}`, ""), false},
		{"bad shape id", base(`{"id":"a b","t":"room","x":1,"y":1,"w":1,"h":1}`, ""), false},
		{"dup ids across shapes and pins", base(`{"id":"a","t":"room","x":1,"y":1,"w":1,"h":1}`, `{"id":"a","x":1,"y":1,"style":"number"}`), false},
		{"icon pin", base("", `{"id":"p1","x":1,"y":1,"style":"icon","icon":"crown"}`), true},
		{"icon pin without icon", base("", `{"id":"p1","x":1,"y":1,"style":"icon"}`), false},
		{"icon not allowed", base("", `{"id":"p1","x":1,"y":1,"style":"icon","icon":"dragon"}`), false},
		{"bad style", base("", `{"id":"p1","x":1,"y":1,"style":"star"}`), false},
		{"pin off map", base("", `{"id":"p1","x":41,"y":1,"style":"number"}`), false},
		{"long note", base("", `{"id":"p1","x":1,"y":1,"style":"number","text":"`+strings.Repeat("n", mapsketch.MaxNote+1)+`"}`), false},
		{"bad entity id", base("", `{"id":"p1","x":1,"y":1,"style":"number","entityId":"a/b"}`), false},
		{"too many shapes", base(strings.Join(manyShapes, ","), ""), false},
		{"too many pins", base("", strings.Join(manyPins, ",")), false},
		{"max shapes", base(strings.Join(manyShapes[:mapsketch.MaxShapes], ","), ""), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validatePageMap(json.RawMessage(tc.doc))
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if err != nil && code(err) != 422 {
				t.Errorf("code = %d, want 422", code(err))
			}
		})
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestPageMapText(t *testing.T) {
	svc, _ := pageMapEnv()
	ctx := context.Background()
	text := "background hex\nroom \"Hall\" 1,1 to 5,4 color rose\npin \"Gate\" 2,2 icon door-open page \"The Keep\": Terms.\npin \"Trap\" 3,3 number dm: Spikes\npin 4,4 icon location-dot page \"The Vault\"\n"
	res, err := svc.SavePageMapText(ctx, camp, page, blk, dm, text)
	if err != nil || len(res.Problems) != 0 || res.Markers != 3 || res.Shapes != 1 {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	if res.Doc.Pins[0].EntityID != "keep" || res.Doc.Pins[0].Page != "" {
		t.Errorf("pin = %+v", res.Doc.Pins[0])
	}

	got, version, err := svc.PageMapText(ctx, camp, page, blk, dm)
	if err != nil || version != 1 || got != text {
		t.Fatalf("dm text = %q v%d err %v\nwant %q", got, version, err, text)
	}
	// A scribe's text omits the DM-only pin and the hidden page's name.
	got, _, err = svc.PageMapText(ctx, camp, page, blk, scribe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Trap") || strings.Contains(got, "Vault") || !strings.Contains(got, "The Keep") {
		t.Errorf("scribe text = %q", got)
	}
	// ... and cannot take it to the editor, or save over it.
	if _, _, err := svc.EditorSketchText(ctx, camp, page, blk, scribe); code(err) != 403 {
		t.Errorf("editor text code = %d", code(err))
	}
	if _, err := svc.SavePageMapText(ctx, camp, page, blk, scribe, got); code(err) != 403 {
		t.Errorf("save code = %d", code(err))
	}
	// A guest sees nothing of a hidden page; a page they cannot see is 404.
	if _, _, err := svc.PageMapText(ctx, camp, "ghost", blk, dm); code(err) != 404 {
		t.Errorf("ghost page code = %d", code(err))
	}
}

func TestSavePageMapText_Rules(t *testing.T) {
	ctx := context.Background()
	t.Run("unknown and hidden names warn and keep the note", func(t *testing.T) {
		svc, _ := pageMapEnv()
		text := "pin \"A\" 1,1 page \"Nowhere\": note a\npin \"B\" 2,2 page \"The Vault\": note b\npin 3,3 page \"The Keep\""
		res, err := svc.SavePageMapText(ctx, camp, page, blk, scribe, text)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Problems) != 2 || !strings.Contains(res.Problems[0], `"Nowhere"`) || !strings.Contains(res.Problems[1], `"The Vault"`) {
			t.Fatalf("problems = %q", res.Problems)
		}
		if res.Doc.Pins[0].EntityID != "" || res.Doc.Pins[0].Text != "note a" || res.Doc.Pins[1].EntityID != "" || res.Doc.Pins[2].EntityID != "keep" {
			t.Errorf("pins = %+v", res.Doc.Pins)
		}
		// The hidden page's name is never confirmed: both warnings read alike.
		if strings.Replace(res.Problems[0], "Nowhere", "X", 1) != strings.Replace(strings.Replace(res.Problems[1], "The Vault", "X", 1), "\"B\"", "\"A\"", 1) {
			t.Errorf("warnings differ in shape: %q", res.Problems)
		}
	})
	t.Run("a scribe cannot write dm", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, scribe, "pin 1,1 dm"); code(err) != 403 {
			t.Fatalf("code = %d", code(err))
		}
	})
	t.Run("bad lines are reported and the rest saved", func(t *testing.T) {
		svc, _ := pageMapEnv()
		res, err := svc.SavePageMapText(ctx, camp, page, blk, dm, "room 1,1 to 2,2\ntower 1,1")
		if err != nil || res.Shapes != 1 || len(res.Problems) != 1 || !strings.HasPrefix(res.Problems[0], "Line 2: ") {
			t.Fatalf("res = %+v err %v", res, err)
		}
	})
	t.Run("text that reads as nothing does not wipe the map", func(t *testing.T) {
		svc, repo := pageMapEnv()
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, dm, "room 1,1 to 2,2"); err != nil {
			t.Fatal(err)
		}
		saves := repo.saves
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, dm, "tower 1,1"); code(err) != 422 {
			t.Fatalf("code = %d", code(err))
		}
		if repo.saves != saves {
			t.Error("wrote")
		}
		// Empty text is refused too: clearing a map is the editor's job.
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, dm, ""); code(err) != 422 {
			t.Fatalf("empty: code = %d", code(err))
		}
		if repo.saves != saves {
			t.Error("empty text wrote")
		}
	})
	t.Run("another kind in the block is refused", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := put(t, svc, page, dm, KindSteps, `{"steps":[]}`, 0); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, dm, "room 1,1 to 2,2"); code(err) != 422 {
			t.Fatalf("code = %d", code(err))
		}
	})
	t.Run("player cannot edit", func(t *testing.T) {
		svc, _ := pageMapEnv()
		if _, err := svc.SavePageMapText(ctx, camp, page, blk, player, "room 1,1 to 2,2"); code(err) != 403 {
			t.Fatalf("code = %d", code(err))
		}
	})
}

func TestPreviewSketch(t *testing.T) {
	svc, repo := pageMapEnv()
	ctx := context.Background()
	text := "background dots\nroom 1,1 to 4,4\npin \"Gate\" 2,2 page \"The Keep\"\npin \"X\" 3,3 page \"Nowhere\": kept\nbogus"
	out, err := svc.PreviewSketch(ctx, camp, page, blk, scribe, text)
	if err != nil {
		t.Fatal(err)
	}
	if repo.saves != 0 {
		t.Error("preview saved")
	}
	if out.Data.Background != "dots" || len(out.Warnings) != 2 {
		t.Fatalf("out = %+v", out)
	}
	if p := out.Data.Pins[0]; p.EntityID != "keep" || p.EntityName != "The Keep" || p.Page != "" {
		t.Errorf("resolved pin = %+v", p)
	}
	if p := out.Data.Pins[1]; p.EntityID != "" || p.Page != "Nowhere" || p.Text != "kept" {
		t.Errorf("unresolved pin = %+v", p)
	}
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), `"warnings":[`) {
		t.Errorf("json = %s", b)
	}
	if o, _ := svc.PreviewSketch(ctx, camp, page, blk, dm, "room 1,1 to 2,2"); o == nil || o.Warnings == nil {
		t.Errorf("warnings must be [] not null: %+v", o)
	}
	if _, err := svc.PreviewSketch(ctx, camp, page, blk, player, text); code(err) != 403 {
		t.Errorf("player code = %d", code(err))
	}
	if _, err := svc.PreviewSketch(ctx, camp, page, blk, scribe, "pin 1,1 dm"); code(err) != 403 {
		t.Errorf("dm pin code = %d", code(err))
	}
}

func TestFirstPageMapBlock(t *testing.T) {
	svc, _ := pageMapEnv()
	ctx := context.Background()
	if id, ok, err := svc.FirstPageMapBlock(ctx, camp, page); err != nil || !ok || id != "blk-adv-map" {
		t.Errorf("got %q %v %v", id, ok, err)
	}
	for _, eid := range []string{"locked", "ghost"} {
		if _, ok, _ := svc.FirstPageMapBlock(ctx, camp, eid); ok {
			t.Errorf("%s: want none", eid)
		}
	}
	if _, ok, _ := svc.FirstPageMapBlock(ctx, other, page); ok {
		t.Error("foreign campaign must find nothing")
	}
}

// A link to a page that was deleted or trashed is nobody's to lose, so it
// neither locks the map nor counts as hidden.
func TestPageMap_LinkToGonePage(t *testing.T) {
	ctx := context.Background()
	doc := `{"grid":{"w":40,"h":26},"shapes":[],"pins":[{"id":"p1","x":2,"y":2,"style":"number","entityId":"keep"},{"id":"p2","x":3,"y":3,"style":"number","entityId":"vault"}]}`
	for _, tt := range []struct {
		name     string
		v        Viewer
		gone     []string
		wantEdit bool
	}{
		{"owner, linked page deleted", dm, []string{"keep"}, true},
		{"scribe, every unseen link deleted", scribe, []string{"vault"}, true},
		{"scribe, a live hidden link remains", scribe, []string{"keep"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, ents := pageMapEnvEnts()
			if _, err := putMap(svc, dm, doc, 0); err != nil {
				t.Fatal(err)
			}
			for _, id := range tt.gone {
				delete(ents.pages, id)
			}
			if got := viewMap(t, svc, tt.v).CanEdit; got != tt.wantEdit {
				t.Fatalf("canEdit = %v, want %v", got, tt.wantEdit)
			}
			_, err := putMap(svc, tt.v, `{"grid":{"w":40,"h":26},"shapes":[],"pins":[]}`, 1)
			if tt.wantEdit && err != nil {
				t.Fatalf("put: %v", err)
			}
			if !tt.wantEdit && code(err) != 403 {
				t.Fatalf("put code = %d, want 403", code(err))
			}
			_, _, err = svc.EditorSketchText(ctx, camp, page, blk, tt.v)
			if tt.wantEdit == (err != nil) {
				t.Fatalf("sketch text err = %v", err)
			}
		})
	}
}

// Text written from a map reads back to the same links, even when two pages
// share a name or a name makes no Latin slug.
func TestPageMap_TextRoundTripKeepsLinks(t *testing.T) {
	ctx := context.Background()
	svc, _, ents := pageMapEnvEnts()
	ents.pages["gate-a"] = fakePage{campaign: camp, name: "The Gate", editable: true}
	ents.pages["gate-b"] = fakePage{campaign: camp, name: "The Gate", editable: true}
	ents.pages["kyiv"] = fakePage{campaign: camp, name: "Київ \"Old\" Town", editable: true}
	doc := `{"grid":{"w":40,"h":26},"shapes":[],"pins":[{"id":"p1","x":2,"y":2,"style":"number","entityId":"gate-b"},{"id":"p2","x":3,"y":3,"style":"number","entityId":"kyiv"}]}`
	if _, err := putMap(svc, dm, doc, 0); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		text, _, err := svc.EditorSketchText(ctx, camp, page, blk, dm)
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.SavePageMapText(ctx, camp, page, blk, dm, text)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Problems) != 0 {
			t.Fatalf("problems %q for text %q", res.Problems, text)
		}
		got := mapOf(t, viewMap(t, svc, dm))
		if len(got.Pins) != 2 || got.Pins[0].EntityID != "gate-b" || got.Pins[1].EntityID != "kyiv" {
			t.Fatalf("round %d: pins = %+v from %q", i, got.Pins, text)
		}
	}
}
