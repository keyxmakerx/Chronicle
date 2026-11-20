package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeHome struct {
	owner string
	homes map[string]HomeLayout
	saved map[string]HomeLayout
	reset []string
}

func (f *fakeHome) IsOwner(_ context.Context, _, u string) (bool, error) { return u == f.owner, nil }
func (f *fakeHome) BlockTypes() []string {
	return []string{"welcome_banner", "start_here", "recent_pages", "text_block", "party", "activity_feed", "coming_up", "brochure"}
}
func (f *fakeHome) Home(_ context.Context, _, aud string) (HomeLayout, bool, error) {
	l, ok := f.homes[aud]
	return l, ok, nil
}
func (f *fakeHome) Check(_ context.Context, _ string, l HomeLayout) error {
	for _, r := range l.Rows {
		w := 0
		for _, c := range r.Columns {
			w += c.Width
		}
		if w > 12 {
			return badRequestf("widths exceed 12")
		}
	}
	return nil
}
func (f *fakeHome) Save(_ context.Context, _, aud string, l HomeLayout) error {
	if f.saved == nil {
		f.saved = map[string]HomeLayout{}
	}
	f.saved[aud] = l
	return nil
}
func (f *fakeHome) Reset(_ context.Context, _, aud string) error {
	f.reset = append(f.reset, aud)
	return nil
}

func playersFake() *fakeHome {
	return &fakeHome{owner: "own", homes: map[string]HomeLayout{"player": {Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{
		{ID: "c1", Width: 7, Blocks: []HomeBlock{{ID: "keep-start", Type: "start_here", Config: map[string]any{"x": 1}}, {ID: "b2", Type: "recent_pages", Config: map[string]any{"limit": 4}}}},
		{ID: "c2", Width: 5, Blocks: []HomeBlock{{ID: "b3", Type: "party"}}},
	}}}}}}
}

func homeRec(action, aud string, rows []any) Record {
	return Record{Kind: "home", Action: action, Fields: map[string]any{"audience": aud, "rows": rows}}
}

func col(w int, blocks ...any) map[string]any { return map[string]any{"width": w, "blocks": blocks} }
func row(cols ...any) map[string]any          { return map[string]any{"columns": cols} }

type mdText struct{}

func (mdText) HTML(b string) (*string, error)         { s := "<p>" + b + "</p>"; return &s, nil }
func (mdText) Entry(string) (*string, *string, error) { return nil, nil, nil }

func TestHomeKind_Plan(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name    string
		actor   Actor
		rec     Record
		summary string
		err     string
		warn    string
	}{
		{"rearranges", owner, homeRec(ActionUpdate, "players", []any{row(col(12, "party", "start_here"))}),
			"the players home becomes: party, start_here (now: start_here, recent_pages | party)", "", "left out, so removed: recent_pages"},
		{"not the owner", Actor{UserID: "co", Role: 3}, homeRec(ActionUpdate, "players", []any{row(col(12, "party"))}), "", "only the campaign's owner", ""},
		{"a player", Actor{UserID: "own", Role: 1}, homeRec(ActionUpdate, "players", []any{row(col(12, "party"))}), "", "only the campaign's owner", ""},
		{"unknown audience", owner, homeRec(ActionUpdate, "elves", []any{row(col(12, "party"))}), "", "not an audience", ""},
		{"unknown block", owner, homeRec(ActionUpdate, "players", []any{row(col(12, "confetti"))}), "", "not a home block", ""},
		{"too wide", owner, homeRec(ActionUpdate, "players", []any{row(col(8, "party"), col(8, "start_here"))}), "", "widths exceed", ""},
		{"text on the wrong block", owner, homeRec(ActionUpdate, "players", []any{row(col(12, map[string]any{"type": "party", "text": "hi"}))}), "", "only a text_block", ""},
		{"no change", owner, homeRec(ActionUpdate, "players", []any{row(col(7, "start_here", "recent_pages"), col(5, "party"))}), "", "already has these blocks", ""},
		{"dm-only block warns", owner, homeRec(ActionUpdate, "players", []any{row(col(12, "activity_feed"))}), "", "", "show only to the DM team"},
		{"delete borrows again", owner, Record{Kind: "home", Action: ActionDelete, Fields: map[string]any{"audience": "players"}}, "goes back to the Everyone home", "", ""},
		{"delete with nothing saved", owner, Record{Kind: "home", Action: ActionDelete, Fields: map[string]any{"audience": "guests"}}, "", "no arrangement of its own", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := HomeKind{Svc: playersFake()}.Plan(context.Background(), "c", tt.actor, tt.rec)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if !strings.Contains(p.Summary, tt.summary) {
				t.Errorf("summary = %q, want %q", p.Summary, tt.summary)
			}
			if tt.warn != "" && !strings.Contains(strings.Join(p.Warnings, ";"), tt.warn) {
				t.Errorf("warnings = %v, want %q", p.Warnings, tt.warn)
			}
		})
	}
}

