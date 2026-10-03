package maps

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

func ptrF(f float64) *float64 { return &f }
func ptrB(b bool) *bool       { return &b }

// storedDisplay is a map's display settings with one group of each kind set,
// the starting point for the merge cases.
func storedDisplay() *DisplaySettings {
	return &DisplaySettings{
		Frame: &FrameDisplay{Style: "arcane", Tint: ptrB(false)},
		Pins:  &PinDisplay{Style: PinStyleSeal, Size: PinSizeLarge},
		Grid:  &GridDisplay{Type: GridHex, Size: 80},
		Draw:  &DrawDisplay{Who: DrawWhoOwners},
	}
}

func TestMergeDisplaySettings(t *testing.T) {
	cases := []struct {
		name     string
		current  *DisplaySettings
		incoming string
		want     *DisplaySettings
		wantErr  bool
	}{
		{
			name:     "empty object preserves every group",
			current:  storedDisplay(),
			incoming: `{}`,
			want:     storedDisplay(),
		},
		{
			name:     "a group that is absent keeps its stored value",
			current:  storedDisplay(),
			incoming: `{"pins":{"style":"flag"}}`,
			want: func() *DisplaySettings {
				d := storedDisplay()
				d.Pins = &PinDisplay{Style: PinStyleFlag}
				return d
			}(),
		},
		{
			name:     "an explicit null clears only that group",
			current:  storedDisplay(),
			incoming: `{"grid":null}`,
			want: func() *DisplaySettings {
				d := storedDisplay()
				d.Grid = nil
				return d
			}(),
		},
		{
			name:     "clearing the last group leaves nothing (NULL column)",
			current:  &DisplaySettings{Draw: &DrawDisplay{Who: DrawWhoOwners}},
			incoming: `{"draw":null}`,
			want:     nil,
		},
		{
			name:     "a group replaces wholesale, it does not merge inside",
			current:  storedDisplay(),
			incoming: `{"pins":{"size":"s"}}`,
			want: func() *DisplaySettings {
				d := storedDisplay()
				d.Pins = &PinDisplay{Size: PinSizeSmall}
				return d
			}(),
		},
		{
			name:     "unknown top-level keys are dropped",
			current:  nil,
			incoming: `{"pins":{"style":"dot"},"bogus":{"x":1},"__proto__":1}`,
			want:     &DisplaySettings{Pins: &PinDisplay{Style: PinStyleDot}},
		},
		{
			name:     "unknown keys inside a group are dropped",
			current:  nil,
			incoming: `{"pins":{"style":"dot","glow":"yes"}}`,
			want:     &DisplaySettings{Pins: &PinDisplay{Style: PinStyleDot}},
		},
		{
			name:     "values equal to the default are not stored",
			current:  storedDisplay(),
			incoming: `{"pins":{"style":"drop","size":"m","labels":"hover"},"draw":{"who":"scribes"},"grid":{"type":"none","size":120},"frame":{"style":"","tint":true},"open":{"mode":"whole"}}`,
			want:     &DisplaySettings{},
		},
		{
			name:     "frame style is checked",
			incoming: `{"frame":{"style":"steampunk"}}`,
			wantErr:  true,
		},
		{
			name:     "every frame id is accepted",
			incoming: `{"frame":{"style":"futuristic"}}`,
			want:     &DisplaySettings{Frame: &FrameDisplay{Style: "futuristic"}},
		},
		{
			name:     "tint off is stored explicitly",
			incoming: `{"frame":{"tint":false}}`,
			want:     &DisplaySettings{Frame: &FrameDisplay{Tint: ptrB(false)}},
		},
		{name: "pin style is checked", incoming: `{"pins":{"style":"star"}}`, wantErr: true},
		{name: "pin size is checked", incoming: `{"pins":{"size":"xl"}}`, wantErr: true},
		{name: "pin labels are checked", incoming: `{"pins":{"labels":"sometimes"}}`, wantErr: true},
		{name: "grid type is checked", incoming: `{"grid":{"type":"triangles"}}`, wantErr: true},
		{name: "draw gate is checked", incoming: `{"draw":{"who":"everyone"}}`, wantErr: true},
		{name: "opening mode is checked", incoming: `{"open":{"mode":"wherever"}}`, wantErr: true},
		{name: "a wrong type in a group is a validation error", incoming: `{"pins":{"style":7}}`, wantErr: true},
		{name: "a group that is not an object is a validation error", incoming: `{"grid":"squares"}`, wantErr: true},
		{name: "the document must be an object", incoming: `[1,2]`, wantErr: true},
		{
			name:     "grid size is clamped to the top",
			incoming: `{"grid":{"type":"square","size":9999,"strength":30}}`,
			want:     &DisplaySettings{Grid: &GridDisplay{Type: GridSquare, Size: gridSizeMax}},
		},
		{
			name:     "grid size is clamped to the bottom, strength likewise",
			incoming: `{"grid":{"type":"hex","size":-4,"strength":500}}`,
			want:     &DisplaySettings{Grid: &GridDisplay{Type: GridHex, Size: gridSizeMin, Strength: gridStrengthMax}},
		},
		{
			name:     "a grid left at its default size and strength keeps only the type",
			incoming: `{"grid":{"type":"square","size":50,"strength":30}}`,
			want:     &DisplaySettings{Grid: &GridDisplay{Type: GridSquare}},
		},
		{
			name:     "a chosen spot is stored with its centre clamped to the picture",
			incoming: `{"open":{"mode":"spot","x":140,"y":-3,"zoom":1.5}}`,
			want:     &DisplaySettings{Open: &OpenDisplay{Mode: OpenSpot, X: ptrF(100), Y: ptrF(0), Zoom: ptrF(1.5)}},
		},
		{
			name:     "zoom is clamped",
			incoming: `{"open":{"mode":"spot","x":50,"y":50,"zoom":40}}`,
			want:     &DisplaySettings{Open: &OpenDisplay{Mode: OpenSpot, X: ptrF(50), Y: ptrF(50), Zoom: ptrF(openZoomMax)}},
		},
		{name: "a spot needs a centre and a zoom", incoming: `{"open":{"mode":"spot","x":50}}`, wantErr: true},
		{
			name:     "left-off mode drops any stray spot",
			incoming: `{"open":{"mode":"last","x":10,"y":10,"zoom":2}}`,
			want:     &DisplaySettings{Open: &OpenDisplay{Mode: OpenLast}},
		},
		{
			name:     "kind labels and colours are stored, colour lower-cased",
			incoming: `{"kinds":{"danger":{"label":"  Monsters ","color":"#FF0000"}}}`,
			want:     &DisplaySettings{Kinds: map[string]KindDisplay{"danger": {Label: "Monsters", Color: "#ff0000"}}},
		},
		{
			name:     "a kind left at its built-in label and colour is not stored",
			incoming: `{"kinds":{"danger":{"label":"Danger","color":"#dc2626"},"quest":{"label":"Hooks"}}}`,
			want:     &DisplaySettings{Kinds: map[string]KindDisplay{"quest": {Label: "Hooks"}}},
		},
		{
			name:     "a sixth kind cannot be added: unknown ids are dropped",
			incoming: `{"kinds":{"faction":{"label":"Factions","color":"#123456"}}}`,
			want:     &DisplaySettings{},
		},
		{name: "kind colour must be hex", incoming: `{"kinds":{"quest":{"color":"red"}}}`, wantErr: true},
		{name: "kind colour rejects css injection", incoming: `{"kinds":{"quest":{"color":"#fff;background:url(x)"}}}`, wantErr: true},
		{
			name:     "kind labels are clamped and stripped of control characters",
			incoming: `{"kinds":{"note":{"label":"` + strings.Repeat("x", 80) + `\u0007\n"}}}`,
			want:     &DisplaySettings{Kinds: map[string]KindDisplay{"note": {Label: strings.Repeat("x", kindLabelMaxRunes)}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergeDisplaySettings(tc.current, json.RawMessage(tc.incoming))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want a validation error, got %+v", got)
				}
				var ae *apperror.AppError
				if !errors.As(err, &ae) || ae.Type != "validation_error" {
					t.Fatalf("want a validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			// An empty-but-non-nil want means "nothing left", which the merge
			// reports as nil so the column goes back to NULL.
			want := tc.want
			if want != nil && want.IsEmpty() {
				want = nil
			}
			if !reflect.DeepEqual(got, want) {
				g, _ := json.Marshal(got)
				w, _ := json.Marshal(want)
				t.Errorf("got  %s\nwant %s", g, w)
			}
		})
	}
}

func TestMergeDisplaySettings_DoesNotMutateCurrent(t *testing.T) {
	cur := storedDisplay()
	before, _ := json.Marshal(cur)
	if _, err := MergeDisplaySettings(cur, json.RawMessage(`{"pins":null,"grid":{"type":"square"}}`)); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(cur)
	if string(before) != string(after) {
		t.Errorf("merge mutated the stored value: %s -> %s", before, after)
	}
}

func TestParseDisplaySettings(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name string
		raw  *string
		want *DisplaySettings
	}{
		{"nil column", nil, nil},
		{"blank", str("  "), nil},
		{"hand-edited garbage reads as defaults", str("{not json"), nil},
		{"empty object", str("{}"), nil},
		{"a stored document", str(`{"draw":{"who":"owners"}}`), &DisplaySettings{Draw: &DrawDisplay{Who: DrawWhoOwners}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseDisplaySettings(tc.raw); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveDisplay(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		r := ResolveDisplay(&Map{}, "")
		if r.Frame != "atlas" || r.FrameSource != "campaign" || !r.Tint {
			t.Errorf("frame defaults wrong: %+v", r)
		}
		if r.PinStyle != "drop" || r.PinSize != "m" || r.PinLabels != "hover" {
			t.Errorf("pin defaults wrong: %+v", r)
		}
		if r.GridType != "none" || r.OpenMode != "whole" || r.DrawWho != "scribes" {
			t.Errorf("other defaults wrong: %+v", r)
		}
		if len(r.Kinds) != 5 {
			t.Errorf("want the five kinds, got %d", len(r.Kinds))
		}
	})
	t.Run("a map follows the campaign frame until it picks its own", func(t *testing.T) {
		if got := ResolveDisplay(&Map{}, "gilded"); got.Frame != "gilded" || got.FrameSource != "campaign" {
			t.Errorf("want campaign gilded, got %+v", got)
		}
		m := &Map{Display: &DisplaySettings{Frame: &FrameDisplay{Style: "old"}}}
		if got := ResolveDisplay(m, "gilded"); got.Frame != "old" || got.FrameSource != "map" || got.CampaignFrame != "gilded" {
			t.Errorf("want map old over campaign gilded, got %+v", got)
		}
	})
	t.Run("an unknown campaign frame falls back to atlas", func(t *testing.T) {
		if got := ResolveDisplay(&Map{}, "rubbish"); got.Frame != "atlas" {
			t.Errorf("got %q", got.Frame)
		}
	})
	t.Run("kinds are renamed and recoloured without adding any", func(t *testing.T) {
		m := &Map{Display: &DisplaySettings{Kinds: map[string]KindDisplay{"danger": {Label: "Monsters", Color: "#111111"}, "location": {Label: "Towns"}}}}
		r := ResolveDisplay(m, "")
		byID := map[string]KindDisplay{}
		for _, k := range r.Kinds {
			byID[k.ID] = k
		}
		if byID["danger"].Label != "Monsters" || byID["danger"].Color != "#111111" {
			t.Errorf("danger not applied: %+v", byID["danger"])
		}
		if byID["location"].Label != "Towns" || byID["location"].Color != "#2563eb" {
			t.Errorf("location should keep its colour: %+v", byID["location"])
		}
		if len(r.Kinds) != 5 {
			t.Errorf("kinds grew to %d", len(r.Kinds))
		}
	})
	t.Run("kind defaults are not mutated by a resolve", func(t *testing.T) {
		m := &Map{Display: &DisplaySettings{Kinds: map[string]KindDisplay{"note": {Label: "Lore"}}}}
		ResolveDisplay(m, "")
		if KindDefaults[4].Label != "Notes" {
			t.Errorf("shared defaults were mutated: %+v", KindDefaults[4])
		}
	})
	t.Run("a spot without coordinates opens on the whole map", func(t *testing.T) {
		m := &Map{Display: &DisplaySettings{Open: &OpenDisplay{Mode: OpenSpot}}}
		if got := ResolveDisplay(m, ""); got.OpenMode != OpenWhole {
			t.Errorf("got %q", got.OpenMode)
		}
	})
	t.Run("a stored spot is carried through", func(t *testing.T) {
		m := &Map{Display: &DisplaySettings{Open: &OpenDisplay{Mode: OpenSpot, X: ptrF(20), Y: ptrF(30), Zoom: ptrF(1)}}}
		got := ResolveDisplay(m, "")
		if got.OpenMode != OpenSpot || got.OpenX != 20 || got.OpenY != 30 || got.OpenZoom != 1 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestIsValidFrame(t *testing.T) {
	for _, id := range []string{"atlas", "arcane", "old", "modern", "futuristic", "gilded"} {
		if !IsValidFrame(id) {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"", "Atlas", "future", "atlas ", "../x"} {
		if IsValidFrame(id) {
			t.Errorf("%q should be invalid", id)
		}
	}
	if len(FrameStyles) != 6 || FrameStyles[0].ID != DefaultFrame {
		t.Errorf("the six frames start with the default: %+v", FrameStyles)
	}
}

func TestMapDrawWho(t *testing.T) {
	cases := []struct {
		name string
		m    *Map
		want string
	}{
		{"nil map", nil, DrawWhoScribes},
		{"no settings", &Map{}, DrawWhoScribes},
		{"no draw group", &Map{Display: &DisplaySettings{}}, DrawWhoScribes},
		{"owners", &Map{Display: &DisplaySettings{Draw: &DrawDisplay{Who: DrawWhoOwners}}}, DrawWhoOwners},
		{"an unknown stored value is never more permissive than the default", &Map{Display: &DisplaySettings{Draw: &DrawDisplay{Who: "players"}}}, DrawWhoScribes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.m.DrawWho(); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
