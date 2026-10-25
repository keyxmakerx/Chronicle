package records

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
)

// parseBlocks reads pasted blocks as the review does.
func parseBlocks(s string) []Record {
	var out []Record
	for _, p := range importer.Parse(s) {
		fields := map[string]any{}
		for k, v := range p.Fields {
			fields[strings.ToLower(k)] = v
		}
		out = append(out, Record{Kind: p.FrontMatter.Kind, Action: p.FrontMatter.Action, Name: p.Name, Fields: fields})
	}
	return out
}

// fakeTests is TestsAPI over a fixed list.
type fakeTests []TestRef

func (f fakeTests) Tests(context.Context, string, Actor) ([]TestRef, error) { return f, nil }

// fakeMonsters is FoundryMonstersAPI over a fixed report.
type fakeMonsters struct{ rep FoundryMonsterReport }

func (f fakeMonsters) Monsters(context.Context, string, Actor) (FoundryMonsterReport, error) {
	return f.rep, nil
}

var roadTests = fakeTests{
	{ID: "t1", Name: "Road to Lastwell", PageName: "The Siege of Harrowmere",
		Results: []TestResult{{ID: "r1", Label: "Success"}, {ID: "r2", Label: "Partial success"}, {ID: "r3", Label: "Failure"}}},
	{ID: "t2", Name: "Hold the walls", PageName: "Elsewhere", Results: []TestResult{{ID: "w1", Label: "Success"}}},
	{ID: "t3", Name: "Hold the walls", PageName: "The Siege of Harrowmere", Results: []TestResult{{ID: "w2", Label: "Success"}}},
}

func version(name, test, result string, creatures ...any) map[string]any {
	v := map[string]any{"name": name, "creatures": creatures}
	if test != "" {
		v["test"], v["result"] = test, result
	}
	return v
}

func TestEncounterVersions_Plan(t *testing.T) {
	tests := []struct {
		name     string
		versions any
		tests    TestsAPI
		wantErr  string
		wantSum  string
	}{
		{name: "three montage results", tests: roadTests, versions: []any{
			version("Three Stalkers", "Road to Lastwell", "Success", "3 Stalker"),
			version("Five Stalkers", "road to lastwell", "partial success", "5 Stalker"),
			version("Six Stalkers", "Road to Lastwell", "Failure", "6 Stalker"),
		}, wantSum: "3 versions"},
		{name: "picked by the DM only", versions: []any{version("Ambush", "", "", "2 Stalker, starts hidden")}, wantSum: "1 version"},
		{name: "clears them", versions: []any{}, wantSum: "no versions"},
		{name: "no such test", tests: roadTests, versions: []any{version("A", "Road to Nowhere", "Success", "Rat")}, wantErr: `no test called "Road to Nowhere"`},
		{name: "no such result", tests: roadTests, versions: []any{version("A", "Road to Lastwell", "Triumph", "Rat")}, wantErr: `has no result "Triumph"; its results are "Success", "Partial success", "Failure"`},
		{name: "same name prefers this page", tests: roadTests, versions: []any{version("A", "Hold the walls", "Success", "Rat")}, wantSum: "1 version"},
		{name: "one result picks one version", tests: roadTests, versions: []any{
			version("A", "Road to Lastwell", "Success", "Rat"), version("B", "Road to Lastwell", "Success", "Rat"),
		}, wantErr: "two versions are picked by"},
		{name: "test without result", versions: []any{map[string]any{"name": "A", "test": "Road to Lastwell", "creatures": []any{"Rat"}}}, wantErr: "both `test` and `result`"},
		{name: "no creatures", versions: []any{map[string]any{"name": "A"}}, wantErr: "lists every creature"},
		{name: "no name", versions: []any{map[string]any{"creatures": []any{"Rat"}}}, wantErr: "needs a `name`"},
		{name: "a bare line", versions: []any{"3 Stalker"}, wantErr: "set of keys"},
		{name: "duplicate names", versions: []any{version("A", "", "", "Rat"), version("a", "", "", "Rat")}, wantErr: "two versions are called"},
		{name: "bad creature line", versions: []any{version("A", "", "", "Rat, group Q")}, wantErr: "A: "},
		{name: "no tests service", versions: []any{version("A", "Road to Lastwell", "Success", "Rat")}, wantErr: "tests can't be read"},
		{name: "too many", versions: []any{version("1", "", "", "Rat"), version("2", "", "", "Rat"), version("3", "", "", "Rat"),
			version("4", "", "", "Rat"), version("5", "", "", "Rat"), version("6", "", "", "Rat"), version("7", "", "", "Rat")}, wantErr: "at most 6 versions"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k := EncounterKind{Svc: &fakeEncounters{byPage: map[string][]EncounterRef{}}, Entities: encounterPages(), Tests: tc.tests}
			p := k.Plan(context.Background(), camp, owner, encounterRecord(ActionCreate, map[string]any{"creatures": []any{"Rat"}, "versions": tc.versions}))
			if tc.wantErr != "" {
				if !strings.Contains(p.Error, tc.wantErr) {
					t.Fatalf("want error %q, got %+v", tc.wantErr, p)
				}
				return
			}
			if p.Error != "" {
				t.Fatalf("unexpected error %q", p.Error)
			}
			if !strings.HasSuffix(p.Summary, tc.wantSum) {
				t.Errorf("summary %q, want it to end %q", p.Summary, tc.wantSum)
			}
		})
	}
}

