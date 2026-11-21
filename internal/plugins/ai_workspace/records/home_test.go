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
	return []string{"welcome_banner", "start_here", "recent_pages", "text_block", "party", "activity_feed", "coming_up", "brochure", "table_agreement", "making_a_hero", "where_we_play"}
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

func (mdText) HTML(b string) (*string, error) {
	if b == "" {
		return nil, nil // as the real renderer does for no words
	}
	s := "<p>" + b + "</p>"
	return &s, nil
}
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
		{"text only for text and brochure", map[string]any{"type": "party", "text": "x"}, nil, nil, "only a text_block, a brochure, a table_agreement or a making_a_hero takes `text`"},
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

// A Table agreement and a Making a hero block take `text` like a Text Block,
// and a Where we play block takes its address and voice; a line left out is
// kept, an empty one clears it, and another block refuses them.
func TestHomeKind_WrittenBlocks(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	ctx := context.Background()
	long := strings.Repeat("a", maxHomeText+1)
	tests := []struct {
		name    string
		block   any
		before  map[string]any
		typ     string
		wantCfg map[string]any
		err     string
	}{
		{"table agreement words", map[string]any{"type": "table_agreement", "text": "## Safety\nNo gore."}, nil, "table_agreement",
			map[string]any{"content": "<p>## Safety\nNo gore.</p>"}, ""},
		{"making a hero words", map[string]any{"type": "making_a_hero", "text": "Start at level 1."}, nil, "making_a_hero",
			map[string]any{"content": "<p>Start at level 1.</p>"}, ""},
		{"listed again keeps its words", "table_agreement", map[string]any{"content": "<p>Kept.</p>"}, "table_agreement",
			map[string]any{"content": "<p>Kept.</p>"}, ""},
		{"empty words clear them", map[string]any{"type": "making_a_hero", "text": ""}, map[string]any{"content": "<p>Old.</p>"}, "making_a_hero",
			map[string]any{"content": ""}, ""},
		{"words too long", map[string]any{"type": "table_agreement", "text": long}, nil, "", nil, "the words in a table_agreement block run over"},
		{"where we play", map[string]any{"type": "where_we_play", "foundry_url": " foundry.example.com ", "voice": "Discord, Ember Coast"}, nil, "where_we_play",
			map[string]any{"foundry_url": "foundry.example.com", "voice": "Discord, Ember Coast"}, ""},
		{"new voice keeps the address", map[string]any{"type": "where_we_play", "voice": "Mumble"},
			map[string]any{"foundry_url": "foundry.example.com", "voice": "Discord"}, "where_we_play",
			map[string]any{"foundry_url": "foundry.example.com", "voice": "Mumble"}, ""},
		{"empty address clears it", map[string]any{"type": "where_we_play", "foundry_url": ""},
			map[string]any{"foundry_url": "foundry.example.com", "voice": "Discord"}, "where_we_play",
			map[string]any{"voice": "Discord"}, ""},
		{"listed bare keeps both", "where_we_play", map[string]any{"foundry_url": "f.example.com", "voice": "Discord"}, "where_we_play",
			map[string]any{"foundry_url": "f.example.com", "voice": "Discord"}, ""},
		{"address too long", map[string]any{"type": "where_we_play", "foundry_url": strings.Repeat("a", maxWhereWePlayLine+1)}, nil, "", nil, "`foundry_url` runs over"},
		{"address on the wrong block", map[string]any{"type": "party", "foundry_url": "f.example.com"}, nil, "", nil, "only a where_we_play takes `foundry_url` and `voice`"},
		{"voice on the wrong block", map[string]any{"type": "table_agreement", "voice": "x"}, nil, "", nil, "only a where_we_play takes"},
		{"text on where we play", map[string]any{"type": "where_we_play", "text": "x"}, nil, "", nil, "only a text_block, a brochure, a table_agreement or a making_a_hero takes `text`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := playersFake()
			if tt.before != nil {
				f.homes["player"] = HomeLayout{Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{{ID: "keep", Type: tt.typ, Config: tt.before}}}}}}}
			}
			k := HomeKind{Svc: f}
			rec := homeRec(ActionUpdate, "players", []any{row(col(12, tt.block, "party"))})
			rec.Text = mdText{}
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
			if got.Type != tt.typ || len(got.Config) != len(tt.wantCfg) {
				t.Fatalf("block = %+v, want config %v", got, tt.wantCfg)
			}
			for key, want := range tt.wantCfg {
				if got.Config[key] != want {
					t.Errorf("config[%s] = %q, want %q", key, got.Config[key], want)
				}
			}
			if tt.before != nil && got.ID != "keep" {
				t.Errorf("id = %s, the block's own id should be kept", got.ID)
			}
		})
	}
}

