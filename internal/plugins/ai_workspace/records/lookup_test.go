package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// fakeJournal is what the viewer can open in the Journal; the real adapter
// gets it from the notes' own visibility rule.
type fakeJournal []JournalNote

func (f fakeJournal) JournalNotes(context.Context, string, permissions.Viewer) ([]JournalNote, error) {
	return f, nil
}

func strp(s string) *string { return &s }

// fakePages hides private pages from anyone below a co-DM, as the real
// role filter does.
type fakePages struct {
	all   []entities.Entity
	types []entities.EntityType
}

func (f *fakePages) List(_ context.Context, _ string, _ int, role int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	if o.Page > 1 {
		return nil, 0, nil
	}
	var out []entities.Entity
	for _, e := range f.all {
		if !e.IsPrivate || role >= 3 {
			out = append(out, e)
		}
	}
	return out, len(out), nil
}
func (f *fakePages) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return f.types, nil
}

type fakeRels struct{ rels []relations.Relation }

func (f *fakeRels) ListByEntity(_ context.Context, _, id string) ([]relations.Relation, error) {
	var out []relations.Relation
	for _, r := range f.rels {
		if r.SourceEntityID == id {
			out = append(out, r)
		}
	}
	return out, nil
}
func (f *fakeRels) Create(context.Context, string, string, string, string, string, string, json.RawMessage, ...bool) (*relations.Relation, error) {
	panic("lookups never write")
}
func (f *fakeRels) UpdateMetadata(context.Context, int, json.RawMessage) error {
	panic("lookups never write")
}
func (f *fakeRels) Delete(context.Context, int) error { panic("lookups never write") }

type fakeParty struct{ p Party }

func (f *fakeParty) Party(context.Context, string, Actor) (Party, error) { return f.p, nil }

func page(id, name, typ string, private bool) entities.Entity {
	return entities.Entity{ID: id, CampaignID: camp, Name: name, Slug: entities.Slugify(name), TypeName: typ, TypeNamePlural: typ + "s", IsPrivate: private}
}

func lookup(what string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["what"] = what
	r := Record{Kind: KindLookup, Fields: fields}
	if n, ok := fields["name"].(string); ok {
		r.Name = n
	}
	return r
}