// A block listed again keeps its settings and id, a new one gets none, and a
// Text Block's words are written through the engine's Text.
func TestHomeKind_ApplyKeepsSettingsAndWritesText(t *testing.T) {
	f := playersFake()
	r := homeRec(ActionUpdate, "players", []any{row(col(12, "recent_pages", "start_here", map[string]any{"type": "text_block", "text": "Bring dice."}))})
	r.Text = mdText{}
	if err := (HomeKind{Svc: f}).Apply(context.Background(), "c", Actor{UserID: "own", Role: 3}, r); err != nil {
		t.Fatal(err)
	}
	got := f.saved["player"].Rows[0].Columns[0].Blocks
	if got[0].ID != "b2" || got[0].Config["limit"] != 4 {
		t.Errorf("recent_pages lost its settings: %+v", got[0])
	}
	if got[1].ID != "keep-start" || got[1].Config["x"] != 1 {
		t.Errorf("start_here lost its settings: %+v", got[1])
	}
	if got[2].Config["content"] != "<p>Bring dice.</p>" || got[2].Config["_md"] != nil {
		t.Errorf("text block = %+v", got[2])
	}
	// the home that was read is untouched by the edit
	if f.homes["player"].Rows[0].Columns[0].Blocks[1].Config["limit"] != 4 {
		t.Error("the stored home was changed in place")
	}
}

func TestHomeKind_DeleteAndTiles(t *testing.T) {
	f := playersFake()
	owner := Actor{UserID: "own", Role: 3}
	if err := (HomeKind{Svc: f}).Apply(context.Background(), "c", owner, Record{Kind: "home", Action: ActionDelete, Fields: map[string]any{"audience": "player"}}); err != nil || len(f.reset) != 1 || f.reset[0] != "player" {
		t.Fatalf("reset = %v, err %v", f.reset, err)
	}
	f.homes["scribe"] = HomeLayout{Tiles: true, Rows: []HomeRow{{ID: "r"}}}
	p := HomeKind{Svc: f}.Plan(context.Background(), "c", owner, homeRec(ActionUpdate, "scribes", []any{row(col(12, "party"))}))
	if !strings.Contains(p.Error, "tile section") {
		t.Errorf("error = %q", p.Error)
	}
}

func TestHomeKind_Export(t *testing.T) {
	f := playersFake()
	f.homes["player"].Rows[0].Columns[1].Blocks = append(f.homes["player"].Rows[0].Columns[1].Blocks,
		HomeBlock{ID: "t", Type: "text_block", Config: map[string]any{"content": "<p>Bring <b>dice</b>.</p>"}})
	k := HomeKind{Svc: f}
	out, err := k.Export(context.Background(), "c", Actor{UserID: "own", Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"audience: players", "width: 7", "- start_here", `{type: text_block, text: "Bring dice ."}`} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q\n%s", want, out)
		}
	}
	if out, _ := k.Export(context.Background(), "c", Actor{UserID: "own", Role: 1}); out != "" {
		t.Error("a player's export must show no homes")
	}
}

// fakeBrochurePages is one campaign's pages, found by slug and by id.
type fakeBrochurePages struct{ bySlug map[string]*entities.Entity }

func (f fakeBrochurePages) GetBySlug(_ context.Context, campaignID, slug string) (*entities.Entity, error) {
	if e, ok := f.bySlug[slug]; ok {
		return e, nil
	}
	return nil, errors.New("not found")
}

func (f fakeBrochurePages) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	for _, e := range f.bySlug {
		if e.ID == id {
			return e, nil
		}
	}
	return nil, errors.New("not found")
}

func brochurePages() fakeBrochurePages {
	return fakeBrochurePages{bySlug: map[string]*entities.Entity{
		"new-player-brochure": {ID: "page-1", CampaignID: "c", Name: "New player brochure"},
		"elsewhere":           {ID: "page-9", CampaignID: "other", Name: "Elsewhere"},
	}}
}

func brochureBlock(m map[string]any) map[string]any {
	m["type"] = "brochure"
	return m
}

