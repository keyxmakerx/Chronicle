package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

func pick(label, from, to string) entities.HeroPickChange {
	return entities.HeroPickChange{FieldKey: strings.ToLower(label), Label: label, From: from, To: to, Kind: entities.HeroChangePick}
}

func list(label string, was, now []string) entities.HeroPickChange {
	return entities.HeroPickChange{FieldKey: strings.ToLower(label), Label: label, Kind: entities.HeroChangeList, WasNames: was, NowNames: now}
}

func story(label string) entities.HeroPickChange {
	return entities.HeroPickChange{FieldKey: strings.ToLower(label), Label: label, Kind: entities.HeroChangeStory}
}

func names(s ...string) []string { return s }

// Each kind of change is worded on its own: the first old value and the
// newest for a pick or a text, added and removed for a list, and never the
// words of a story.
func TestHeroChangeDetail_Wording(t *testing.T) {
	cases := []struct {
		name string
		in   []entities.HeroPickChange
		want string
	}{
		{"a pick", []entities.HeroPickChange{pick("Class", "Fury", "Tactician")}, "Class: Fury → Tactician"},
		{"a cleared value reads nothing", []entities.HeroPickChange{pick("Kit", "Panther", "")}, "Kit: Panther → nothing"},
		{"a short text", []entities.HeroPickChange{{FieldKey: "pronouns", Label: "Pronouns", From: "she/her", To: "they/them", Kind: entities.HeroChangeText}}, "Pronouns: she/her → they/them"},
		{"a list adds and removes", []entities.HeroPickChange{list("Skills", names("Sneak", "Pick Lock"), names("Sneak", "Alchemy"))}, "Skills: added Alchemy and removed Pick Lock"},
		{"a list that only adds", []entities.HeroPickChange{list("Languages", names("Elvish"), names("Elvish", "Dwarvish", "Sylvan"))}, "Languages: added Dwarvish, Sylvan"},
		{"a list that only removes", []entities.HeroPickChange{list("Perks", names("Lucky", "Tough"), names("Lucky"))}, "Perks: removed Tough"},
		{"a list that is cleared", []entities.HeroPickChange{list("Skills", names("Sneak"), nil)}, "Skills: removed Sneak"},
		{"four names a side are all named", []entities.HeroPickChange{list("Skills", names("a", "b", "c", "d"), names("e", "f", "g", "h"))}, "Skills: added e, f, g, h and removed a, b, c, d"},
		{"more than four a side cap with a count", []entities.HeroPickChange{list("Skills", names("a", "b", "c", "d", "e", "f"), names("g", "h", "i", "j", "k"))},
			"Skills: added g, h, i, j and 1 more and removed a, b, c, d and 2 more"},
		{"a story is never quoted", []entities.HeroPickChange{story("Backstory")}, "Backstory rewritten"},
		{"three parts then a count", []entities.HeroPickChange{pick("A", "1", "2"), pick("B", "1", "2"), story("Backstory"), pick("D", "1", "2")},
			"A: 1 → 2; B: 1 → 2; Backstory rewritten; and 1 more"},
		{"five parts", []entities.HeroPickChange{pick("A", "1", "2"), pick("B", "1", "2"), pick("C", "1", "2"), pick("D", "1", "2"), pick("E", "1", "2")},
			"A: 1 → 2; B: 1 → 2; C: 1 → 2; and 2 more"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := heroChangeDetail(tc.in); got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// A burst stays one row: each field shows its first old value and its newest,
// a field that ends where it began drops out, and a row with nothing left
// says so.
func TestFoldHeroChanges(t *testing.T) {
	detail := func(kept string, steps ...[]entities.HeroPickChange) (string, string) {
		for _, s := range steps {
			kept = encodeHeroFold(foldHeroChanges(kept, s))
		}
		return heroFoldDetail(decodeHeroFold(kept)), kept
	}
	one := func(c ...entities.HeroPickChange) []entities.HeroPickChange { return c }
	cases := []struct {
		name  string
		steps [][]entities.HeroPickChange
		want  string
	}{
		{"first and newest value",
			[][]entities.HeroPickChange{one(pick("Class", "Fury", "Tactician")), one(pick("Class", "Tactician", "Shadow"))},
			"Class: Fury → Shadow"},
		{"put back drops out",
			[][]entities.HeroPickChange{one(pick("Class", "Fury", "Tactician")), one(pick("Class", "Tactician", "fury"))},
			"Back as it was"},
		{"put back leaves the others",
			[][]entities.HeroPickChange{
				one(pick("Class", "Fury", "Tactician"), pick("Kit", "Panther", "Wolf")),
				one(pick("Class", "Tactician", "Fury"))},
			"Kit: Panther → Wolf"},
		{"a field after a drop-out starts again",
			[][]entities.HeroPickChange{
				one(pick("Class", "Fury", "Tactician")), one(pick("Class", "Tactician", "Fury")), one(pick("Class", "Fury", "Shadow"))},
			"Class: Fury → Shadow"},
		{"a list is folded against the names at the start",
			[][]entities.HeroPickChange{
				one(list("Skills", names("Sneak"), names("Sneak", "Alchemy"))),
				one(list("Skills", names("Sneak", "Alchemy"), names("Alchemy", "Lore")))},
			"Skills: added Alchemy, Lore and removed Sneak"},
		{"a list added then removed again drops out",
			[][]entities.HeroPickChange{
				one(list("Skills", names("Sneak"), names("Sneak", "Alchemy"))),
				one(list("Skills", names("Sneak", "Alchemy"), names("sneak")))},
			"Back as it was"},
		{"a story stays rewritten",
			[][]entities.HeroPickChange{one(story("Backstory")), one(story("Backstory"))},
			"Backstory rewritten"},
		{"kinds fold side by side",
			[][]entities.HeroPickChange{
				one(pick("Class", "Fury", "Shadow")), one(story("Backstory")), one(list("Skills", names("A"), names("B")))},
			"Class: Fury → Shadow; Backstory rewritten; Skills: added B and removed A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := detail("", tc.steps...)
			if got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
	// A state that can't be read starts over rather than failing the save.
	if got, _ := detail("not json", one(pick("Class", "Fury", "Shadow"))); got != "Class: Fury → Shadow" {
		t.Fatalf("a broken state: %q", got)
	}
	// The kept state never holds story text: the changes don't carry any.
	_, kept := detail("", one(story("Backstory")))
	if strings.Contains(kept, "tale") {
		t.Fatalf("the state holds story text: %s", kept)
	}
}

// Through the bell: the Director keeps one unread row per hero, folded; once
// read, the next change starts a fresh one; every Director keeps their own
// row and the player never gets one.
func TestHeroChangeBell_OneRowPerHero(t *testing.T) {
	bell := &fakeBell{}
	b := heroChangeBell{camps: fakeHeroCamps{}, bell: bell}
	tess := &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tess"}
	ring := func(e *entities.Entity, c ...entities.HeroPickChange) {
		t.Helper()
		if err := b.HeroChanged(context.Background(), e, "pl", c); err != nil {
			t.Fatal(err)
		}
	}
	ring(tess, pick("Class", "Fury", "Tactician"))
	ring(tess, pick("Class", "Tactician", "Shadow"))
	ring(tess, story("Backstory"))
	for _, dir := range []string{"own", "codm"} {
		rows := bell.unread(dir, notifHeroChanged)
		if len(rows) != 1 {
			t.Fatalf("%s has %d unread rows, want one", dir, len(rows))
		}
		if rows[0].message != "Ana changed Tess" || rows[0].detail != "Class: Fury → Shadow; Backstory rewritten" {
			t.Fatalf("%s: %q / %q", dir, rows[0].message, rows[0].detail)
		}
		if rows[0].link != "/campaigns/c1/entities/e1" {
			t.Fatalf("link %q", rows[0].link)
		}
	}
	for _, other := range []string{"pl", "pl2", "scr"} {
		if len(bell.unread(other, notifHeroChanged)) != 0 {
			t.Fatalf("%s was rung", other)
		}
	}
	// A second hero is its own row.
	ring(&entities.Entity{ID: "e2", CampaignID: "c1", Name: "Wren"}, pick("Class", "Fury", "Shadow"))
	if len(bell.unread("own", notifHeroChanged)) != 2 {
		t.Fatalf("one row per hero, got %d", len(bell.unread("own", notifHeroChanged)))
	}
	// The owner reads theirs; the next change starts a fresh row for them, and
	// folds on into the Co-DM's, who has not read.
	for _, r := range bell.rows {
		if r.user == "own" && r.link == "/campaigns/c1/entities/e1" {
			r.read = true
		}
	}
	ring(tess, pick("Class", "Shadow", "Censor"))
	var own, codm *foldedRow
	for _, r := range bell.unread("own", notifHeroChanged) {
		if r.link == "/campaigns/c1/entities/e1" {
			own = r
		}
	}
	for _, r := range bell.unread("codm", notifHeroChanged) {
		if r.link == "/campaigns/c1/entities/e1" {
			codm = r
		}
	}
	if own == nil || own.detail != "Class: Shadow → Censor" {
		t.Fatalf("a read row starts a fresh one: %+v", own)
	}
	if codm == nil || codm.detail != "Class: Fury → Censor; Backstory rewritten" {
		t.Fatalf("each Director folds their own: %+v", codm)
	}
}

// The level bell is its own row each time and is not folded.
func TestHeroChangeBell_LevelNotFolded(t *testing.T) {
	bell := &fakeBell{}
	b := heroChangeBell{camps: fakeHeroCamps{}, bell: bell}
	if err := b.LevelRaised(context.Background(), &entities.Entity{ID: "e1", CampaignID: "c1", Name: "Tess"}, "pl", 3); err != nil {
		t.Fatal(err)
	}
	if bell.ntype != notifHeroLevelRaised || len(bell.rows) != 0 {
		t.Fatalf("type %q, folded rows %d", bell.ntype, len(bell.rows))
	}
}

// bellRules is a campaign whose hero picks are shut.
type shutRules struct{}

func (shutRules) HeroChangeRule(context.Context, string) (*campaigns.HeroChangeRule, error) {
	return &campaigns.HeroChangeRule{AfterMakingDays: 3}, nil
}

func (shutRules) SaveHeroChangeRule(context.Context, string, *campaigns.HeroChangeRule) error {
	return nil
}

// bellSheetEntities is the sheets door's entities with a type that has the
// owner keys a list, a story and an identity text need.
type bellSheetEntities struct{ *sheetEntities }

func (bellSheetEntities) GetEntityTypeByID(context.Context, int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: 1, Name: "Character", Fields: []entities.FieldDefinition{
		{Key: "class"}, {Key: "stamina"}, {Key: "ancestry"}, {Key: "pronouns"}, {Key: "backstory", Label: "Backstory"},
		{Key: "skills_json", Label: "Skills Data"}, {Key: "secret_past", GMOnly: true},
	}}, nil
}

type recordedBell struct{ rang [][]entities.HeroPickChange }

func (r *recordedBell) HeroChanged(_ context.Context, _ *entities.Entity, _ string, c []entities.HeroPickChange) error {
	r.rang = append(r.rang, c)
	return nil
}

func (r *recordedBell) LevelRaised(context.Context, *entities.Entity, string, int) error { return nil }

// A player's AI rings for a list change and a story change, stores a list
// while the picks are shut, refuses a locked pick as before, and never names
// a GM-only key.
func TestAIPlayerSheets_PatchSheetBell(t *testing.T) {
	cases := []struct {
		name      string
		shut      bool
		user      string
		patch     map[string]any
		ok        bool
		wantBells int
	}{
		{"a list change", false, "player-1", map[string]any{"skills_json": `["Sneak","Alchemy"]`}, true, 1},
		{"a list change while picks are shut", true, "player-1", map[string]any{"skills_json": `["Sneak","Alchemy"]`}, true, 1},
		{"a story change while picks are shut", true, "player-1", map[string]any{"backstory": "A new tale"}, true, 1},
		{"a short text", true, "player-1", map[string]any{"pronouns": "they/them"}, true, 1},
		{"a changed identity text rings alone", true, "player-1", map[string]any{"ancestry": "Dwarf"}, true, 1},
		{"a locked pick is refused and nothing rings", true, "player-1", map[string]any{"class": "Tactician", "skills_json": `["A"]`}, false, 0},
		{"a GM-only key is refused", true, "player-1", map[string]any{"secret_past": "x"}, false, 0},
		{"someone else's hero rings nothing", false, "player-2", map[string]any{"skills_json": `["A"]`}, false, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ents := newSheetEntities()
			ents.ents["tyne"].CreatedAt = time.Now().Add(-90 * 24 * time.Hour)
			ents.ents["tyne"].FieldsData = map[string]any{"class": "Fury", "ancestry": "Elf", "skills_json": `["Sneak"]`, "backstory": "Old tale", "pronouns": "she/her"}
			bell := &recordedBell{}
			var rules entities.HeroChangeRules
			if tc.shut {
				rules = shutRules{}
			}
			door := aiPlayerSheets{entities: bellSheetEntities{ents}, planner: sheetPlanner{},
				heroChanges: entities.NewHeroChangeService(rules, nil, bell)}
			err := door.PatchSheet(context.Background(), "camp-1", tc.user, "tyne", tc.patch, "Claude", true)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v", err, tc.ok)
			}
			if !tc.ok {
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Code >= 500 || len(ents.merged) != 0 {
					t.Fatalf("a refusal must be plain and store nothing: %v, %v", err, ents.merged)
				}
			}
			if len(bell.rang) != tc.wantBells {
				t.Fatalf("bells = %d, want %d", len(bell.rang), tc.wantBells)
			}
			for _, ch := range bell.rang {
				for _, c := range ch {
					if c.FieldKey == "secret_past" || c.FieldKey == "class" {
						t.Fatalf("named %q", c.FieldKey)
					}
				}
			}
		})
	}
}