func world() *Lookups {
	c := newCal()
	c.stored = []calendar.Event{
		{ID: "e1", Name: "Midwinter Feast", Year: 1492, Month: 1, Day: 21, Visibility: "everyone"},
		{ID: "e2", Name: "Secret Council", Year: 1492, Month: 1, Day: 12, Visibility: "dm_only"},
		{ID: "e3", Name: "Spring Fair", Year: 1492, Month: 2, Day: 3, Visibility: "everyone"},
	}
	label := "Blizzard"
	temp := -12.0
	c.days = []calendar.DayWeather{{Year: 1492, Month: 1, Day: 3, PresetLabel: &label, TemperatureCelsius: &temp}}
	return &Lookups{
		Cal:    c,
		Tables: &fakeTables{doc: TableDoc{Tables: []Table{{Name: "Tavern names", Entries: []Entry{{Name: "The Rusty Anchor"}, {Name: "The Drowned Rat", Brief: "smugglers"}}}}}},
		Pages: &fakePages{all: []entities.Entity{
			page("aldric", "Ser Aldric", "Character", false),
			page("ghost", "The Ghost", "Character", true),
			page("anchor", "The Rusty Anchor", "Location", false),
			page("rope", "Rope", "Item", false),
			page("map", "Treasure Map", "Item", true),
		}},
		Rels: &fakeRels{rels: []relations.Relation{
			{SourceEntityID: "anchor", TargetEntityID: "rope", TargetEntityName: "Rope", RelationType: "sells", Metadata: json.RawMessage(`{"price":1,"quantity":5,"currency":"gp"}`)},
			{SourceEntityID: "anchor", TargetEntityID: "map", TargetEntityName: "Treasure Map", RelationType: "sells"},
			{SourceEntityID: "aldric", TargetEntityID: "rope", TargetEntityName: "Rope", RelationType: "Has Item", Metadata: json.RawMessage(`{"quantity":2}`)},
			{SourceEntityID: "aldric", TargetEntityID: "anchor", TargetEntityName: "The Rusty Anchor", RelationType: "Has Item", DmOnly: true},
		}},
		Notes: &fakeNotes{notes: []notes.Note{
			{ID: "n1", CampaignID: camp, UserID: "owner", Title: "Who poisoned the duke?"},
			{ID: "n2", CampaignID: camp, UserID: "someone-else", Title: "Their plans", IsShared: true},
		}},
		Journal: fakeJournal{
			{ID: "n1", Title: "Who poisoned the duke?", AuthorID: "owner", Author: "Key", Audience: "private", Body: strp("<p>The <strong>cook</strong> had the means.</p>")},
			{ID: "n3", Title: "Session 12, 2026-10-09", AuthorID: "owner", Author: "Key", Folder: "Game nights", Night: "Session 12, 2026-10-09", Audience: "private", Body: strp("<p>Mine.</p>")},
			{ID: "n4", Title: "Session 12, 2026-10-09", AuthorID: "mira", Author: "Mira", Folder: "Game nights", Night: "Session 12, 2026-10-09", Audience: "gm", Body: strp("<p>We met @Ser Aldric.</p>")},
			{ID: "n5", Title: "Rope knots", AuthorID: "owner", PageID: "rope", Audience: "private"},
			{ID: "p1", Title: "Price check", AuthorID: "owner", PageID: "anchor", PlayerNote: true, Audience: "dm_scribe", Body: strp("<p>Too cheap.</p>")},
			{ID: "p2", Title: "Can I buy rope?", AuthorID: "mira", Author: "Mira", PageID: "anchor", PlayerNote: true, Audience: "dm_only"},
		},
		Party: &fakeParty{p: Party{
			Heroes: []PartyHero{
				{ID: "aldric", Name: "Ser Aldric", Player: "Mira", Subtitle: "Level 3 Fury", Meters: []PartyMeter{{Label: "Stamina", Current: "31", Max: "42"}}},
				{ID: "ghost", Name: "The Ghost", Player: "Sam"},
			},
			Night: &PartyNight{Name: "Session 12", When: "Fri 9 Oct", Answers: map[string]string{"Mira": "yes"}},
		}},
	}
}