// Changing only a where_we_play line is a change to the home; the same lines
// again are not.
func TestHomeKind_WhereWePlayChangeIsSeen(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	f := playersFake()
	f.homes["player"] = HomeLayout{Rows: []HomeRow{{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{
		{ID: "b", Type: "where_we_play", Config: map[string]any{"foundry_url": "f.example.com"}}}}}}}}
	k := HomeKind{Svc: f}
	same := homeRec(ActionUpdate, "players", []any{row(col(12, map[string]any{"type": "where_we_play", "foundry_url": "f.example.com"}))})
	if p := k.Plan(context.Background(), "c", owner, same); !strings.Contains(p.Error, "already has these blocks") {
		t.Errorf("same address: error = %q", p.Error)
	}
	changed := homeRec(ActionUpdate, "players", []any{row(col(12, map[string]any{"type": "where_we_play", "foundry_url": "g.example.com"}))})
	if p := k.Plan(context.Background(), "c", owner, changed); p.Error != "" {
		t.Errorf("new address: unexpected error %q", p.Error)
	}
}

// roundTripHome is a saved home holding every block whose words or settings
// a record can write.
func roundTripHome() HomeLayout {
	return HomeLayout{Rows: []HomeRow{
		{ID: "r1", Columns: []HomeColumn{{ID: "c1", Width: 12, Blocks: []HomeBlock{
			{ID: "b-banner", Type: "welcome_banner", Config: map[string]any{"style": "wide"}},
		}}}},
		{ID: "r2", Columns: []HomeColumn{
			{ID: "c2", Width: 7, Blocks: []HomeBlock{
				{ID: "b-text", Type: "text_block", Config: map[string]any{"content": "<p>Bring dice.</p>"}},
				{ID: "b-agree", Type: "table_agreement", Config: map[string]any{
					"content": "<h2>Safety and lines</h2><p>No gore, and <strong>ask first</strong>.</p><ul><li>Pause card</li><li>Veils</li></ul><h2>Pacing</h2><p>We stop at ten.</p>"}},
				{ID: "b-hero", Type: "making_a_hero", Config: map[string]any{"content": "Start at level 1.\n\nAsk me before you pick a homebrew class."}},
				{ID: "b-bro", Type: "brochure", Config: map[string]any{"entity_id": "page-1", "content": "Read this first."}},
			}},
			{ID: "c3", Width: 5, Blocks: []HomeBlock{
				{ID: "b-where", Type: "where_we_play", Config: map[string]any{"foundry_url": "foundry.example.com", "voice": "Discord, the Ember Coast channel"}},
				{ID: "b-empty-agree", Type: "table_agreement"},
				{ID: "b-party", Type: "party"},
			}},
		}},
	}}
}

// readableWords is the words of a block's stored content with the markup and
// the spacing gone, so two forms of the same words compare equal.
func readableWords(v any) string {
	s, _ := v.(string)
	return strings.Join(strings.Fields(textOf(s)), " ")
}

