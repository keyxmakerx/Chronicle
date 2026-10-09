package records

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/notes"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// fakePages hides private pages from anyone below a co-DM, as the real
// role filter does.
type fakePages struct{ all []entities.Entity }

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
	return nil, nil
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
		{"tables stay out in Safe", lookup("table", nil), player, nil, nil, "left out in Safe"},
		{"pages index", lookup("pages", nil), player, []string{"### Characters (1)", "Ser Aldric", "The Rusty Anchor"}, []string{"The Ghost", "Treasure Map"}, ""},
		{"pages of one type", lookup("pages", map[string]any{"type": "Location"}), owner, []string{"The Rusty Anchor"}, []string{"Ser Aldric"}, ""},
		{"a page", lookup("page", map[string]any{"name": "ser aldric"}), owner, []string{"## Page: Ser Aldric", "Has Item [Rope](#rope)"}, []string{"{#"}, ""},
		{"a hidden page is not found in Safe", lookup("page", map[string]any{"name": "The Ghost"}), player, nil, nil, "no page called"},
		{"shop stock", lookup("stock", map[string]any{"shop": "The Rusty Anchor"}), player, []string{"Rope: 1 gp, 5 in stock"}, []string{"Treasure Map"}, ""},
		{"inventory hides Director-only links in Safe", lookup("inventory", map[string]any{"character": "Ser Aldric"}), player, []string{"Rope x2"}, []string{"Rusty Anchor"}, ""},
		{"inventory shows them to a Director", lookup("inventory", map[string]any{"character": "Ser Aldric"}), owner, []string{"Rope x2", "The Rusty Anchor"}, nil, ""},
		{"only my notes", lookup("notes", nil), owner, []string{"Who poisoned the duke? (Journal)"}, []string{"Their plans"}, ""},
		{"players, live", lookup("players", nil), player, []string{"Ser Aldric (Mira): Level 3 Fury. Stamina 31 of 42. Carries Rope x2. Coming to the next game night.", "Next game night: Session 12"}, []string{"The Ghost"}, ""},
		{"game-system entries need a field", lookup("system-entries", nil), owner, nil, nil, "needs `field:`"},
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