func TestLookups_Answers(t *testing.T) {
	tests := []struct {
		name        string
		r           Record
		a           Actor
		want        []string
		notWant     []string
		wantErrText string
	}{
		{"events in a month", lookup("events", map[string]any{"from": "Hammer 1 1492", "to": "Hammer 30 1492"}), owner,
			[]string{"Midwinter Feast: Hammer 21 1492, everyone", "Secret Council", "Directors only"}, []string{"Spring Fair"}, ""},
		{"events hide Director-only in Safe", lookup("events", map[string]any{"year": 1492, "month": "Hammer"}), player,
			[]string{"Midwinter Feast"}, []string{"Secret Council"}, ""},
		{"events across months", lookup("events", map[string]any{"from": "1492-1-20", "to": "Alturiak 5 1492"}), owner,
			[]string{"Midwinter Feast", "Spring Fair"}, []string{"Secret Council"}, ""},
		{"events need a range", lookup("events", nil), owner, nil, nil, "it needs `from` and `to`"},
		{"events refuse a bad month", lookup("events", map[string]any{"from": "Smarch 1 1492"}), owner, nil, nil, "not a month"},
		{"weather", lookup("weather", map[string]any{"year": 1492, "month": 1}), owner, []string{"Hammer 3 1492: Blizzard, -12°C"}, nil, ""},
		{"table entries", lookup("table", map[string]any{"name": "tavern names"}), owner, []string{"The Drowned Rat: smugglers"}, nil, ""},
		{"tables stay out in Safe", lookup("table", nil), player, nil, nil, "left out while your AI sees what players see"},
		{"pages index", lookup("pages", nil), player, []string{"### Characters (1)", "Ser Aldric", "The Rusty Anchor"}, []string{"The Ghost", "Treasure Map"}, ""},
		{"pages of one type", lookup("pages", map[string]any{"type": "Location"}), owner, []string{"The Rusty Anchor"}, []string{"Ser Aldric"}, ""},
		{"a page", lookup("page", map[string]any{"name": "ser aldric"}), owner, []string{"## Page: Ser Aldric", "Has Item [Rope](#rope)"}, []string{"{#"}, ""},
		{"a hidden page is not found in Safe", lookup("page", map[string]any{"name": "The Ghost"}), player, nil, nil, "no page called"},
		{"shop stock", lookup("stock", map[string]any{"shop": "The Rusty Anchor"}), player, []string{"Rope: 1 gp, 5 in stock"}, []string{"Treasure Map"}, ""},
		{"inventory hides Director-only links in Safe", lookup("inventory", map[string]any{"character": "Ser Aldric"}), player, []string{"Rope x2"}, []string{"Rusty Anchor"}, ""},
		{"inventory shows them to a Director", lookup("inventory", map[string]any{"character": "Ser Aldric"}), owner, []string{"Rope x2", "The Rusty Anchor"}, nil, ""},
		{"notes", lookup("notes", nil), owner, []string{
			"## My Journal (2)", "- Who poisoned the duke? (private)",
			"- Session 12, 2026-10-09 (in the folder Game nights; game night entry for Session 12, 2026-10-09; private)",
			"## My jots on pages (1)", "- Rope knots (a jot on Rope; private)",
			"## My Player Notes on pages (1)", "- Price check (a Player Note on The Rusty Anchor; shared with the DM and Scribes)",
			"## Shared with me by other members (2)", "- Session 12, 2026-10-09 (by Mira; in the folder Game nights; game night entry for Session 12, 2026-10-09; shared with the DM)",
			"- Can I buy rope? (by Mira; a Player Note on The Rusty Anchor; shared with the DM)",
		}, nil, ""},
		{"my Player Note", lookup("note", map[string]any{"name": "price check"}), owner, []string{"a Player Note on The Rusty Anchor", "Too cheap.", "with a `player-note` block"}, nil, ""},
		{"someone else's Player Note", lookup("note", map[string]any{"name": "Can I buy rope?"}), owner, []string{"by Mira; a Player Note", "no block can change it"}, []string{"`player-note` block"}, ""},
		{"a note's text", lookup("note", map[string]any{"name": "who poisoned the duke?"}), owner, []string{"## Note: Who poisoned the duke?", "*private*", "The **cook** had the means."}, []string{"can't change it"}, ""},
		{"my entry before a shared one", lookup("note", map[string]any{"name": "Session 12, 2026-10-09"}), owner, []string{"Mine.", "2 notes have this title"}, []string{"Mira"}, ""},
		{"a shared entry by its writer", lookup("note", map[string]any{"name": "Session 12, 2026-10-09", "by": "mira"}), owner, []string{"by Mira", "We met @Ser Aldric.", "no block can change it"}, []string{"Mine."}, ""},
		{"a jot by its page", lookup("note", map[string]any{"name": "Rope knots", "page": "Rope"}), owner, []string{"a jot on Rope", "_(empty)_"}, nil, ""},
		{"a jot on another page is not found", lookup("note", map[string]any{"name": "Rope knots", "page": "Ser Aldric"}), owner, nil, nil, "no note called"},
		{"a note needs a name", lookup("note", nil), owner, nil, nil, "give the note's `name`"},
		{"a note stays out in Safe", lookup("note", map[string]any{"name": "Who poisoned the duke?"}), player, nil, nil, "your notes are left out"},
		{"notes stay out in Safe", lookup("notes", nil), player, nil, nil, "your notes are left out"},
		{"players, live", lookup("players", nil), owner, []string{"Ser Aldric (Mira): Level 3 Fury. Stamina 31 of 42. Carries Rope x2, The Rusty Anchor. Coming to the next game night.", "The Ghost (Sam)", "Next game night: Session 12"}, nil, ""},
		{"players in Safe leave out the DM Screen's figures", lookup("players", nil), player, []string{"Ser Aldric (Mira): Level 3 Fury. Carries Rope x2. Coming to the next game night."}, []string{"The Ghost", "Stamina"}, ""},
		{"game-system entry fields not wired", lookup("system-entries", nil), owner, nil, nil, "can't be looked up on this server yet"},
		{"game-system entries not wired", lookup("system-entries", map[string]any{"field": "ancestry"}), owner, nil, nil, "can't be looked up on this server yet"},
		{"unknown what", lookup("secrets", nil), owner, nil, nil, "not something Chronicle can look up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := world().Answer(context.Background(), camp, tt.a, []Record{tt.r})
			if len(ans) != 1 {
				t.Fatalf("%d answers", len(ans))
			}
			got := ans[0]
			if tt.wantErrText != "" {
				if !strings.Contains(got.Error, tt.wantErrText) {
					t.Fatalf("error = %q, want %q", got.Error, tt.wantErrText)
				}
				return
			}
			if got.Error != "" {
				t.Fatalf("unexpected error %q", got.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, got.Text)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("answer leaks %q:\n%s", w, got.Text)
				}
			}
		})
	}
}

