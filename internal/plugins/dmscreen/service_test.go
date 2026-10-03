package dmscreen

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/systems"
)

type fakeSystem struct {
	m     *systems.SystemManifest
	items []systems.ReferenceItem
}

func (f fakeSystem) Info() *systems.SystemManifest                { return f.m }
func (f fakeSystem) DataProvider() systems.DataProvider           { return f }
func (f fakeSystem) TooltipRenderer() systems.TooltipRenderer     { return nil }
func (f fakeSystem) List(string) ([]systems.ReferenceItem, error) { return f.items, nil }
func (f fakeSystem) Get(string, string) (*systems.ReferenceItem, error) {
	return nil, errors.New("unused")
}
func (f fakeSystem) Search(string) ([]systems.ReferenceItem, error) { return nil, nil }
func (f fakeSystem) Categories() []string                           { return nil }

type fakeSystemSource struct{ sys systems.System }

func (f fakeSystemSource) EnabledSystem(context.Context, string) systems.System { return f.sys }

type fakeParty struct {
	heroes []Hero
	err    error
}

func (f fakeParty) Heroes(context.Context, string, Viewer) ([]Hero, error) { return f.heroes, f.err }

type fakeDowntime struct {
	open, ok bool
	pending  int
	err      error
}

func (f fakeDowntime) Downtime(context.Context, string, Viewer) (bool, int, bool, error) {
	return f.open, f.pending, f.ok, f.err
}

func (f fakeDowntime) SetDowntime(_ context.Context, _ string, _ Viewer, open bool) (int, int, error) {
	if f.err != nil {
		return 0, 0, f.err
	}
	if open {
		return f.pending, 0, nil
	}
	return 0, 0, nil
}

type fakeFoundry struct {
	last      *time.Time
	connected bool
}

func (f fakeFoundry) FoundryPresence(string) (*time.Time, bool) { return f.last, f.connected }

type fakeHidden struct {
	revealed string
}

func (f *fakeHidden) HiddenCharacters(context.Context, string, Viewer, int) ([]Hidden, error) {
	return []Hidden{{ID: "e1", Name: "Captain Vosk"}}, nil
}
func (f *fakeHidden) Reveal(_ context.Context, id, _ string) (string, error) {
	f.revealed = id
	return "Captain Vosk", nil
}

var drawSteel = &systems.SystemManifest{
	Name: "Draw Steel",
	DMScreen: &systems.DMScreenDef{
		Party: []systems.DMScreenMeter{
			{Label: "Stamina", Current: "stamina_current", Max: "stamina_max", WarnBelow: 0.5},
			{Label: "Recoveries", Current: "recoveries", Max: "recoveries_max"},
			{LabelField: "heroic_resource_name", Current: "heroic_resource_current"},
		},
		HeroSubtitle:   "class",
		HeroConditions: "conditions_json",
		Conditions:     &systems.DMScreenConditions{Category: "rules-glossary", Property: "category", Value: "condition"},
	},
}

func TestBuild_Roles(t *testing.T) {
	svc := NewService(Sources{})
	tests := []struct {
		name    string
		role    int
		wantErr bool
	}{
		{"player refused", 1, true},
		{"scribe allowed", 2, false},
		{"owner allowed", 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Build(context.Background(), "c1", Viewer{UserID: "u", Role: tt.role})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if _, err := svc.Reveal(context.Background(), "e1", "c1", Viewer{Role: tt.role}); tt.wantErr && err == nil {
				t.Fatal("Reveal should refuse a player")
			}
		})
	}
}

