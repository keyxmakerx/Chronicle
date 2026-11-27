package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/homebrew"
	"github.com/keyxmakerx/chronicle/internal/plugins/trackers"
)

func TestCheckButtonPressed(t *testing.T) {
	tests := []struct {
		name, in, want, wantErr string
	}{
		{name: "a name", in: `{"button":"Dose"}`, want: `{"button":"Dose"}`},
		{name: "trimmed and single-spaced", in: `{"button":"  Calm   down "}`, want: `{"button":"Calm down"}`},
		{name: "at the limit", in: `{"button":"` + strings.Repeat("d", buttonNameChars) + `"}`, want: `{"button":"` + strings.Repeat("d", buttonNameChars) + `"}`},
		{name: "a new When starts empty", in: `{}`, wantErr: "the button's name is required"},
		{name: "only spaces", in: `{"button":"   "}`, wantErr: "the button's name is required"},
		{name: "too long", in: `{"button":"` + strings.Repeat("d", buttonNameChars+1) + `"}`, wantErr: "too long (at most 40 characters)"},
		{name: "a control character", in: `{"button":"Do\u0007se"}`, wantErr: "not allowed"},
		{name: "an unknown arg", in: `{"button":"Dose","part":"x"}`, wantErr: "doesn't recognise"},
		{name: "not text", in: `{"button":3}`, wantErr: "doesn't recognise"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := checkButtonPressed(context.Background(), "camp", json.RawMessage(tt.in))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestButtonMatches(t *testing.T) {
	tests := []struct {
		name, args, payload string
		want                bool
	}{
		{"same name", `{"button":"Dose"}`, `{"button":"Dose"}`, true},
		{"another case", `{"button":"Dose"}`, `{"button":"DOSE"}`, true},
		{"other spacing", `{"button":"Calm down"}`, `{"button":" calm  down"}`, true},
		{"another button", `{"button":"Dose"}`, `{"button":"Calm"}`, false},
		{"no payload", `{"button":"Dose"}`, ``, false},
		{"a rule with no name", `{}`, `{"button":""}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buttonMatches(json.RawMessage(tt.args), json.RawMessage(tt.payload))
			if err != nil || got != tt.want {
				t.Errorf("got %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	if _, err := buttonMatches(json.RawMessage(`{"button":"Dose"}`), json.RawMessage(`[`)); err == nil {
		t.Error("a broken payload should be an error")
	}
}

// doseTrackers is the trackers service as a Dose press meets it: kinds sync
// without complaint, and every logged change is recorded.
type doseTrackers struct {
	trackers.Service
	logged []string
}

func (d *doseTrackers) CheckOwned(context.Context, string, string, []trackers.OwnedKind) error {
	return nil
}
func (d *doseTrackers) SyncOwned(context.Context, string, string, []trackers.OwnedKind) error {
	return nil
}
func (d *doseTrackers) LogOwned(_ context.Context, _, _, owner string, _ trackers.Viewer, op string, amount int, note string) ([]trackers.TrackerView, error) {
	d.logged = append(d.logged, fmt.Sprintf("%s %s %d: %s", owner[strings.LastIndex(owner, ":")+1:], op, amount, note))
	return []trackers.TrackerView{{Value: amount}}, nil
}

// doseRecipes lists what it stores, so the drawer can find the buttons.
type doseRecipes struct{ *memRecipes }

func (l doseRecipes) List(context.Context, string) ([]homebrew.Recipe, error) {
	out := []homebrew.Recipe{}
	for _, r := range l.stored {
		out = append(out, r)
	}
	return out, nil
}

// memRuns keeps runs just well enough for a press that runs at once.
type memRuns struct{ runs []homebrew.Run }

func (m *memRuns) CreateRun(_ context.Context, r homebrew.Run) error {
	m.runs = append(m.runs, r)
	return nil
}
func (m *memRuns) GetRun(context.Context, string, string) (homebrew.Run, bool, error) {
	return homebrew.Run{}, false, nil
}
func (m *memRuns) FinishRun(context.Context, homebrew.Run, string) (bool, error) { return false, nil }
func (m *memRuns) ListRuns(context.Context, string, bool, int) ([]homebrew.Run, error) {
	return nil, nil
}
func (m *memRuns) PruneRuns(context.Context, string, int) error { return nil }
func (m *memRuns) ListAsks(context.Context, string, string, int) ([]homebrew.Run, error) {
	return nil, nil
}

// A "Dose" button sets two trackers to 0 at once, and each log line says the
// button and the recipe.
func TestDoseButtonSetsTrackers(t *testing.T) {
	ctx := context.Background()
	tk := &doseTrackers{}
	reg := homebrew.NewRegistry()
	if err := registerTrackerParts(reg, tk); err != nil {
		t.Fatal(err)
	}
	for _, s := range workshopSources {
		if s.Label == "drawer buttons" {
			if err := s.Register(reg, workshopDeps{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	svc := homebrew.NewService(doseRecipes{&memRecipes{stored: map[string]homebrew.Recipe{}}}, reg, nil, &memRuns{})
	shape := json.RawMessage(`{"shape":"count","min":0,"max":5}`)
	to0 := json.RawMessage(`{"to":0}`)
	doc := homebrew.Doc{
		Parts: []homebrew.Part{{Key: "need", Kind: "tracker", Label: "Need", Data: shape}, {Key: "dread", Kind: "tracker", Label: "Dread", Data: shape}},
		Rules: []homebrew.Rule{{When: homebrew.When{Event: buttonPressedEvent, Args: json.RawMessage(`{"button":" Dose "}`)}, Timing: homebrew.TimingNow,
			Then: []homebrew.Do{{Action: "set", Part: "need", Args: to0}, {Action: "set", Part: "dread", Args: to0}}}},
	}
	if _, err := svc.Create(ctx, "camp", hbDirector, homebrew.RecipeInput{Name: "Remnant need", Doc: doc}); err != nil {
		t.Fatal(err)
	}
	d, err := svc.Drawer(ctx, "camp", hbDirector)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Presses) != 1 || d.Presses[0].Label != "Dose" || d.Presses[0].Rules != 1 {
		t.Fatalf("presses = %+v", d.Presses)
	}
	if _, err := svc.Press(ctx, "camp", buttonPressedEvent, "dose", hbDirector); err != nil {
		t.Fatal(err)
	}
	want := "need set 0: Dose, from Remnant need|dread set 0: Dose, from Remnant need"
	if got := strings.Join(tk.logged, "|"); got != want {
		t.Errorf("logged %q, want %q", got, want)
	}
	if _, err := svc.Press(ctx, "camp", buttonPressedEvent, "Heal", hbDirector); hbCode(err) != 404 {
		t.Errorf("an unnamed button: %v", err)
	}
}

// The event joins the Workshop with its arg and its named buttons, whatever
// else is healthy.
func TestButtonPressedRegisters(t *testing.T) {
	reg := homebrew.NewRegistry()
	for _, s := range workshopSources {
		if s.Label == "drawer buttons" {
			if err := s.Register(reg, workshopDeps{}); err != nil {
				t.Fatal(err)
			}
		}
	}
	ev := reg.Catalog().Events
	if len(ev) != 1 || ev[0].Name != buttonPressedEvent || ev[0].PressArg != "button" || len(ev[0].PartKinds) != 0 ||
		len(ev[0].Fields) != 1 || ev[0].Fields[0] != (homebrew.Field{Name: "button", Label: "Button", Type: homebrew.FieldText}) {
		t.Fatalf("catalog events = %+v", ev)
	}
}
