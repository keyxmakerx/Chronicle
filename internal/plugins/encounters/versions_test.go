package encounters

import (
	"context"
	"errors"
	"testing"
)

// stalkers is the base fight with three versions, one per Lastwell result.
func stalkers() SaveInput {
	in := ramparts()
	in.Name = "Stalkers"
	in.Versions = []Version{
		{ID: "v1", Name: "They were ready", Pick: &Pick{Test: "lastwell", Result: "win"}, Remove: []string{"r3"}},
		{ID: "v2", Name: "Half warned", Pick: &Pick{Test: "lastwell", Result: "part"},
			Add:    []Creature{{ID: "a1", EntityID: "gob", Name: "Stalker alpha", Count: 1, Group: "a", Start: StartLater}},
			Counts: []CountChange{{Row: "r2", Count: 4}}},
		{ID: "v3", Name: "Ambushed", Pick: &Pick{Test: "lastwell", Result: "lose"}},
	}
	return in
}

func TestVersionsSaveAndPick(t *testing.T) {
	svc, _, _, results := newVersionService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, stalkers())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(e.Versions) != 3 {
		t.Fatalf("versions: %+v", e.Versions)
	}
	if a := e.Versions[1].Add[0]; a.Group != "A" {
		t.Errorf("added row not cleaned: %+v", a)
	}
	if p := e.Versions[0].Pick; p == nil || p.TestName != "Restore the Lastwell core" || p.ResultLabel != "Total success" {
		t.Errorf("pick not named: %+v", p)
	}
	if e.Picked == nil || e.Picked.Version != "" || len(e.Picked.Open) != 1 || e.Picked.Open[0] != "Restore the Lastwell core" {
		t.Fatalf("before a result, the base is played and the test is open: %+v", e.Picked)
	}

	o := results.tests["lastwell"]
	o.Recorded = "part"
	results.tests["lastwell"] = o
	got, err := svc.Get(ctx, "c1", e.ID, dm)
	if err != nil {
		t.Fatal(err)
	}
	if got.Picked.Version != "v2" || got.Picked.Result != "Partial success" || len(got.Picked.Open) != 0 {
		t.Errorf("recorded result should pick its version: %+v", got.Picked)
	}
}

func TestNoVersionsMeansNoPick(t *testing.T) {
	svc, _, _ := newTestService()
	e, err := svc.Create(context.Background(), "c1", "page1", dm, ramparts())
	if err != nil {
		t.Fatal(err)
	}
	if e.Picked != nil || e.Versions == nil || len(e.Versions) != 0 {
		t.Errorf("an encounter without versions has none and no pick: %+v %+v", e.Versions, e.Picked)
	}
}

func TestVersionValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	tests := []struct {
		name   string
		mutate func(*SaveInput)
	}{
		{"too many", func(in *SaveInput) {
			for i := 0; i < MaxVersions; i++ {
				in.Versions = append(in.Versions, Version{ID: "x" + string(rune('a'+i)), Name: "X"})
			}
		}},
		{"duplicate id", func(in *SaveInput) { in.Versions[1].ID = "v1" }},
		{"no name", func(in *SaveInput) { in.Versions[0].Name = " " }},
		{"half a pick", func(in *SaveInput) { in.Versions[0].Pick = &Pick{Test: "lastwell"} }},
		{"same pick twice", func(in *SaveInput) { in.Versions[2].Pick = &Pick{Test: "lastwell", Result: "win"} }},
		{"test elsewhere", func(in *SaveInput) { in.Versions[0].Pick = &Pick{Test: "other", Result: "win"} }},
		{"result not the test's", func(in *SaveInput) { in.Versions[0].Pick = &Pick{Test: "hunt", Result: "win"} }},
		{"added row reuses a base id", func(in *SaveInput) { in.Versions[1].Add[0].ID = "r1" }},
		{"added row reuses another version's id", func(in *SaveInput) {
			in.Versions[2].Add = []Creature{{ID: "a1", Name: "Rat", Count: 1}}
		}},
		{"added page from elsewhere", func(in *SaveInput) { in.Versions[1].Add[0].EntityID = "other" }},
		{"remove a row not in the base", func(in *SaveInput) { in.Versions[0].Remove = []string{"a1"} }},
		{"remove and recount", func(in *SaveInput) { in.Versions[1].Remove = []string{"r2"} }},
		{"count out of range", func(in *SaveInput) { in.Versions[1].Counts[0].Count = MaxCreatureCount + 1 }},
		{"too many added", func(in *SaveInput) {
			for i := 0; i <= MaxVersionAdds; i++ {
				in.Versions[0].Add = append(in.Versions[0].Add, Creature{ID: "z" + string(rune('a'+i)), Name: "Rat", Count: 1})
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := stalkers()
			tc.mutate(&in)
			if _, err := svc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
				t.Errorf("want 422, got %v", err)
			}
		})
	}
}