func TestEncounterVersions_Apply(t *testing.T) {
	ctx := context.Background()
	svc := &fakeEncounters{byPage: map[string][]EncounterRef{}}
	k := EncounterKind{Svc: svc, Entities: encounterPages(), Tests: roadTests}
	err := k.Apply(ctx, camp, owner, encounterRecord(ActionCreate, map[string]any{
		"creatures": []any{"3 Goblin War Chief"},
		"versions": []any{
			version("Five", "Hold the walls", "Success", "5 Goblin War Chief"),
			version("Ambush", "", "", "Goblin War Chief, starts hidden"),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	c := svc.created[0]
	if !c.SetVersions || len(c.Versions) != 2 {
		t.Fatalf("versions: %+v", c)
	}
	if v := c.Versions[0]; v.TestID != "t3" || v.ResultID != "w2" || v.Creatures[0].Count != 5 || v.Creatures[0].EntityID != "gob" {
		t.Errorf("first version: %+v", v)
	}
	if v := c.Versions[1]; v.TestID != "" || v.Creatures[0].Start != "hidden" {
		t.Errorf("second version: %+v", v)
	}

	// An update that leaves `versions` out keeps the DM's.
	svc = &fakeEncounters{byPage: map[string][]EncounterRef{"page1": {{ID: "e1", Name: "Ramparts fight", Version: 2}}}}
	k.Svc = svc
	if err := k.Apply(ctx, camp, owner, encounterRecord(ActionUpdate, map[string]any{"ready": true})); err != nil {
		t.Fatal(err)
	}
	if svc.updated[0].SetVersions {
		t.Error("an update without versions must keep them")
	}
	if err := k.Apply(ctx, camp, owner, encounterRecord(ActionUpdate, map[string]any{"versions": []any{}})); err != nil {
		t.Fatal(err)
	}
	if u := svc.updated[1]; !u.SetVersions || len(u.Versions) != 0 {
		t.Errorf("versions: [] must clear them: %+v", u)
	}
}

func TestEncounterKind_ForeignNameWarning(t *testing.T) {
	at := time.Now()
	report := fakeMonsters{FoundryMonsterReport{ReportedAt: &at, Monsters: []FoundryMonster{{Name: "Stalker", Pack: "Monsters"}}}}
	k := EncounterKind{Svc: &fakeEncounters{byPage: map[string][]EncounterRef{}}, Entities: encounterPages(), Monsters: report}
	p := k.Plan(context.Background(), camp, owner, encounterRecord(ActionCreate, map[string]any{"creatures": []any{"3 Stalker", "2 Stalker, group B", "Wyrm"}}))
	w := strings.Join(p.Warnings, "|")
	if !strings.Contains(w, `"Stalker" is not a page yet; it goes in by name, and Foundry has it (in the Monsters compendium)`) {
		t.Errorf("warnings %q", p.Warnings)
	}
	if !strings.Contains(w, `"Wyrm" is not a page yet; it goes in by name, but Foundry did not report`) {
		t.Errorf("warnings %q", p.Warnings)
	}
	if strings.Count(w, `"Stalker"`) != 1 {
		t.Errorf("one warning per name: %q", p.Warnings)
	}
}

func TestEncounterExport_RoundTrips(t *testing.T) {
	e := EncounterRef{Name: "Road fight", PageName: "The Siege of Harrowmere", Ready: true, Heroes: []string{"Ash"},
		Creatures: []EncounterCreature{{Name: "Stalker", Count: 3, Group: "A", Start: "seen"}},
		Versions: []EncounterVersion{{Name: "Six", Test: "Road to Lastwell", Result: "Failure",
			Creatures: []EncounterCreature{{Name: "Stalker", Count: 6, Group: "A", Start: "later"}}}}}
	var b strings.Builder
	writeEncounterBlock(&b, e)
	recs := parseBlocks(b.String())
	if len(recs) != 1 {
		t.Fatalf("parsed %d blocks from %q", len(recs), b.String())
	}
	vs, err := parseVersions(recs[0].List("versions"))
	if err != nil || len(vs) != 1 || vs[0].test != "Road to Lastwell" || vs[0].result != "Failure" ||
		vs[0].creatures[0].Count != 6 || vs[0].creatures[0].Start != "later" {
		t.Fatalf("versions read back: %+v %v\n%s", vs, err, b.String())
	}
}
