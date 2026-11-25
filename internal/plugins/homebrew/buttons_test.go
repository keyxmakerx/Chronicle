package homebrew

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// matchButton is a named-button Match as an owner writes one: the rule's
// button against the pressed one, without regard to case.
func matchButton(args, payload json.RawMessage) (bool, error) {
	var a, p struct{ Button string }
	if err := json.Unmarshal(args, &a); err != nil {
		return false, err
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, err
	}
	return strings.EqualFold(a.Button, p.Button), nil
}

var buttonField = []Field{{Name: "button", Label: "Button", Type: FieldText}}

func TestRegistryPressArg(t *testing.T) {
	whole := []Field{{Name: "button", Label: "Button", Type: FieldWhole}}
	pressed := func(context.Context, string, Viewer) error { return nil }
	tests := []struct {
		name    string
		def     EventDef
		wantErr bool
	}{
		{"a text field from no part", EventDef{Fields: buttonField, PressArg: "button", Match: matchButton}, false},
		{"not one of its fields", EventDef{Fields: buttonField, PressArg: "name", Match: matchButton}, true},
		{"a whole-number field", EventDef{Fields: whole, PressArg: "button", Match: matchButton}, true},
		{"from a part", EventDef{PartKinds: []string{"widget"}, Fields: buttonField, PressArg: "button", Match: matchButton}, true},
		{"with Press", EventDef{Fields: buttonField, PressArg: "button", Press: "Press", Match: matchButton}, true},
		{"with Pressed", EventDef{Fields: buttonField, PressArg: "button", Pressed: pressed, Match: matchButton}, true},
		{"with no Match", EventDef{Fields: buttonField, PressArg: "button"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.def.Name, tt.def.Label, tt.def.Check = "button.pressed", "A button", okCheck
			r := NewRegistry()
			err := r.RegisterEvent(tt.def)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if c := r.Catalog(); len(c.Events) != 1 || c.Events[0].PressArg != "button" {
					t.Errorf("catalog = %+v", c.Events)
				}
			}
		})
	}
}

// buttonRig is the run rig with a named-button event, so the drawer offers
// one button per name the saved rules use.
func buttonRig(t *testing.T) *runRig {
	t.Helper()
	rg := newRunRig(t)
	if err := rg.svc.reg.RegisterEvent(EventDef{Name: "button.pressed", Label: "A drawer button is pressed", Fields: buttonField,
		PressArg: "button", Check: okCheck, Match: matchButton}); err != nil {
		t.Fatal(err)
	}
	return rg
}

func pressedRule(name, timing string, then ...Do) Rule {
	args, _ := json.Marshal(map[string]string{"button": name})
	return Rule{When: When{Event: "button.pressed", Args: args}, Timing: timing, Then: then}
}

func TestDrawerNamedButtons(t *testing.T) {
	ctx := context.Background()
	rg := buttonRig(t)
	add := Do{Action: "add", Part: "a"}
	rg.save(t, "A Dose", []Part{counter("a")},
		pressedRule("Dose", TimingNow, add), pressedRule("  dose ", TimingNow, add), pressedRule("Calm  down", TimingNow, add))
	rg.save(t, "B Rest", []Part{counter("a")},
		pressedRule("DOSE", TimingNow, add), pressedRule("", TimingNow, add), pressedRule(strings.Repeat("x", MaxNameChars+1), TimingNow, add),
		Rule{When: When{Event: "rest"}, Timing: TimingNow, Then: []Do{add}})
	d, err := rg.svc.Drawer(ctx, camp, dm)
	if err != nil {
		t.Fatal(err)
	}
	want := []Press{
		{Event: "rest", Label: "Log a rest", Rules: 1},
		{Event: "button.pressed", Label: "Dose", Button: "Dose", Arg: "button", Rules: 3},
		{Event: "button.pressed", Label: "Calm down", Button: "Calm down", Arg: "button", Rules: 1},
	}
	if len(d.Presses) != len(want) {
		t.Fatalf("presses = %+v", d.Presses)
	}
	for i := range want {
		if d.Presses[i] != want[i] {
			t.Errorf("press %d = %+v, want %+v", i, d.Presses[i], want[i])
		}
	}

	var b strings.Builder
	writeDrawer(&b, d)
	for _, line := range []string{
		"- Log a rest (`rest`): starts 1 saved rules",
		"- Dose (`button.pressed`, button \"Dose\"): starts 3 saved rules",
		"- Calm down (`button.pressed`, button \"Calm down\"): starts 1 saved rules",
	} {
		if !strings.Contains(b.String(), line) {
			t.Errorf("AI drawer lacks %q:\n%s", line, b.String())
		}
	}
	if tail := catalogTail(CatalogEntry{Fields: buttonField, PressArg: "button"}); !strings.Contains(tail, "`button` (Button, text)") ||
		!strings.Contains(tail, "each distinct `button` among the saved rules is a drawer button of that name") {
		t.Errorf("catalog tail = %q", tail)
	}
}