// A stage may bring in a row only a version adds; a stage naming a row no
// version has is still refused.
func TestStagesMayNameAddedRows(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	in := stalkers()
	in.Stages = []Stage{{ID: "s1", When: StageWhen{Kind: WhenRound, Round: 3}, Arrive: []string{"a1"}}}
	e, err := svc.Create(ctx, "c1", "page1", dm, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(e.Stages) != 1 || len(e.Stages[0].Arrive) != 1 {
		t.Errorf("stage lost its added row: %+v", e.Stages)
	}
	in.Stages[0].Arrive = []string{"nope"}
	if _, err := svc.Create(ctx, "c1", "page1", dm, in); statusOf(err) != 422 {
		t.Errorf("unknown row: want 422, got %v", err)
	}
}

// A pick whose test has gone is cleared on read, so the version stays for the
// DM to pick by hand and a later save is not refused.
func TestGoneTestClearsPick(t *testing.T) {
	svc, _, _, results := newVersionService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, stalkers())
	if err != nil {
		t.Fatal(err)
	}
	delete(results.tests, "lastwell")
	got, err := svc.Get(ctx, "c1", e.ID, dm)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range got.Versions {
		if v.Pick != nil {
			t.Errorf("pick of a gone test kept: %+v", v)
		}
	}
	if got.Picked == nil || got.Picked.Version != "" || len(got.Picked.Open) != 0 {
		t.Errorf("nothing picks once the test is gone: %+v", got.Picked)
	}
	body := SaveInput{Name: got.Name, Heroes: []string{"hero1"}, Creatures: got.Creatures, Versions: got.Versions, Version: &got.Version}
	if _, err := svc.Update(ctx, "c1", e.ID, dm, body); err != nil {
		t.Errorf("saving what was read back should work: %v", err)
	}
}

// A test directory that fails must not take the encounter down; its picks
// read as unknown.
func TestResultsFailureReadsAsUnknown(t *testing.T) {
	svc, _, _, results := newVersionService()
	ctx := context.Background()
	e, err := svc.Create(ctx, "c1", "page1", dm, stalkers())
	if err != nil {
		t.Fatal(err)
	}
	results.err = errors.New("boom")
	got, err := svc.Get(ctx, "c1", e.ID, dm)
	if err != nil {
		t.Fatalf("get should still work: %v", err)
	}
	if p := got.Versions[0].Pick; p == nil || p.Test == "" || p.TestName != "" {
		t.Errorf("an unreadable pick must be kept as stored, unnamed: %+v", got.Versions[0])
	}
	if got.Picked != nil {
		t.Errorf("no answer can be given while the tests cannot be read: %+v", got.Picked)
	}
	// A save in that state is refused rather than dropping the picks.
	in := stalkers()
	in.Version = &got.Version
	if _, err := svc.Update(ctx, "c1", e.ID, dm, in); err == nil {
		t.Error("a save whose picks cannot be checked must be refused")
	}
	results.err = nil
	if got, _ = svc.Get(ctx, "c1", e.ID, dm); got.Versions[0].Pick == nil {
		t.Error("the pick must survive the outage")
	}
}

func TestPicksNeedTheTestsBlock(t *testing.T) {
	svc := NewService(newMemRepo(), newFakePages(), nil)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "c1", "page1", dm, stalkers()); statusOf(err) != 422 {
		t.Errorf("a pick with no tests: want 422, got %v", err)
	}
	in := stalkers()
	for i := range in.Versions {
		in.Versions[i].Pick = nil
	}
	if _, err := svc.Create(ctx, "c1", "page1", dm, in); err != nil {
		t.Errorf("versions without picks save without tests: %v", err)
	}
}

func TestCarryVersions(t *testing.T) {
	in := []Version{{ID: "v1", Name: "X", Pick: &Pick{Test: "t", Result: "r", TestName: "n"},
		Remove: []string{"r1", "gone"}, Counts: []CountChange{{Row: "gone", Count: 2}, {Row: "r2", Count: 3}},
		Add: []Creature{{ID: "a1", Name: "Rat", Count: 1}}}}
	out := CarryVersions(in, []Creature{{ID: "r1"}, {ID: "r2"}})
	v := out[0]
	if len(v.Remove) != 1 || v.Remove[0] != "r1" || len(v.Counts) != 1 || v.Counts[0].Row != "r2" || len(v.Add) != 1 {
		t.Errorf("carry kept the wrong changes: %+v", v)
	}
	if v.Pick.TestName != "" {
		t.Errorf("read-only names should not be sent back: %+v", v.Pick)
	}
}
