package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

// fakeChoiceRules holds one ancestry list with a game entry, a marked game
// entry and a campaign entry sharing a game entry's name, and records the
// batches it is asked to check and write.
type fakeChoiceRules struct {
	broken  *systems.MarksCheck
	checked []systems.ChoiceMarksInput
	set     []systems.ChoiceMarksInput
	rules   []systems.ChoiceRule
}

func (f *fakeChoiceRules) Rules(context.Context, string, string, systems.EntryActor) ([]systems.ChoiceRule, error) {
	return f.rules, nil
}

func (f *fakeChoiceRules) DirectorEntries(_ context.Context, _ string, a systems.EntryActor, field, _ string) (*systems.DirectorEntryList, error) {
	if !a.IsDirector {
		return nil, nil
	}
	return &systems.DirectorEntryList{FieldKey: field, Entries: []systems.DirectorEntry{
		{Key: "time-raider", Source: systems.ChoiceSourcePackage, Slug: "time-raider", Name: "Time Raider", State: systems.RuleOpen},
		{Key: "memonek", Source: systems.ChoiceSourcePackage, Slug: "memonek", Name: "Memonek", State: systems.RuleNotOffered,
			Everyone: &systems.MarkView{State: systems.RuleNotOffered, Reason: "Not on Therin."}},
		{Key: "dwarf", Source: systems.ChoiceSourcePackage, Slug: "dwarf", Name: "Dwarf", State: systems.RuleOpen},
		{Key: "12", Source: systems.ChoiceSourceCampaign, Slug: "dwarf", Name: "Dwarf", State: systems.RuleOpen},
	}}, nil
}

func (f *fakeChoiceRules) CheckMarks(_ context.Context, _ string, _ systems.EntryActor, in systems.ChoiceMarksInput) (*systems.MarksCheck, error) {
	f.checked = append(f.checked, in)
	if f.broken != nil {
		return f.broken, nil
	}
	return &systems.MarksCheck{}, nil
}

func (f *fakeChoiceRules) SetMarks(_ context.Context, _ string, _ systems.EntryActor, in systems.ChoiceMarksInput) ([]systems.ChoiceRule, error) {
	f.set = append(f.set, in)
	return nil, nil
}