func TestLookups_Caps(t *testing.T) {
	var recs []Record
	for i := 0; i < maxLookups+2; i++ {
		recs = append(recs, lookup("notes", nil))
	}
	ans := world().Answer(context.Background(), camp, owner, recs)
	if ans[maxLookups].Error == "" || ans[maxLookups-1].Error != "" {
		t.Fatalf("lookup %d answered past the cap", maxLookups+1)
	}
	if got := capText(strings.Repeat("line\n", 10000), 100); len(got) > 160 || !strings.Contains(got, "cut here") {
		t.Fatalf("capText did not cut: %d bytes", len(got))
	}
	if got := capText(strings.Repeat("é", 100), 51); !utf8.ValidString(got) {
		t.Fatal("capText split a character")
	}
	long := world().Answer(context.Background(), camp, owner, []Record{lookup(strings.Repeat("x", 10000), nil)})
	if len(long[0].Chip) > 90 || len(long[0].Error) > 310 {
		t.Fatalf("echoed input not capped: %d, %d", len(long[0].Chip), len(long[0].Error))
	}
}

func TestLookups_Unwired(t *testing.T) {
	ans := (&Lookups{}).Answer(context.Background(), camp, owner, []Record{lookup("players", nil), lookup("events", map[string]any{"year": 1})})
	for _, a := range ans {
		if !strings.Contains(a.Error, "on this server") {
			t.Fatalf("unwired lookup answered %+v", a)
		}
	}
	if md := Markdown(ans); !strings.Contains(md, "Not answered:") {
		t.Fatalf("markdown hides the failures: %s", md)
	}
}

func TestLookups_PageIndexFollowsPrivacy(t *testing.T) {
	idx := world().PageIndex(context.Background(), camp, player)
	if !strings.Contains(idx, "**Characters** (1): Ser Aldric") || strings.Contains(idx, "Ghost") {
		t.Fatalf("index:\n%s", idx)
	}
}

func TestParseDateText(t *testing.T) {
	cal := &newCal().cal
	tests := []struct {
		in      string
		want    calDate
		wantErr bool
	}{
		{"Hammer 21 1492", calDate{1492, 1, 21}, false},
		{"21 Hammer 1492", calDate{1492, 1, 21}, false},
		{"Alturiak 3, 1492", calDate{1492, 2, 3}, false},
		{"1492-2-3", calDate{1492, 2, 3}, false},
		{"Hammer 31 1492", calDate{}, true},
		{"Hammer 1492", calDate{}, true},
		{"next Tuesday", calDate{}, true},
	}
	for _, tt := range tests {
		got, err := parseDateText(cal, tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("%q: got %v, %v", tt.in, got, err)
		}
	}
}