func TestPressNamedButton(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) *runRig {
		rg := buttonRig(t)
		rg.save(t, "Remnant need", []Part{counter("a"), counter("b")},
			pressedRule("Dose", TimingNow, Do{Action: "add", Part: "a"}, Do{Action: "add", Part: "b"}),
			pressedRule("Calm", TimingNow, Do{Action: "add", Part: "b"}),
			pressedRule("dose", TimingHeld, Do{Action: "add", Part: "a"}))
		return rg
	}

	t.Run("fires only the rules with that name, any case, noting the button", func(t *testing.T) {
		rg := setup(t)
		d, err := rg.svc.Press(ctx, camp, "button.pressed", " DOSE ", dm)
		if err != nil {
			t.Fatal(err)
		}
		if rg.values["a"] != 1 || rg.values["b"] != 1 {
			t.Fatalf("values = %v; Calm must not run", rg.values)
		}
		for _, a := range rg.acts {
			if a.Note != "Dose, from Remnant need" {
				t.Errorf("note = %q", a.Note)
			}
		}
		if len(d.Waiting) != 1 || len(d.Recent) != 1 {
			t.Fatalf("drawer = %+v", d)
		}
		held := rg.runs.runs[d.Waiting[0].ID]
		if string(held.Payload) != `{"button":"Dose"}` {
			t.Errorf("held payload = %s, want the first spelling", held.Payload)
		}

		// A held run keeps its button for the note when it runs later.
		rg.acts = nil
		if _, err := rg.svc.RunHeld(ctx, camp, d.Waiting[0].ID, dm); err != nil {
			t.Fatal(err)
		}
		if len(rg.acts) != 1 || rg.acts[0].Note != "Dose, from Remnant need" {
			t.Errorf("held acts = %+v", rg.acts)
		}
	})

	t.Run("a rule set off by another keeps its own note", func(t *testing.T) {
		rg := setup(t)
		rg.save(t, "Chain", []Part{counter("c"), counter("d")},
			pressedRule("Calm", TimingNow, Do{Action: "add", Part: "c"}),
			reached("c", 1, TimingNow, Do{Action: "add", Part: "d"}))
		if _, err := rg.svc.Press(ctx, camp, "button.pressed", "calm", dm); err != nil {
			t.Fatal(err)
		}
		var notes []string
		for _, a := range rg.acts {
			notes = append(notes, a.Part.Key+":"+a.Note)
		}
		want := "c:Calm, from Chain|d:Rule from Chain|b:Calm, from Remnant need"
		if strings.Join(notes, "|") != want {
			t.Errorf("notes = %v, want %s", notes, want)
		}
	})

	t.Run("refusals", func(t *testing.T) {
		rg := setup(t)
		tests := []struct {
			name, event, button string
			who                 Viewer
			code                int
		}{
			{"a name no rule uses", "button.pressed", "Heal", dm, 404},
			{"no name", "button.pressed", "", dm, 404},
			{"a name on a registered press", "rest", "Dose", dm, 404},
			{"a name on an event with no buttons", "quiet", "Dose", dm, 404},
			{"a player", "button.pressed", "Dose", player, 403},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				if _, err := rg.svc.Press(ctx, camp, tt.event, tt.button, tt.who); codeOf(err) != tt.code {
					t.Errorf("err = %v, want %d", err, tt.code)
				}
			})
		}
		if len(rg.runs.runs) != 0 || len(rg.values) != 0 {
			t.Errorf("a refused press ran something: %d runs, values %v", len(rg.runs.runs), rg.values)
		}
	})
}