func TestHeroChoiceRuleKind_Plan(t *testing.T) {
	tests := []struct {
		name     string
		actor    Actor
		action   string
		fields   map[string]any
		broken   *systems.MarksCheck
		wantErr  string
		wantSum  string
		wantWarn string
		wantMark systems.ChoiceMark
	}{
		{name: "hides a game entry", actor: owner, action: ActionCreate,
			fields:   map[string]any{"entry": "ancestry", "name": "Time Raider", "mark": "not-here", "reason": "No Time Raiders here."},
			wantSum:  `takes "Time Raider" (the game system's) off players' ancestry list in the hero maker (Not here); players read: "No Time Raiders here."`,
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourcePackage, Key: "time-raider", State: systems.RuleNotOffered}},
		{name: "by slug, stored state name", actor: owner, action: ActionUpdate,
			fields:   map[string]any{"entry": "Ancestry", "name": "time-raider", "mark": "ask_first"},
			wantSum:  "(Ask first)",
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourcePackage, Key: "time-raider", State: systems.RuleAskFirst}},
		{name: "delete clears the mark", actor: owner, action: ActionDelete,
			fields:   map[string]any{"entry": "ancestry", "name": "Memonek"},
			wantSum:  "removes the mark on",
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourcePackage, Key: "memonek", State: "clear"}},
		{name: "same mark again warns", actor: owner, action: ActionUpdate,
			fields:   map[string]any{"entry": "ancestry", "name": "Memonek", "mark": "not here"},
			wantWarn: "already has this mark",
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourcePackage, Key: "memonek", State: systems.RuleNotOffered}},
		{name: "heroes that hold it are named", actor: owner, action: ActionCreate,
			fields:   map[string]any{"entry": "ancestry", "name": "Time Raider", "mark": "not-here"},
			broken:   &systems.MarksCheck{Count: 1, Heroes: []systems.BrokenHero{{HeroName: "Vex", PlayerName: "Pat"}}},
			wantWarn: "1 hero(es) already hold it and keep it, flagged on their sheets: Vex (Pat)",
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourcePackage, Key: "time-raider", State: systems.RuleNotOffered}},
		{name: "source picks between two of a name", actor: owner, action: ActionCreate,
			fields:   map[string]any{"entry": "ancestry", "name": "Dwarf", "mark": "open", "source": "campaign"},
			wantMark: systems.ChoiceMark{Source: systems.ChoiceSourceCampaign, Key: "12", State: "clear"}},
		{name: "two of a name need a source", actor: owner, action: ActionCreate,
			fields: map[string]any{"entry": "ancestry", "name": "Dwarf", "mark": "not-here"}, wantErr: "add `source: game` or `source: campaign`"},
		{name: "unknown entry", actor: owner, action: ActionCreate,
			fields: map[string]any{"entry": "ancestry", "name": "Elf", "mark": "not-here"}, wantErr: "no ancestry entry is called"},
		{name: "unknown mark", actor: owner, action: ActionCreate,
			fields: map[string]any{"entry": "ancestry", "name": "Memonek", "mark": "maybe"}, wantErr: "`mark:` must be"},
		{name: "needs a field", actor: owner, action: ActionCreate,
			fields: map[string]any{"name": "Memonek", "mark": "not-here"}, wantErr: "needs `entry:`"},
		{name: "players cannot", actor: player, action: ActionCreate,
			fields: map[string]any{"entry": "ancestry", "name": "Memonek", "mark": "open"}, wantErr: "only the campaign's Directors"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeChoiceRules{broken: tt.broken}
			k := HeroChoiceRuleKind{Svc: f}
			r := rec("hero-choice-rule", tt.action, tt.fields["name"].(string), tt.fields, "")
			p := k.Plan(context.Background(), camp, tt.actor, r)
			if tt.wantErr != "" {
				if !strings.Contains(p.Error, tt.wantErr) {
					t.Fatalf("error %q, want %q", p.Error, tt.wantErr)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if !strings.Contains(p.Summary, tt.wantSum) {
				t.Errorf("summary %q, want %q", p.Summary, tt.wantSum)
			}
			if tt.wantWarn != "" && !strings.Contains(strings.Join(p.Warnings, "|"), tt.wantWarn) {
				t.Errorf("warnings %q, want %q", p.Warnings, tt.wantWarn)
			}
			if len(f.checked) != 1 || len(f.checked[0].Marks) != 1 {
				t.Fatalf("checked %+v", f.checked)
			}
			got := f.checked[0].Marks[0]
			if got.Source != tt.wantMark.Source || got.Key != tt.wantMark.Key || got.State != tt.wantMark.State || got.Target != "" {
				t.Errorf("mark %+v, want %+v", got, tt.wantMark)
			}
			if len(f.set) != 0 {
				t.Error("Plan wrote a mark")
			}
		})
	}
}

// Apply writes the one mark for everyone, carrying the reason only when the
// block names one.
func TestHeroChoiceRuleKind_Apply(t *testing.T) {
	f := &fakeChoiceRules{}
	k := HeroChoiceRuleKind{Svc: f}
	r := rec("hero-choice-rule", ActionCreate, "Time Raider", map[string]any{"entry": "ancestry", "mark": "not-here", "reason": "Gone."}, "")
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	r = rec("hero-choice-rule", ActionUpdate, "Memonek", map[string]any{"entry": "ancestry", "mark": "ask-first"}, "")
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	if len(f.set) != 2 || f.set[0].FieldKey != "ancestry" {
		t.Fatalf("set %+v", f.set)
	}
	if m := f.set[0].Marks[0]; m.Key != "time-raider" || m.State != systems.RuleNotOffered || m.Reason == nil || *m.Reason != "Gone." {
		t.Errorf("first mark %+v", m)
	}
	if m := f.set[1].Marks[0]; m.Reason != nil {
		t.Errorf("a block without reason must keep the stored one, got %q", *m.Reason)
	}
	if err := k.Apply(context.Background(), camp, player, r); err == nil {
		t.Error("a player's Apply should be refused")
	}
}