// An event to change is found through the role-filtered list: the
// operator finds what they can see, and a player never finds a hidden one.
func TestEventKind_FindsEventToChange(t *testing.T) {
	c := newCal()
	c.stored = []calendar.Event{
		{ID: "e1", Name: "Midwinter Feast", Year: 1492, Month: 1, Day: 21},
		{ID: "e2", Name: "Secret Council", Year: 1492, Month: 1, Day: 12, Visibility: "dm_only"},
	}
	k := EventKind{Svc: c}
	if p := k.Plan(context.Background(), camp, owner, rec("event", ActionUpdate, "Secret Council", nil, "")); p.Error != "" {
		t.Fatalf("owner: %s", p.Error)
	}
	if p := k.Plan(context.Background(), camp, player, rec("event", ActionDelete, "Secret Council", nil, "")); p.Error == "" {
		t.Fatal("a player found a hidden event to remove")
	}
}

func TestLookups_PageIndexShowsVisibleParentOnly(t *testing.T) {
	ghost, anchor, rope := "ghost", "anchor", "rope"
	mk := func(id, name, typ string, private bool, parent *string) entities.Entity {
		e := page(id, name, typ, private)
		e.ParentID = parent
		return e
	}
	l := &Lookups{Pages: &fakePages{all: []entities.Entity{
		mk("anchor", "The Rusty Anchor", "Location", false, nil),
		mk("ghost", "The Ghost", "Location", true, nil),
		mk("cellar", "Cellar", "Location", false, &anchor),
		mk("crypt", "Crypt", "Location", false, &ghost),
		mk("rope", "Rope", "Item", false, nil),
		mk("knot", "Knot", "Item", false, &rope),
	}}}
	tests := []struct {
		name    string
		a       Actor
		want    []string
		notWant []string
	}{
		{"player sees visible parent", player, []string{"Cellar (in The Rusty Anchor)", "Knot (in Rope)"}, []string{"Ghost"}},
		{"hidden parent is not named", player, []string{"Crypt"}, []string{"Crypt (in", "Ghost"}},
		{"owner sees the hidden parent", owner, []string{"Crypt (in The Ghost)"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := l.PageIndex(context.Background(), camp, tt.a)
			for _, w := range tt.want {
				if !strings.Contains(idx, w) {
					t.Errorf("missing %q in:\n%s", w, idx)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(idx, w) {
					t.Errorf("unexpected %q in:\n%s", w, idx)
				}
			}
		})
	}
}

// A page's sheet comes back as a `fields:` block the AI can edit and send
// back, with GM-only values left out for a player.
func TestLookups_PageSheet(t *testing.T) {
	brute := page("brute", "Orc Brute", "Creature", false)
	brute.EntityTypeID = 7
	brute.FieldsData = map[string]any{"level": 3, "stamina": 30, "abilities_json": `[{"name":"Spear"}]`, "book_source": "Monsters p. 12", "faction": ""}
	l := world()
	l.Pages = &fakePages{all: []entities.Entity{brute}, types: []entities.EntityType{{ID: 7, Name: "Creature", Fields: []entities.FieldDefinition{
		{Key: "level", Label: "Level", Type: "number"},
		{Key: "stamina", Label: "Stamina", Type: "number"},
		{Key: "faction", Label: "Faction", Type: "text"},
		{Key: "abilities_json", Label: "Abilities", Type: "textarea"},
		{Key: "book_source", Label: "Book source", Type: "text", GMOnly: true},
	}}}}
	for _, tc := range []struct {
		name    string
		a       Actor
		want    []string
		notWant []string
	}{
		{"director", owner, []string{"### Sheet", "fields:\n  level: 3\n  stamina: 30\n  abilities_json: '[{\"name\":\"Spear\"}]'\n  book_source: Monsters p. 12\n"}, []string{"faction"}},
		{"player", player, []string{"### Sheet", "level: 3"}, []string{"book_source", "Monsters p. 12"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ans := l.Answer(context.Background(), camp, tc.a, []Record{lookup("page", map[string]any{"name": "Orc Brute"})})
			if ans[0].Error != "" {
				t.Fatal(ans[0].Error)
			}
			for _, w := range tc.want {
				if !strings.Contains(ans[0].Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, ans[0].Text)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(ans[0].Text, w) {
					t.Errorf("answer leaks %q:\n%s", w, ans[0].Text)
				}
			}
		})
	}
}