// A brochure block names its page by name and carries the words on the box;
// a bad name is a clear error, and listing the block again without them keeps
// what it has.
func TestHomeKind_Brochure(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	ctx := context.Background()
	tests := []struct {
		name    string
		block   any
		before  map[string]any
		wantCfg map[string]any
		err     string
	}{
		{"page and words", brochureBlock(map[string]any{"page": "New player brochure", "text": "Read this first."}), nil,
			map[string]any{"entity_id": "page-1", "content": "Read this first."}, ""},
		{"page alone", brochureBlock(map[string]any{"page": "new player brochure"}), nil,
			map[string]any{"entity_id": "page-1"}, ""},
		{"listed again keeps its page and words", "brochure", map[string]any{"entity_id": "page-1", "content": "Hello."},
			map[string]any{"entity_id": "page-1", "content": "Hello."}, ""},
		{"new words keep the page", brochureBlock(map[string]any{"text": "New words."}), map[string]any{"entity_id": "page-1", "content": "Old."},
			map[string]any{"entity_id": "page-1", "content": "New words."}, ""},
		{"empty words clear them", brochureBlock(map[string]any{"text": ""}), map[string]any{"entity_id": "page-1", "content": "Old."},
			map[string]any{"entity_id": "page-1"}, ""},
		{"page not found", brochureBlock(map[string]any{"page": "Nowhere"}), nil, nil, `no page called "Nowhere"`},
		{"another campaign's page is not found", brochureBlock(map[string]any{"page": "Elsewhere"}), nil, nil, `no page called "Elsewhere"`},
		{"page only for a brochure", map[string]any{"type": "party", "page": "New player brochure"}, nil, nil, "only a brochure takes `page`"},
		{"text only for text and brochure", map[string]any{"type": "party", "text": "x"}, nil, nil, "only a text_block or a brochure takes `text`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playersFake()
			if tt.before != nil {
				f.homes["player"] = HomeLayout{Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{{ID: "keep-bro", Type: "brochure", Config: tt.before}}}}}}}
			}
			k := HomeKind{Svc: f, Pages: brochurePages()}
			rec := homeRec(ActionUpdate, "players", []any{row(col(12, tt.block, "party"))})
			p := k.Plan(ctx, "c", owner, rec)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if err := k.Apply(ctx, "c", owner, rec); err != nil {
				t.Fatal(err)
			}
			got := f.saved["player"].Rows[0].Columns[0].Blocks[0]
			if got.Type != "brochure" || len(got.Config) != len(tt.wantCfg) {
				t.Fatalf("block = %+v, want config %v", got, tt.wantCfg)
			}
			for key, want := range tt.wantCfg {
				if got.Config[key] != want {
					t.Errorf("config[%s] = %v, want %v", key, got.Config[key], want)
				}
			}
			if tt.before != nil && got.ID != "keep-bro" {
				t.Errorf("id = %s, the block's own id should be kept", got.ID)
			}
			// the stored home is never edited in place
			if tt.before != nil && len(f.homes["player"].Rows[0].Columns[0].Blocks[0].Config) != len(tt.before) {
				t.Error("the stored home was changed in place")
			}
		})
	}
}

// Changing only a brochure's words is a change; the same words again is not.
func TestHomeKind_BrochureChangeIsSeen(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	f := playersFake()
	f.homes["player"] = HomeLayout{Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{
		{ID: "b", Type: "brochure", Config: map[string]any{"entity_id": "page-1", "content": "Old."}}}}}}}}
	k := HomeKind{Svc: f, Pages: brochurePages()}
	same := homeRec(ActionUpdate, "players", []any{row(col(12, brochureBlock(map[string]any{"text": "Old."})))})
	if p := k.Plan(context.Background(), "c", owner, same); !strings.Contains(p.Error, "already has these blocks") {
		t.Errorf("same words: error = %q", p.Error)
	}
	changed := homeRec(ActionUpdate, "players", []any{row(col(12, brochureBlock(map[string]any{"text": "New."})))})
	if p := k.Plan(context.Background(), "c", owner, changed); p.Error != "" {
		t.Errorf("new words: unexpected error %q", p.Error)
	}
}

// Export writes the brochure as the same record form it reads back.
func TestHomeKind_ExportBrochure(t *testing.T) {
	f := playersFake()
	f.homes["player"] = HomeLayout{Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{
		{ID: "a", Type: "brochure", Config: map[string]any{"entity_id": "page-1", "content": "Read this."}},
		{ID: "b", Type: "brochure", Config: map[string]any{"entity_id": "page-gone"}},
		{ID: "c", Type: "brochure"},
	}}}}}}
	out, err := HomeKind{Svc: f, Pages: brochurePages()}.Export(context.Background(), "c", Actor{UserID: "own", Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`{type: brochure, page: "New player brochure", text: "Read this."}`, "          - brochure\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("export missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "page-1") || strings.Contains(out, "page-gone") {
		t.Errorf("an id leaked into the export:\n%s", out)
	}
}
