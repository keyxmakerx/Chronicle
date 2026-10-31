package crafting

import (
	"context"
	"encoding/json"
	"testing"
)

func TestCleanUpgrades(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"nothing", ``, `{"pieces":[]}`},
		{"not an object", `[1]`, `{"pieces":[]}`},
		{"a ladder: the first level is the start, the reached level stays on it", `{"pieces":[{"name":" Mortar ","have":9,"levels":[
			{"name":"Granite mortar","how":"bought","changes":"none at the start"},
			{"name":"Saltglass mortar","how":"recipe","changes":"Grinding takes one fewer turn.","note":"The Saltglass mortar recipe."},
			{"name":"Moonstone mortar","how":"stolen","rule":{"when":[{"kind":"moon","moon":"Pale","phase":"full"}],"then":{"kind":"fine"},"chance":"1 in 6","players":"secret","draft":true}}]}]}`,
			`{"pieces":[{"name":"Mortar","have":2,"levels":[{"name":"Granite mortar","how":"start"},` +
				`{"name":"Saltglass mortar","changes":"Grinding takes one fewer turn.","how":"recipe","note":"The Saltglass mortar recipe."},` +
				`{"name":"Moonstone mortar","rule":{"when":[{"kind":"moon","moon":"Pale","phase":"full"}],"then":{"kind":"fine"},"chance":"1 in 6","players":"book"},"how":"given"}]}]}`},
		{"a piece needs a name and a level", `{"pieces":[{"name":"","levels":[{"name":"x"}]},{"name":"Burner","levels":[]},{"name":"Flask","levels":[{"name":""}]}]}`, `{"pieces":[]}`},
		{"an unreadable rule leaves the level in words", `{"pieces":[{"name":"Burner","have":-1,"levels":[{"name":"Oil"},{"name":"Drake","rule":{"when":[{"kind":"comet"}]}}]}]}`,
			`{"pieces":[{"name":"Burner","have":0,"levels":[{"name":"Oil","how":"start"},{"name":"Drake","how":"given"}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := json.Marshal(CleanUpgrades(json.RawMessage(tc.in)))
			if string(got) != tc.want {
				t.Fatalf("CleanUpgrades =\n%s\nwant\n%s", got, tc.want)
			}
		})
	}
}

func TestRollProject_BenchUpgrades(t *testing.T) {
	plus := &Rule{When: []Cond{{Kind: "always"}}, Then: Effect{Kind: "points", Amount: 2}, Players: "book"}
	burner := Piece{Name: "Burner", Levels: []Level{{Name: "Oil burner", How: "start"}, {Name: "Drakefire burner", Rule: plus, How: "recipe"}}}
	cases := []struct {
		name  string
		have  int
		bench string
		total int
	}{
		{"at the bench, upgraded", 1, "lab", 11},
		{"at the bench, not yet upgraded", 0, "lab", 9},
		{"not at the bench", 1, "", 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newProjectFx()
			p := burner
			p.Have = tc.have
			fx.repo.benches["lab"] = Bench{ID: "lab", Name: "Tower lab", Upgrades: Upgrades{Pieces: []Piece{p}}, Version: 1}
			pv := fx.start(t)
			fx.ready(pv.ID, 1)
			fx.dice = []int{3, 4}
			res, err := fx.s.RollProject(context.Background(), "c", pv.ID, player, RollInput{Work: WorkSteady, Bench: tc.bench, Version: fx.store.projects[pv.ID].Version})
			if err != nil {
				t.Fatal(err)
			}
			if res.Total != tc.total {
				t.Fatalf("total = %d, want %d; terms %q", res.Total, tc.total, termTexts(res))
			}
		})
	}
}

func TestSaveUpgrades(t *testing.T) {
	s, repo, _ := newTestService()
	repo.benches["lab"] = Bench{ID: "lab", Name: "Tower lab", Version: 3}
	in := UpgradesInput{Upgrades: json.RawMessage(`{"pieces":[{"name":"Flask","have":1,"levels":[{"name":"Round"},{"name":"Etched","how":"bought"}]}]}`), Version: 3}
	if _, err := s.SaveUpgrades(context.Background(), "c", "lab", player, in); code(err) != 403 {
		t.Fatalf("a player saved upgrades: %v", err)
	}
	b, err := s.SaveUpgrades(context.Background(), "c", "lab", director, in)
	if err != nil {
		t.Fatal(err)
	}
	if b.Version != 4 || len(b.Upgrades.Pieces) != 1 || b.Upgrades.Pieces[0].Have != 1 {
		t.Fatalf("bench = %+v", b)
	}
	if _, err := s.SaveUpgrades(context.Background(), "c", "lab", director, in); code(err) != 409 {
		t.Fatalf("an old version saved: %v", err)
	}
}