func TestBuild_DowntimeOwnerOnlyToggle(t *testing.T) {
	tests := []struct {
		name       string
		role       int
		src        fakeDowntime
		wantNil    bool
		wantToggle bool
	}{
		{"owner can toggle", 3, fakeDowntime{open: true, ok: true, pending: 2}, false, true},
		{"scribe sees but can't toggle", 2, fakeDowntime{ok: true}, false, false},
		{"no armory hides it", 3, fakeDowntime{ok: false}, true, false},
		{"error hides it", 3, fakeDowntime{ok: true, err: errors.New("db down")}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewService(Sources{Downtime: tt.src}).Build(context.Background(), "c1", Viewer{Role: tt.role})
			if err != nil {
				t.Fatal(err)
			}
			if (v.Downtime == nil) != tt.wantNil {
				t.Fatalf("Downtime = %+v, wantNil %v", v.Downtime, tt.wantNil)
			}
			if v.Downtime != nil && v.Downtime.CanToggle != tt.wantToggle {
				t.Fatalf("CanToggle = %v, want %v", v.Downtime.CanToggle, tt.wantToggle)
			}
		})
	}
}

func TestBuild_PartyMeters(t *testing.T) {
	heroes := []Hero{
		{ID: "a", Name: "Aria", Fields: map[string]any{
			"stamina_current": float64(34), "stamina_max": "42",
			"recoveries": "6", "recoveries_max": float64(8),
			"heroic_resource_name": "Focus", "heroic_resource_current": float64(3),
			"class": "Tactician", "conditions_json": `["bleeding",{"name":"slowed"}]`,
		}},
		{ID: "b", Name: "Bren", Fields: map[string]any{"stamina_current": "11", "stamina_max": "39"}},
	}
	tests := []struct {
		name       string
		sys        systems.System
		wantFilled bool
		wantMeters []int
	}{
		{"draw steel declares meters", fakeSystem{m: drawSteel}, true, []int{3, 1}},
		{"system without dm_screen", fakeSystem{m: &systems.SystemManifest{Name: "Plain"}}, false, []int{0, 0}},
		{"no system", nil, false, []int{0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := Sources{Party: fakeParty{heroes: heroes}}
			if tt.sys != nil {
				src.System = fakeSystemSource{sys: tt.sys}
			}
			v, err := NewService(src).Build(context.Background(), "c1", Viewer{Role: 3})
			if err != nil {
				t.Fatal(err)
			}
			if v.PartyFilled != tt.wantFilled {
				t.Fatalf("PartyFilled = %v", v.PartyFilled)
			}
			for i, n := range tt.wantMeters {
				if got := len(v.Party[i].Meters); got != n {
					t.Fatalf("hero %d meters = %d, want %d", i, got, n)
				}
			}
			if tt.wantFilled {
				a := v.Party[0]
				if a.Subtitle != "Tactician" || strings.Join(a.Conditions, ",") != "Bleeding,Slowed" {
					t.Fatalf("Aria subtitle=%q conditions=%v", a.Subtitle, a.Conditions)
				}
				if v.Party[1].Subtitle != "" || v.Party[1].Conditions != nil {
					t.Fatalf("Bren has no class or conditions, got %+v", v.Party[1])
				}
			}
		})
	}
}

