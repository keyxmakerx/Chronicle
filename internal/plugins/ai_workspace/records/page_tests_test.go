package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeTestWriter keeps tests by page; noBlock lists pages whose layout has no
// Tests block.
type fakeTestWriter struct {
	byPage  map[string][]TestSetup
	noBlock map[string]bool
	created []TestSetup
	updated []TestSetup
	deleted []string
}

func (f *fakeTestWriter) OnPage(_ context.Context, _, pageID string, _ Actor) ([]TestSetup, error) {
	return f.byPage[pageID], nil
}
func (f *fakeTestWriter) Create(_ context.Context, _, _ string, _ Actor, in TestSetup) error {
	f.created = append(f.created, in)
	return nil
}
func (f *fakeTestWriter) Update(_ context.Context, _, id string, _ Actor, in TestSetup) error {
	in.ID = id
	f.updated = append(f.updated, in)
	return nil
}
func (f *fakeTestWriter) Delete(_ context.Context, _, id string, _ Actor) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeTestWriter) HasBlock(_ context.Context, _, pageID string) (bool, error) {
	return !f.noBlock[pageID], nil
}
func (f *fakeTestWriter) Pages(context.Context, string, Actor) ([]entities.Entity, error) {
	return []entities.Entity{{ID: "p-storm", Name: "The Sandstorm"}}, nil
}

func testKindFixture() (TestKind, *fakeTestWriter) {
	svc := &fakeTestWriter{byPage: map[string][]TestSetup{
		"p-storm": {{ID: "t1", Name: "Hold the walls", Type: "Montage", Successes: 4, Version: 3,
			Results: []TestSetupResult{{ID: "win", Label: "Success", On: "successes"}, {ID: "lose", Label: "Failure"}}}},
	}, noBlock: map[string]bool{}}
	pages := fakeEntities{"the-sandstorm": {ID: "p-storm", CampaignID: camp, Name: "The Sandstorm"}}
	return TestKind{Svc: svc, Entities: pages}, svc
}

func TestTestKind_Create(t *testing.T) {
	k, svc := testKindFixture()
	r := rec("test", ActionCreate, "Cross the sandstorm", map[string]any{
		"page": "The Sandstorm", "type": "Montage", "successes": 5, "failures": 3,
		"results": []any{"Total success, on successes", "Partial success", "Total failure, on failures"},
	}, "")
	p := k.Plan(context.Background(), camp, owner, r)
	if p.Error != "" {
		t.Fatalf("plan: %s", p.Error)
	}
	if !strings.Contains(p.Summary, "5 successes") || !strings.Contains(p.Summary, "hidden from players") {
		t.Fatalf("summary %q", p.Summary)
	}
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	got := svc.created[0]
	if got.Name != "Cross the sandstorm" || got.Type != "Montage" || got.Successes != 5 || got.Failures != 3 || got.Shown {
		t.Fatalf("created %+v", got)
	}
	want := []TestSetupResult{{"total-success", "Total success", "successes"}, {"partial-success", "Partial success", ""}, {"total-failure", "Total failure", "failures"}}
	for i, w := range want {
		if got.Results[i] != w {
			t.Fatalf("result %d = %+v, want %+v", i, got.Results[i], w)
		}
	}

	// No results gives the block's own three.
	bare := rec("test", ActionCreate, "Chase", map[string]any{"page": "The Sandstorm"}, "")
	if err := k.Apply(context.Background(), camp, owner, bare); err != nil {
		t.Fatal(err)
	}
	if n := len(svc.created[1].Results); n != 3 {
		t.Fatalf("default results: %d", n)
	}
}

// An update keeps the ids of results whose names stay, so encounter
// versions that name them are still picked, and keeps keys it leaves out.
func TestTestKind_UpdateKeepsResultIDs(t *testing.T) {
	k, svc := testKindFixture()
	r := rec("test", ActionUpdate, "hold the walls", map[string]any{
		"page": "The Sandstorm", "players_see": true,
		"results": []any{"Success, on successes", "Partial success", "Failure"},
	}, "")
	if err := k.Apply(context.Background(), camp, owner, r); err != nil {
		t.Fatal(err)
	}
	got := svc.updated[0]
	if got.ID != "t1" || got.Version != 3 || got.Type != "Montage" || got.Successes != 4 || !got.Shown {
		t.Fatalf("updated %+v", got)
	}
	if got.Results[0].ID != "win" || got.Results[2].ID != "lose" || got.Results[1].ID != "partial-success" {
		t.Fatalf("result ids %+v", got.Results)
	}
}

func TestTestKind_Refusals(t *testing.T) {
	k, _ := testKindFixture()
	tests := []struct {
		name   string
		a      Actor
		action string
		test   string
		fields map[string]any
		want   string
	}{
		{"player", player, ActionCreate, "New", map[string]any{"page": "The Sandstorm"}, "owner or a co-DM"},
		{"no page", owner, ActionCreate, "New", map[string]any{}, "needs `page`"},
		{"unknown page", owner, ActionCreate, "New", map[string]any{"page": "Nowhere"}, "no page called"},
		{"already there", owner, ActionCreate, "Hold the walls", map[string]any{"page": "The Sandstorm"}, "use action: update"},
		{"update missing", owner, ActionUpdate, "Gone", map[string]any{"page": "The Sandstorm"}, "no test called"},
		{"on successes without a total", owner, ActionCreate, "New", map[string]any{"page": "The Sandstorm", "results": []any{"Win, on successes"}}, "give `successes`"},
		{"two on failures", owner, ActionCreate, "New", map[string]any{"page": "The Sandstorm", "failures": 2, "results": []any{"A, on failures", "B, on failures"}}, "only one result"},
		{"same name twice", owner, ActionCreate, "New", map[string]any{"page": "The Sandstorm", "results": []any{"A", "a"}}, "two results"},
		{"bad option", owner, ActionCreate, "New", map[string]any{"page": "The Sandstorm", "results": []any{"A, on luck"}}, "not `on successes`"},
		{"total too big", owner, ActionCreate, "New", map[string]any{"page": "The Sandstorm", "successes": 31}, "0 to 30"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := k.Plan(context.Background(), camp, tc.a, rec("test", tc.action, tc.test, tc.fields, ""))
			if !strings.Contains(p.Error, tc.want) {
				t.Fatalf("error %q, want it to say %q", p.Error, tc.want)
			}
		})
	}
}

func TestTestKind_WarnsWithoutTheBlock(t *testing.T) {
	k, svc := testKindFixture()
	svc.noBlock["p-storm"] = true
	p := k.Plan(context.Background(), camp, owner, rec("test", ActionCreate, "New", map[string]any{"page": "The Sandstorm"}, ""))
	if p.Error != "" || len(p.Warnings) != 1 || !strings.Contains(p.Warnings[0], "Customizer") {
		t.Fatalf("plan %+v", p)
	}
}

// The export is blocks that plan cleanly when sent back.
func TestTestKind_ExportRoundTrips(t *testing.T) {
	k, _ := testKindFixture()
	out, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	recs := parseBlocks(out)
	if len(recs) != 1 {
		t.Fatalf("exported %q", out)
	}
	if p := k.Plan(context.Background(), camp, owner, recs[0]); p.Error != "" {
		t.Fatalf("exported block plans with %q:\n%s", p.Error, out)
	}
	if s, _ := k.Export(context.Background(), camp, player); s != "" {
		t.Fatal("a player's export lists tests")
	}
}
