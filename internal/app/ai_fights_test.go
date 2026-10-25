package app

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/encounters"
)

func TestAIVersions(t *testing.T) {
	base := []encounters.Creature{
		{ID: "ai1", EntityID: "stalker", Name: "Stalker", Count: 3, Group: "A", Start: "seen"},
		{ID: "ai2", Name: "Wolf", Count: 2, Group: "B", Start: "seen"},
	}
	line := func(entity, name string, n int, group, start string) records.EncounterCreature {
		return records.EncounterCreature{EntityID: entity, Name: name, Count: n, Group: group, Start: start}
	}
	old := []encounters.Version{{ID: "v7", Name: "Six", Add: []encounters.Creature{{ID: "ai9", Name: "Bone Wyrm", Count: 1, Group: "C", Start: "later"}}}}
	in := []records.EncounterVersionInput{
		{Name: "Six", TestID: "t1", ResultID: "r3", Creatures: []records.EncounterCreature{
			line("stalker", "Stalker", 6, "A", "seen"), line("", "bone wyrm", 1, "C", "later")}},
		{Name: "Quiet", Creatures: []records.EncounterCreature{line("", "Wolf", 2, "B", "seen"), line("", "Wolf", 1, "B", "hidden")}},
	}
	got := aiVersions(in, base, old, []encounters.Creature{{ID: "ai3"}})
	if len(got) != 2 {
		t.Fatalf("versions: %+v", got)
	}
	six := got[0]
	if six.ID != "v7" || six.Pick == nil || six.Pick.Test != "t1" || six.Pick.Result != "r3" {
		t.Errorf("a version named as before keeps its id; pick from the input: %+v", six)
	}
	if !reflect.DeepEqual(six.Counts, []encounters.CountChange{{Row: "ai1", Count: 6}}) || !reflect.DeepEqual(six.Remove, []string{"ai2"}) {
		t.Errorf("six changes: counts %+v remove %+v", six.Counts, six.Remove)
	}
	if len(six.Add) != 1 || six.Add[0].ID != "ai9" {
		t.Errorf("an added row it had keeps its id: %+v", six.Add)
	}
	quiet := got[1]
	if quiet.ID != "v1" || quiet.Pick != nil {
		t.Errorf("a new version gets a fresh id and no pick: %+v", quiet)
	}
	if !reflect.DeepEqual(quiet.Remove, []string{"ai1"}) || len(quiet.Counts) != 0 || len(quiet.Add) != 1 || quiet.Add[0].ID != "ai4" {
		t.Errorf("quiet: %+v (ai1–ai3 and ai9 are taken)", quiet)
	}

	// Reading back gives the version's whole list again.
	rows := aiVersionRows(base, six)
	want := []records.EncounterCreature{line("stalker", "Stalker", 6, "A", "seen"), line("", "bone wyrm", 1, "C", "later")}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("rows %+v, want %+v", rows, want)
	}
}

func TestAIEncounterRef_Versions(t *testing.T) {
	e := encounters.Encounter{Name: "Road", Creatures: []encounters.Creature{{ID: "ai1", Name: "Stalker", Count: 3, Group: "A", Start: "seen"}},
		Versions: []encounters.Version{{ID: "v1", Name: "Six", Counts: []encounters.CountChange{{Row: "ai1", Count: 6}},
			Pick: &encounters.Pick{Test: "t", Result: "r", TestName: "Road to Lastwell", ResultLabel: "Failure"}}},
		Picked: &encounters.Picked{Version: "v1"}}
	ref := aiEncounterRef(e)
	if ref.Playing != "Six" || len(ref.Versions) != 1 || ref.Versions[0].Test != "Road to Lastwell" || ref.Versions[0].Result != "Failure" ||
		ref.Versions[0].Creatures[0].Count != 6 {
		t.Fatalf("ref: %+v", ref)
	}
}