func TestBuildMeters(t *testing.T) {
	defs := drawSteel.DMScreen.Party
	got := buildMeters(defs, map[string]any{
		"stamina_current": "11", "stamina_max": float64(39),
		"heroic_resource_name": "Ferocity", "heroic_resource_current": float64(5),
		"recoveries": []any{"not", "a", "number"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d meters, want 2 (list-valued recoveries skipped): %+v", len(got), got)
	}
	st := got[0]
	if !st.HasMax || st.Percent != 28 || !st.Low || st.Current != "11" || st.Max != "39" {
		t.Fatalf("stamina meter wrong: %+v", st)
	}
	if got[1].Label != "Ferocity" || got[1].HasMax || got[1].Current != "5" {
		t.Fatalf("resource meter wrong: %+v", got[1])
	}
	// Current above max clamps the bar instead of overflowing it.
	over := buildMeters(defs[:1], map[string]any{"stamina_current": "50", "stamina_max": "40"})
	if over[0].Percent != 100 || over[0].Low {
		t.Fatalf("overfull meter wrong: %+v", over[0])
	}
}

func TestPickConditions(t *testing.T) {
	items := []systems.ReferenceItem{
		{Name: "Slowed", Properties: map[string]any{"category": "condition"}, Description: "Your speed is 2 unless {@condition dazed|you are dazed}."},
		{Name: "Bleeding", Properties: map[string]any{"category": "condition"}, Summary: "<b>Lose</b> stamina when you {@action act}."},
		{Name: "Shift", Properties: map[string]any{"category": "movement"}, Description: "Move without provoking."},
	}
	got := pickConditions(items, drawSteel.DMScreen.Conditions)
	if len(got) != 2 || got[0].Name != "Bleeding" || got[1].Name != "Slowed" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Text != "Lose stamina when you act." {
		t.Fatalf("markup not flattened: %q", got[0].Text)
	}
	if got[1].Text != "Your speed is 2 unless you are dazed." {
		t.Fatalf("display override not used: %q", got[1].Text)
	}
	all := pickConditions(items, &systems.DMScreenConditions{Category: "x"})
	if len(all) != 3 {
		t.Fatalf("no filter should keep all, got %d", len(all))
	}
}

func TestBuild_FoundryAndFailures(t *testing.T) {
	src := Sources{
		Foundry: fakeFoundry{},
		Party:   fakeParty{err: errors.New("boom")},
		Hidden:  &fakeHidden{},
	}
	v, err := NewService(src).Build(context.Background(), "c1", Viewer{Role: 2})
	if err != nil {
		t.Fatalf("a failing source must not fail the screen: %v", err)
	}
	if !v.Foundry.NeverSeen || v.Foundry.Connected {
		t.Fatalf("Foundry = %+v", v.Foundry)
	}
	if len(v.Party) != 0 || len(v.Hidden) != 1 {
		t.Fatalf("party %d hidden %d", len(v.Party), len(v.Hidden))
	}
}

func TestReveal(t *testing.T) {
	h := &fakeHidden{}
	name, err := NewService(Sources{Hidden: h}).Reveal(context.Background(), "e1", "c1", Viewer{Role: 2})
	if err != nil || name != "Captain Vosk" || h.revealed != "e1" {
		t.Fatalf("name %q err %v revealed %q", name, err, h.revealed)
	}
	if _, err := NewService(Sources{Hidden: h}).Reveal(context.Background(), "", "c1", Viewer{Role: 2}); err == nil {
		t.Fatal("empty id should fail")
	}
}

func TestHeroConditions(t *testing.T) {
	many := make([]any, 20)
	for i := range many {
		many[i] = "dazed"
	}
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{"json list of ids", `["frightened","dazed-1"]`, "Frightened|Dazed 1"},
		{"foundry objects", []any{map[string]any{"name": "grabbed"}, map[string]any{"severity": "x"}}, "Grabbed"},
		{"comma string", "Prone, taunted ,", "Prone|Taunted"},
		{"broken json", `["bleeding"`, ""},
		{"empty", "", ""},
		{"number", float64(3), ""},
		{"nil", nil, ""},
		{"capped", many, strings.TrimSuffix(strings.Repeat("Dazed|", maxHeroConditions), "|")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(heroConditions(tt.raw), "|"); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetDowntime(t *testing.T) {
	tests := []struct {
		name    string
		src     Sources
		role    int
		open    bool
		wantErr bool
		want    DowntimeResult
	}{
		{"owner opens and waiting requests apply", Sources{Downtime: fakeDowntime{ok: true, pending: 2}}, 3, true, false, DowntimeResult{Open: true, Applied: 2}},
		{"owner closes", Sources{Downtime: fakeDowntime{ok: true}}, 3, false, false, DowntimeResult{}},
		{"scribe refused", Sources{Downtime: fakeDowntime{ok: true}}, 2, true, true, DowntimeResult{}},
		{"no armory", Sources{}, 3, true, true, DowntimeResult{}},
		{"armory error passes through", Sources{Downtime: fakeDowntime{err: errors.New("boom")}}, 3, true, true, DowntimeResult{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewService(tt.src).SetDowntime(context.Background(), "c1", Viewer{Role: tt.role}, tt.open)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}