// What Export writes, parsed and applied again, gives back the same layout:
// the same rows, columns, widths, block types and ids, and the same settings
// and words. A block's markup may be re-rendered, so words compare as words.
func TestHomeKind_ExportRoundTrips(t *testing.T) {
	ctx := context.Background()
	owner := Actor{UserID: "own", Role: 3}
	f := &fakeHome{owner: "own", homes: map[string]HomeLayout{"player": roundTripHome()}}
	k := HomeKind{Svc: f, Pages: brochurePages()}
	out, err := k.Export(ctx, "c", owner)
	if err != nil {
		t.Fatal(err)
	}
	recs := parseBlocks(out)
	if len(recs) != 1 || recs[0].Kind != "home" {
		t.Fatalf("parsed %d records from:\n%s", len(recs), out)
	}
	rec := recs[0]
	rec.Action = ActionUpdate
	rec.Text = BodyText(nil)
	// The block is the same on screen, so the plan sees no change in its shape
	// only when no words changed; the words are re-rendered, which is a change.
	if p := k.Plan(ctx, "c", owner, rec); p.Error != "" && !strings.Contains(p.Error, "already has these blocks") {
		t.Fatalf("plan: %s\n%s", p.Error, out)
	}
	if err := k.Apply(ctx, "c", owner, rec); err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	want, got := roundTripHome(), f.saved["player"]
	if len(got.Rows) != len(want.Rows) {
		t.Fatalf("rows = %d, want %d\n%s", len(got.Rows), len(want.Rows), out)
	}
	for ri, wr := range want.Rows {
		for ci, wc := range wr.Columns {
			gc := got.Rows[ri].Columns[ci]
			if gc.Width != wc.Width || len(gc.Blocks) != len(wc.Blocks) {
				t.Fatalf("row %d column %d = width %d with %d blocks, want %d with %d", ri, ci, gc.Width, len(gc.Blocks), wc.Width, len(wc.Blocks))
			}
			for bi, wb := range wc.Blocks {
				gb := gc.Blocks[bi]
				if gb.Type != wb.Type || gb.ID != wb.ID {
					t.Errorf("block %d.%d.%d = %s %s, want %s %s", ri, ci, bi, gb.Type, gb.ID, wb.Type, wb.ID)
				}
				switch wb.Type {
				case "text_block", "table_agreement", "making_a_hero":
					if readableWords(gb.Config["content"]) != readableWords(wb.Config["content"]) {
						t.Errorf("%s words = %q, want %q", wb.Type, readableWords(gb.Config["content"]), readableWords(wb.Config["content"]))
					}
					if gb.Config["_md"] != nil {
						t.Errorf("%s kept its unrendered words", wb.Type)
					}
				default:
					if len(gb.Config) != len(wb.Config) {
						t.Errorf("%s config = %v, want %v", wb.Type, gb.Config, wb.Config)
					}
					for key, v := range wb.Config {
						if gb.Config[key] != v {
							t.Errorf("%s config[%s] = %v, want %v", wb.Type, key, gb.Config[key], v)
						}
					}
				}
			}
		}
	}
	// The agreement keeps its headings and list as markdown, not a run-on line.
	agree := got.Rows[1].Columns[0].Blocks[1].Config["content"].(string)
	for _, tag := range []string{"<h2>Safety and lines</h2>", "<li>Pause card</li>", "<strong>ask first</strong>", "<h2>Pacing</h2>"} {
		if !strings.Contains(agree, tag) {
			t.Errorf("the agreement lost %s:\n%s", tag, agree)
		}
	}
}

// An agreement a Director typed as plain text is exported as typed, and its
// line breaks survive the trip.
func TestHomeKind_ExportPlainAgreement(t *testing.T) {
	f := &fakeHome{owner: "own", homes: map[string]HomeLayout{"player": {Rows: []HomeRow{{ID: "r", Columns: []HomeColumn{{ID: "c", Width: 12, Blocks: []HomeBlock{
		{ID: "a", Type: "table_agreement", Config: map[string]any{"content": "Safety\nNo gore.\n\nPacing\nTen o'clock."}}}}}}}}}}
	out, err := HomeKind{Svc: f}.Export(context.Background(), "c", Actor{UserID: "own", Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	rec := parseBlocks(out)[0]
	blocks := rec.List("rows")[0].(map[string]any)["columns"].([]any)[0].(map[string]any)["blocks"].([]any)
	if got := blocks[0].(map[string]any)["text"]; got != "Safety\nNo gore.\n\nPacing\nTen o'clock." {
		t.Errorf("text = %q", got)
	}
}

func TestWrittenMarkdown(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain text as typed", "Safety\r\nNo gore.\n\nPacing", "Safety\nNo gore.\n\nPacing"},
		{"a stray less-than is plain", "a < b and c", "a < b and c"},
		{"paragraphs", "<p>One.</p><p>Two.</p>", "One.\n\nTwo."},
		{"headings", "<h2>Safety</h2><p>No gore.</p><h3>Veils</h3>", "## Safety\n\nNo gore.\n\n### Veils"},
		{"bold and italic keep their spaces", "<p>Bring <b>dice</b> and <em>snacks</em>.</p>", "Bring **dice** and *snacks*."},
		{"a bullet list", "<ul><li>Pause card</li><li>Veils</li></ul><p>After.</p>", "- Pause card\n- Veils\n\nAfter."},
		{"a numbered list", "<ol><li>One</li><li>Two</li></ol>", "1. One\n2. Two"},
		{"a nested list", "<ul><li>Top<ul><li>Inner</li></ul></li></ul>", "- Top\n  - Inner"},
		{"a link", `<p>See <a href="https://example.com/x">the wiki</a>.</p>`, "See [the wiki](https://example.com/x)."},
		{"an unsafe link keeps its words only", `<p><a href="javascript:alert(1)">click</a></p>`, "[click]"},
		{"a line break", "<p>One<br>Two</p>", "One  \nTwo"},
		{"unknown tags drop", "<section><p>Hi <span>there</span></p></section>", "Hi there"},
		{"entities decode", "<p>Fish &amp; chips</p>", "Fish & chips"},
		{"whitespace between tags is layout", "<p>One.</p>\n  <p>Two.</p>", "One.\n\nTwo."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := writtenMarkdown(tt.in); got != tt.want {
				t.Errorf("writtenMarkdown(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
