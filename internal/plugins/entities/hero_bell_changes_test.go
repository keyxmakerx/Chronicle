package entities

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// bellType is a hero's category as the bell sees it: the owner keys, a pick,
// a number, and one identity key the type marks GM-only.
func bellType() []FieldDefinition {
	return []FieldDefinition{
		{Key: "class", Label: "Class"}, {Key: "pronouns", Label: "Pronouns"}, {Key: "gender"},
		{Key: "backstory", Label: "Backstory"}, {Key: "appearance", Label: "Looks"},
		{Key: "skills_json", Label: "Skills Data"}, {Key: "languages_json"}, {Key: "perks_json"},
		{Key: "stamina"}, {Key: "background", GMOnly: true}, {Key: "gm_notes", GMOnly: true},
	}
}

func bellSummary(changes []HeroPickChange) []string {
	var out []string
	for _, c := range changes {
		out = append(out, string(c.Kind)+" "+c.Label+" "+c.From+">"+c.To+" +"+strings.Join(c.NowNames, ",")+" -"+strings.Join(c.WasNames, ","))
	}
	return out
}

// What rings is every key a claimed owner may write whose stored value is not
// blank and differs; filling a blank rings nothing, for every kind.
func TestHeroBellChanges(t *testing.T) {
	picks := builderPicks{"class": {}}
	labels := map[string]string{"class": "Class"}
	stored := map[string]any{
		"class": "Fury", "pronouns": "she/her", "gender": "", "backstory": "Born in Wode.", "appearance": "",
		"skills_json": `["Sneak","Pick Lock"]`, "languages_json": "[]", "perks_json": `[{"name":"Lucky","description":"Once a day"}]`,
		"stamina": float64(20), "background": "Noble", "gm_notes": "secret",
	}
	cases := []struct {
		name  string
		patch map[string]any
		want  string // each entry "kind Label from>to +now -was", joined with |
	}{
		{"a pick changed", map[string]any{"class": "Shadow"}, "pick Class Fury>Shadow + -"},
		{"a short text changed", map[string]any{"pronouns": "they/them"}, "text Pronouns she/her>they/them + -"},
		{"a short text in another case is the same", map[string]any{"pronouns": " SHE/her "}, ""},
		{"a short text cleared", map[string]any{"pronouns": nil}, "text Pronouns she/her> + -"},
		{"filling a blank text", map[string]any{"gender": "woman"}, ""},
		{"a story rewritten never carries its words", map[string]any{"backstory": "Raised by wolves."}, "story Backstory > + -"},
		{"a story trimmed is the same", map[string]any{"backstory": "  Born in Wode.\n"}, ""},
		{"a story that differs only in case rings", map[string]any{"backstory": "born in wode."}, "story Backstory > + -"},
		{"a story cleared rings", map[string]any{"backstory": nil}, "story Backstory > + -"},
		{"filling a blank story", map[string]any{"appearance": "Tall"}, ""},
		{"a list gains and loses names", map[string]any{"skills_json": `["Sneak","Alchemy"]`}, "list Skills > +Sneak,Alchemy -Sneak,Pick Lock"},
		{"a list in another order and case is the same", map[string]any{"skills_json": `[" pick lock","SNEAK"]`}, ""},
		{"a list written as a decoded array", map[string]any{"skills_json": []any{"Sneak", "Pick Lock"}}, ""},
		{"a list of objects compares by name", map[string]any{"skills_json": []any{map[string]any{"name": "Sneak", "x": 1}, map[string]any{"name": "Pick Lock"}}}, ""},
		{"a change only to an object's description is not a change", map[string]any{"perks_json": []any{map[string]any{"name": "Lucky", "description": "Twice a day"}}}, ""},
		{"a list cleared with null", map[string]any{"skills_json": nil}, "list Skills > + -Sneak,Pick Lock"},
		{"a list cleared with []", map[string]any{"skills_json": "[]"}, "list Skills > + -Sneak,Pick Lock"},
		{"filling a blank list", map[string]any{"languages_json": `["Elvish"]`}, ""},
		{"a list that is not a list is no change", map[string]any{"skills_json": "Sneak, Alchemy"}, ""},
		{"a perk renamed", map[string]any{"perks_json": `["Fortunate"]`}, "list Perks > +Fortunate -Lucky"},
		{"a GM-only key is never named", map[string]any{"background": "Outlaw", "gm_notes": "changed"}, ""},
		{"a key the player may not write is never named", map[string]any{"stamina": float64(99)}, ""},
		{"several, in key order", map[string]any{"pronouns": "he/him", "class": "Shadow", "backstory": "x"},
			"story Backstory > + -|pick Class Fury>Shadow + -|text Pronouns she/her>he/him + -"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Entity{FieldsData: stored}
			got := strings.Join(bellSummary(heroBellChanges(e, tc.patch, picks, labels, bellType())), "|")
			if got != tc.want {
				t.Fatalf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// The story key's label is the type's, else the key with a capital.
func TestHeroBellChanges_Labels(t *testing.T) {
	e := &Entity{FieldsData: map[string]any{"appearance": "Tall", "pronouns": "she/her", "culture": "Wode"}}
	got := heroBellChanges(e, map[string]any{"appearance": "Short", "pronouns": "he/him", "culture": "Other"}, nil, nil,
		[]FieldDefinition{{Key: "appearance", Label: "Looks"}, {Key: "pronouns"}, {Key: "culture"}})
	var labels []string
	for _, c := range got {
		labels = append(labels, c.Label)
	}
	if strings.Join(labels, ",") != "Looks,Culture,Pronouns" {
		t.Fatalf("labels %v", labels)
	}
	// An unknown type limits nothing but the owner keys themselves.
	if got := heroBellChanges(e, map[string]any{"appearance": "Short"}, nil, nil, nil); len(got) != 1 {
		t.Fatalf("unknown type: %v", got)
	}
}

// A GM-only key never reaches the bell, even where a door let the write
// through; neither does a key the type lacks.
func TestHeroBellChanges_NeverNamesGMOnly(t *testing.T) {
	e := &Entity{FieldsData: map[string]any{"background": "Noble", "backstory": "Old", "pronouns": "she/her"}}
	fields := []FieldDefinition{{Key: "background", GMOnly: true}, {Key: "backstory", GMOnly: true}, {Key: "pronouns"}}
	got := heroBellChanges(e, map[string]any{"background": "Outlaw", "backstory": "New", "pronouns": "he/him", "skills_json": `["A"]`}, nil, nil, fields)
	if len(got) != 1 || got[0].FieldKey != "pronouns" {
		t.Fatalf("only the player-visible key rings, got %v", got)
	}
}

// Foundry resends the whole mapped set. A second identical push rings
// nothing for any kind of key, with realistic values: a list of objects for
// perks, a long text for the backstory, in whatever form the first push stored.
func TestHeroBellChanges_ResendRingsNothing(t *testing.T) {
	long := strings.Repeat("The ash fell on Wode for nine winters. ", 120) + "\nShe left at dawn, unseen."
	push := func() map[string]any {
		// A fresh decode each time, as each request body would be.
		var p, s any
		_ = json.Unmarshal([]byte(`[{"name":"Lucky","description":"Once a day, \"reroll\" a die."},{"name":"Tough","description":"+2 stamina"}]`), &p)
		_ = json.Unmarshal([]byte(`["Sneak","Pick Lock"]`), &s)
		return map[string]any{
			"class": "Fury", "pronouns": "she/her", "backstory": long, "appearance": "Tall, scarred",
			"perks_json": p, "skills_json": s, "languages_json": `["Elvish"]`, "stamina": float64(20),
		}
	}
	// The first push is stored exactly as sent.
	e := &Entity{FieldsData: push()}
	picks := builderPicks{"class": {}}
	if got := heroBellChanges(e, push(), picks, nil, bellType()); len(got) != 0 {
		t.Fatalf("an identical second push rang: %v", bellSummary(got))
	}
	// The same set with the lists held as text still rings nothing.
	again := push()
	again["perks_json"] = `[{"name":"Lucky","description":"x"},{"name":"Tough"}]`
	again["skills_json"] = ` ["pick lock", "Sneak"] `
	if got := heroBellChanges(e, again, picks, nil, bellType()); len(got) != 0 {
		t.Fatalf("the same names as text rang: %v", bellSummary(got))
	}
	// And one real change in the same push still rings, alone.
	changed := push()
	changed["backstory"] = long + " Then she returned."
	got := heroBellChanges(e, changed, picks, nil, bellType())
	if len(got) != 1 || got[0].FieldKey != "backstory" || got[0].From != "" || got[0].To != "" {
		t.Fatalf("one change, no words: %v", bellSummary(got))
	}
}

// A list change while the hero's picks are shut is stored and rings; a locked
// pick in the same patch is refused as before and nothing rings.
func TestCheckPicks_ListChangeWhilePicksShut(t *testing.T) {
	shut := &campaigns.HeroChangeRule{AfterMakingDays: 3}
	made := heroNow.Add(-30 * 24 * time.Hour)
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Shadow"}}}}}
	e := &Entity{ID: "e1", CampaignID: "c1", Name: "Tess", CreatedAt: made, FieldsData: map[string]any{
		"class": "Fury", "skills_json": `["Sneak"]`, "backstory": "Old tale", "pronouns": "she/her"}}
	picks := builderPicksFrom(plan)
	s, _, _ := newTestHeroChanges(shut)

	for _, patch := range []map[string]any{
		{"skills_json": `["Sneak","Alchemy"]`},
		{"backstory": "A new tale"},
		{"pronouns": "they/them"},
	} {
		changes, err := s.CheckPicks(context.Background(), e, patch, picks, plan, bellType())
		if err != nil || len(changes) != 1 {
			t.Fatalf("%v: a non-pick change is never locked: changes=%v err=%v", patch, changes, err)
		}
	}
	if _, err := s.CheckPicks(context.Background(), e, map[string]any{"class": "Shadow", "skills_json": `["A"]`}, picks, plan, bellType()); err == nil {
		t.Fatal("a locked pick must still be refused, whatever travels with it")
	}
	// Writers that keep what passes: the pick is held, the list is stored and rings.
	held, changes, err := s.HeldPicks(context.Background(), e, map[string]any{"class": "Shadow", "skills_json": `["A"]`}, plan, bellType())
	if err != nil || len(held) != 1 || held["class"] == "" || len(changes) != 1 || changes[0].FieldKey != "skills_json" {
		t.Fatalf("held=%v changes=%v err=%v", held, changes, err)
	}
	held, changes, err = s.HeldPicks(context.Background(), e, map[string]any{"skills_json": `["A"]`}, plan, bellType())
	if err != nil || len(held) != 0 || len(changes) != 1 {
		t.Fatalf("a list alone is never held: held=%v changes=%v err=%v", held, changes, err)
	}
}

// bellStubSvc is the identity stub with a type that has every owner key.
type bellStubSvc struct{ identityStubSvc }

func (s *bellStubSvc) GetEntityTypeByID(_ context.Context, _ int) (*EntityType, error) {
	return &EntityType{Fields: bellType()}, nil
}

// The fields door rings for a story-only or list-only patch (with no plan
// loaded for it), stores it while the picks are shut, and rings nothing for a
// Director's or Scribe's own edit or for filling a blank.
func TestUpdateFieldsAPI_BellForListAndStory(t *testing.T) {
	owner := "user-owner"
	shut := &campaigns.HeroChangeRule{AfterMakingDays: 3}
	old := heroNow.Add(-30 * 24 * time.Hour)
	cases := []struct {
		name      string
		role      campaigns.Role
		userID    string
		body      string
		status    int
		wantBells int
		noPlan    bool // the patch needs no hero plan, so none is loaded
	}{
		{"a list change while picks are shut", campaigns.RolePlayer, owner, `{"fields_patch":{"skills_json":"[\"Sneak\",\"Alchemy\"]"}}`, 200, 1, true},
		{"a story change", campaigns.RolePlayer, owner, `{"fields_patch":{"backstory":"A new tale"}}`, 200, 1, true},
		{"a short text", campaigns.RolePlayer, owner, `{"fields_patch":{"pronouns":"they/them"}}`, 200, 1, false},
		{"a list and a story are one bell", campaigns.RolePlayer, owner, `{"fields_patch":{"skills_json":"[]","backstory":"x"}}`, 200, 1, true},
		{"filling a blank list rings nothing", campaigns.RolePlayer, owner, `{"fields_patch":{"languages_json":"[\"Elvish\"]"}}`, 200, 0, true},
		{"the same list resent rings nothing", campaigns.RolePlayer, owner, `{"fields_patch":{"skills_json":"[\"sneak\"]"}}`, 200, 0, true},
		{"a locked pick is refused", campaigns.RolePlayer, owner, `{"fields_patch":{"class":"Shadow","skills_json":"[]"}}`, 403, 0, false},
		{"a GM-only key is refused, not rung", campaigns.RolePlayer, owner, `{"fields_patch":{"gm_notes":"x"}}`, 403, 0, false},
		{"a Scribe's own edit rings nothing", campaigns.RoleScribe, "user-scribe", `{"fields_patch":{"skills_json":"[]","backstory":"x"}}`, 200, 0, false},
		{"the owner Director's own edit rings nothing", campaigns.RoleOwner, "user-dm", `{"fields_patch":{"backstory":"x"}}`, 200, 0, false},
	}
	plan := &HeroPlan{Steps: []HeroStep{{FieldKey: "class", Label: "Class", Choices: []HeroChoice{{Name: "Fury"}, {Name: "Shadow"}}}}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &bellStubSvc{identityStubSvc{entity: &Entity{ID: "e1", CampaignID: "c1", Name: "Hero", OwnerUserID: &owner,
				CreatedAt: old, FieldsData: map[string]any{"class": "Fury", "skills_json": `["Sneak"]`, "backstory": "Old tale", "pronouns": "she/her"}}}}
			hc, _, bell := newTestHeroChanges(shut)
			planner := &stubPlanner{plan: plan}
			h := &Handler{service: svc, heroPlanner: planner, heroChanges: hc}
			wantStatus(t, runIdentityCallOn(t, h, (*Handler).UpdateFieldsAPI, tc.role, tc.userID, "c1", tc.body), tc.status)
			if tc.status == 200 && svc.writeCount != 1 {
				t.Fatalf("the write must be stored once, stored %d", svc.writeCount)
			}
			if tc.status != 200 && svc.writeCount != 0 {
				t.Fatalf("a refused write stored something")
			}
			if len(bell.rang) != tc.wantBells {
				t.Fatalf("bells = %d, want %d (%v)", len(bell.rang), tc.wantBells, bell.rang)
			}
			if tc.noPlan && planner.reads != 0 {
				t.Fatalf("the hero plan was loaded %d times for a patch that needs none", planner.reads)
			}
			for _, ch := range bell.rang {
				for _, c := range ch {
					if c.FieldKey == "gm_notes" || c.FieldKey == "background" {
						t.Fatalf("a GM-only key was named: %+v", c)
					}
				}
			}
		})
	}
}
