package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

// memRecipes is the smallest homebrew.Repository that lets a recipe go
// through the real service, and so through every registered CheckFunc,
// without exporting anything from the homebrew package.
type memRecipes struct{ stored map[string]homebrew.Recipe }

func (m *memRecipes) List(context.Context, string) ([]homebrew.Recipe, error) { return nil, nil }
func (m *memRecipes) Get(_ context.Context, _, id string) (homebrew.Recipe, bool, error) {
	r, ok := m.stored[id]
	return r, ok, nil
}
func (m *memRecipes) Count(context.Context, string) (int, error) { return len(m.stored), nil }
func (m *memRecipes) Create(_ context.Context, r homebrew.Recipe) error {
	m.stored[r.ID] = r
	return nil
}
func (m *memRecipes) Update(context.Context, homebrew.Recipe, int) (bool, error) { return false, nil }
func (m *memRecipes) Delete(context.Context, string, string) (bool, error)       { return false, nil }

func trackerService(t *testing.T) (homebrew.Service, *homebrew.Registry) {
	t.Helper()
	reg := homebrew.NewRegistry()
	if err := registerTrackerParts(reg, nil); err != nil {
		t.Fatal(err)
	}
	return homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg), reg
}

var hbDirector = homebrew.Viewer{UserID: "dm", Director: true}

// saveDoc runs a doc through Create and returns the stored recipe.
func saveDoc(t *testing.T, svc homebrew.Service, doc homebrew.Doc) (*homebrew.Recipe, error) {
	t.Helper()
	return svc.Create(context.Background(), "camp", hbDirector, homebrew.RecipeInput{Name: "R", Doc: doc})
}

func hbCode(err error) int {
	var ae *apperror.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return 0
}

func TestRegisterTrackerParts(t *testing.T) {
	reg := homebrew.NewRegistry()
	if err := registerTrackerParts(reg, nil); err != nil {
		t.Fatal(err)
	}
	cat := reg.Catalog()
	names := func(es []homebrew.CatalogEntry) []string {
		out := []string{}
		for _, e := range es {
			out = append(out, e.Name)
		}
		return out
	}
	tests := []struct {
		what string
		got  []string
		want []string
	}{
		{"parts", names(cat.Parts), []string{"clock", "tracker"}},
		{"events", names(cat.Events), []string{"tracker.reached"}},
		{"actions", names(cat.Actions), []string{"set", "step", "tick"}},
	}
	for _, tt := range tests {
		if strings.Join(tt.got, ",") != strings.Join(tt.want, ",") {
			t.Errorf("%s = %v, want %v", tt.what, tt.got, tt.want)
		}
	}
	for _, a := range cat.Actions {
		want := "tracker,clock"
		if a.Name == "tick" {
			want = "clock"
		}
		if strings.Join(a.PartKinds, ",") != want {
			t.Errorf("action %s works on %v, want %s", a.Name, a.PartKinds, want)
		}
	}

	// The Workshop draws its inspector from these fields; each must name the
	// arg key the matching Check reads.
	fields := map[string]string{}
	for _, e := range append(append([]homebrew.CatalogEntry{}, cat.Events...), cat.Actions...) {
		for _, f := range e.Fields {
			fields[e.Name] += f.Name + ":" + f.Type
		}
	}
	wantFields := map[string]string{"tracker.reached": "value:whole", "step": "by:whole", "set": "to:whole"}
	if fmt.Sprint(fields) != fmt.Sprint(wantFields) {
		t.Errorf("fields = %v, want %v", fields, wantFields)
	}

	// The live preview acts trackers out from these, so each must match what
	// the rule does at the table.
	previews := map[string]string{}
	for _, l := range [][]homebrew.CatalogEntry{cat.Parts, cat.Events, cat.Actions} {
		for _, e := range l {
			previews[e.Name] = fmt.Sprint(e.Preview)
		}
	}
	wantPreviews := map[string]string{"tracker": "shape", "clock": "shape", "tracker.reached": "&{reach value}",
		"step": "&{add by}", "set": "&{set to}", "tick": "&{add }"}
	if fmt.Sprint(previews) != fmt.Sprint(wantPreviews) {
		t.Errorf("previews = %v, want %v", previews, wantPreviews)
	}

	t.Run("a second call fails as a duplicate", func(t *testing.T) {
		if err := registerTrackerParts(reg, nil); err == nil {
			t.Error("registering twice succeeded")
		}
	})
}