func TestHeroChoiceRuleKind_Export(t *testing.T) {
	f := &fakeChoiceRules{rules: []systems.ChoiceRule{
		{FieldKey: "ancestry", Source: systems.ChoiceSourcePackage, Key: "memonek", Name: "Memonek", State: systems.RuleNotOffered, Reason: "Not on Therin."},
		{FieldKey: "kit", Source: systems.ChoiceSourcePackage, Key: systems.RuleGroupKey, State: systems.RuleAskFirst},
		{FieldKey: "ancestry", Source: systems.ChoiceSourcePackage, Key: "memonek", Name: "Memonek", Target: "pat", State: systems.RuleOpen},
	}}
	k := HeroChoiceRuleKind{Svc: f}
	out, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`- ancestry: Memonek, Not here, reason "Not on Therin."`,
		"- kit: every game-system entry (set on the Own entries page), Ask first",
		"- and 1 mark(s) for single players",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("export lacks %q:\n%s", want, out)
		}
	}
	if out, _ := k.Export(context.Background(), camp, player); out != "" {
		t.Errorf("a player's export should be empty, got %q", out)
	}
}

// The system-entry kind tells the AI where hiding a game entry lives, since
// it cannot change those entries itself.
func TestSystemEntryDoc_PointsAtHeroChoiceRule(t *testing.T) {
	if !strings.Contains((SystemEntryKind{}).Doc(), "`kind: hero-choice-rule`") {
		t.Error("system-entry doc does not point at hero-choice-rule")
	}
}

type markedEntries struct{}

func (markedEntries) Entries(context.Context, string, string, bool) (string, []SystemEntry, error) {
	return "Draw Steel", []SystemEntry{
		{Name: "Dwarf", Slug: "dwarf", Source: "package"},
		{Name: "Memonek", Slug: "memonek", Source: "package", Mark: systems.RuleNotOffered, MarkReason: "Not on Therin."},
		{Name: "Hakaan", Slug: "hakaan", Source: "package", Mark: systems.RuleAskFirst, MarkFromGroup: true, PlayerMarks: 2},
	}, nil
}
func (markedEntries) Fields(context.Context, string, bool) (string, []SystemEntryField, error) {
	return "", nil, nil
}

func TestLookups_SystemEntriesShowMarks(t *testing.T) {
	l := &Lookups{Entries: markedEntries{}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("system-entries", map[string]any{"field": "ancestry"})})[0]
	if ans.Error != "" {
		t.Fatal(ans.Error)
	}
	for _, want := range []string{
		"Change a mark for every player with `kind: hero-choice-rule`.",
		"- Dwarf (dwarf), package\n",
		`- Memonek (memonek), package, marked Not here: players' hero maker leaves it out, reason "Not on Therin."`,
		"- Hakaan (hakaan), package, marked Ask first: players need a Director's yes to pick it (from the mark on every entry of its source), 2 marks for single players",
	} {
		if !strings.Contains(ans.Text, want) {
			t.Errorf("answer lacks %q:\n%s", want, ans.Text)
		}
	}
	// A list with no marks carries no note about them.
	l = &Lookups{Entries: fakeEntryStatus{}}
	if ans := l.Answer(context.Background(), camp, owner, []Record{lookup("system-entries", map[string]any{"field": "ancestry"})})[0]; strings.Contains(ans.Text, "hero-choice rules") {
		t.Errorf("an unmarked list explains marks:\n%s", ans.Text)
	}
}