func TestTrackerPartChecks(t *testing.T) {
	svc, _ := trackerService(t)
	tests := []struct {
		name string
		kind string
		data string
		want string // normalised data; empty means refused
	}{
		{"count accepted", "tracker", `{"shape":"count","max":3}`, `{"shape":"count","max":3}`},
		{"empty shape defaults to count", "tracker", `{"max":3}`, `{"shape":"count","max":3}`},
		{"absent data defaults to unbounded count", "tracker", ``, `{"shape":"count"}`},
		{"null data defaults to unbounded count", "tracker", `null`, `{"shape":"count"}`},
		{"tracker may be steps", "tracker", `{"shape":"steps","steps":[{"name":"a"},{"name":"b"}]}`, `{"shape":"steps","steps":[{"name":"a","text":""},{"name":"b","text":""}]}`},
		{"unknown field refused", "tracker", `{"shape":"count","colour":"red"}`, ""},
		{"wrong type refused", "tracker", `{"shape":"count","max":"3"}`, ""},
		{"min above max refused", "tracker", `{"shape":"count","min":5,"max":3}`, ""},
		{"unknown shape refused", "tracker", `{"shape":"dial"}`, ""},
		{"clock forced", "clock", `{"max":4}`, `{"shape":"clock","max":4}`},
		{"clock explicit ok", "clock", `{"shape":"clock","max":4}`, `{"shape":"clock","max":4}`},
		{"clock refuses steps", "clock", `{"shape":"steps","steps":[{"name":"a"},{"name":"b"}]}`, ""},
		{"clock refuses count", "clock", `{"shape":"count","max":4}`, ""},
		{"clock needs segments", "clock", `{}`, ""},
		{"clock segment range", "clock", `{"max":1}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := homebrew.Part{Key: "p", Kind: tt.kind, Label: "P", See: homebrew.SeeParty}
			if tt.data != "" {
				p.Data = json.RawMessage(tt.data)
			}
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{p}})
			if tt.want == "" {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := string(r.Doc.Parts[0].Data); got != tt.want {
				t.Errorf("data = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestTrackerRuleChecks(t *testing.T) {
	svc, _ := trackerService(t)
	parts := []homebrew.Part{
		{Key: "heat", Kind: "tracker", Label: "Heat", Data: json.RawMessage(`{"shape":"count"}`)},
		{Key: "doom", Kind: "clock", Label: "Doom", Data: json.RawMessage(`{"max":6}`)},
	}
	when := func(args string) homebrew.Rule {
		r := homebrew.Rule{When: homebrew.When{Event: "tracker.reached", Part: "heat"}, Then: []homebrew.Do{{Action: "tick", Part: "doom"}}}
		if args != "" {
			r.When.Args = json.RawMessage(args)
		}
		return r
	}
	do := func(action, part, args string) homebrew.Rule {
		d := homebrew.Do{Action: action, Part: part}
		if args != "" {
			d.Args = json.RawMessage(args)
		}
		return homebrew.Rule{When: homebrew.When{Event: "tracker.reached", Part: "heat", Args: json.RawMessage(`{"value":1}`)}, Then: []homebrew.Do{d}}
	}
	tests := []struct {
		name string
		rule homebrew.Rule
		ok   bool
	}{
		{"reached with value", when(`{"value":5}`), true},
		{"reached negative value", when(`{"value":-5}`), true},
		{"reached zero value", when(`{"value":0}`), true},
		{"reached needs value", when(``), false},
		{"reached empty object", when(`{}`), false},
		{"reached null value", when(`{"value":null}`), false},
		{"reached string value", when(`{"value":"5"}`), false},
		{"reached unknown field", when(`{"value":5,"also":1}`), false},
		{"reached out of range", when(`{"value":1000001}`), false},
		{"reached on a clock part", homebrew.Rule{When: homebrew.When{Event: "tracker.reached", Part: "doom", Args: json.RawMessage(`{"value":3}`)}, Then: []homebrew.Do{{Action: "tick", Part: "doom"}}}, true},

		{"step by", do("step", "heat", `{"by":2}`), true},
		{"step negative", do("step", "heat", `{"by":-2}`), true},
		{"step needs by", do("step", "heat", ``), false},
		{"step wrong field", do("step", "heat", `{"to":2}`), false},
		{"step out of range", do("step", "heat", `{"by":-1000001}`), false},
		{"step on a clock", do("step", "doom", `{"by":1}`), true},

		{"set to", do("set", "heat", `{"to":4}`), true},
		{"set needs to", do("set", "heat", `{}`), false},
		{"set wrong field", do("set", "heat", `{"by":4}`), false},

		{"tick a clock", do("tick", "doom", ``), true},
		{"tick explicit empty args", do("tick", "doom", `{}`), true},
		{"tick refuses args", do("tick", "doom", `{"by":1}`), false},
		{"tick refuses a tracker part", do("tick", "heat", ``), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := saveDoc(t, svc, homebrew.Doc{Parts: parts, Rules: []homebrew.Rule{tt.rule}})
			if !tt.ok {
				if hbCode(err) != 422 {
					t.Fatalf("err = %v, want 422", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.rule.Then[0].Action == "tick" {
				if got := string(r.Doc.Rules[0].Then[0].Args); got != `{}` {
					t.Errorf("tick args = %s, want {}", got)
				}
			}
		})
	}
}

// ownedTrackers is a trackers.Service that records owned-kind syncs; every
// other method is the nil embedded interface and would panic if called.
type ownedTrackers struct {
	trackers.Service
	synced map[string][]trackers.OwnedKind
	refuse error
}

func (o *ownedTrackers) CheckOwned(_ context.Context, _ string, _ string, _ []trackers.OwnedKind) error {
	return o.refuse
}

func (o *ownedTrackers) SyncOwned(_ context.Context, _ string, prefix string, want []trackers.OwnedKind) error {
	o.synced[prefix] = want
	return nil
}

func TestTrackerPartsBecomeKinds(t *testing.T) {
	reg := homebrew.NewRegistry()
	tk := &ownedTrackers{synced: map[string][]trackers.OwnedKind{}}
	if err := registerTrackerParts(reg, tk); err != nil {
		t.Fatal(err)
	}
	svc := homebrew.NewService(&memRecipes{stored: map[string]homebrew.Recipe{}}, reg)
	r, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{
		{Key: "need", Kind: "tracker", Label: "Need", See: homebrew.SeeParty, Data: json.RawMessage(`{"max":5}`)},
		{Key: "doom", Kind: "clock", Label: "Doom", See: homebrew.SeeName, Data: json.RawMessage(`{"max":6}`)},
	}})
	if err != nil {
		t.Fatal(err)
	}
	tp, cp := "homebrew:"+r.ID+":tracker:", "homebrew:"+r.ID+":clock:"
	got := fmt.Sprint(tk.synced[tp], tk.synced[cp])
	five, six := 5, 6
	want := fmt.Sprint(
		[]trackers.OwnedKind{{Owner: tp + "need", Name: "Need", Shape: trackers.Shape{Shape: "count", Max: &five}, Visibility: trackers.SeeEveryone}},
		[]trackers.OwnedKind{{Owner: cp + "doom", Name: "Doom", Shape: trackers.Shape{Shape: "clock", Max: &six}, Visibility: trackers.SeeDirectors}})
	// Pointers print as addresses, so compare the parts that matter.
	if len(tk.synced) != 2 || tk.synced[tp][0].Owner != tp+"need" || tk.synced[tp][0].Visibility != trackers.SeeEveryone ||
		*tk.synced[tp][0].Shape.Max != 5 || tk.synced[cp][0].Shape.Shape != "clock" || tk.synced[cp][0].Visibility != trackers.SeeDirectors {
		t.Fatalf("synced %s, want %s", got, want)
	}

	t.Run("the trackers plugin's refusal reaches the Director", func(t *testing.T) {
		tk.refuse = apperror.NewValidation("there is already a tracker kind called Need; rename the part")
		_, err := saveDoc(t, svc, homebrew.Doc{Parts: []homebrew.Part{{Key: "n", Kind: "tracker", Label: "Need", See: homebrew.SeeParty}}})
		if hbCode(err) != 422 || !strings.Contains(err.Error(), "rename the part") {
			t.Fatalf("err = %v", err)
		}
	})
}
