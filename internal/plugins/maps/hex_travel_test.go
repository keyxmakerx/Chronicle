package maps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestUpdateLayer_Travel(t *testing.T) {
	tests := []struct {
		name     string
		actor    HexActor
		in       UpdateHexLayerInput
		start    *HexLayer
		wantErr  bool
		wantHex  int
		wantDay  int
		wantSets int
	}{
		{"owner sets miles per hex only", actorOwner, UpdateHexLayerInput{MilesPerHex: patch.Of(10)},
			&HexLayer{MapID: "map-1", MilesPerHex: 6, MilesPerDay: 24}, false, 10, 24, 1},
		{"DM grant sets miles per day only", actorDM, UpdateHexLayerInput{MilesPerDay: patch.Of(30)},
			&HexLayer{MapID: "map-1", MilesPerHex: 6, MilesPerDay: 24}, false, 6, 30, 1},
		{"both at once is one write", actorOwner, UpdateHexLayerInput{MilesPerHex: patch.Of(3), MilesPerDay: patch.Of(18)},
			nil, false, 3, 18, 1},
		{"lowest and highest allowed", actorOwner, UpdateHexLayerInput{MilesPerHex: patch.Of(1), MilesPerDay: patch.Of(1000)},
			nil, false, 1, 1000, 1},
		{"null restores the default", actorOwner, UpdateHexLayerInput{MilesPerHex: patch.Null[int]()},
			&HexLayer{MapID: "map-1", MilesPerHex: 12, MilesPerDay: 40}, false, 6, 40, 1},
		{"zero is refused", actorOwner, UpdateHexLayerInput{MilesPerHex: patch.Of(0)}, nil, true, 0, 0, 0},
		{"negative is refused", actorOwner, UpdateHexLayerInput{MilesPerDay: patch.Of(-5)}, nil, true, 0, 0, 0},
		{"over the ceiling is refused", actorOwner, UpdateHexLayerInput{MilesPerDay: patch.Of(1001)}, nil, true, 0, 0, 0},
		{"a bad figure refuses the whole body", actorOwner, UpdateHexLayerInput{FogEnabled: patch.Of(true), MilesPerHex: patch.Of(0)},
			nil, true, 0, 0, 0},
		{"a scribe is refused", actorScribe, UpdateHexLayerInput{MilesPerHex: patch.Of(10)}, nil, true, 0, 0, 0},
		{"a player is refused", actorPlayer, UpdateHexLayerInput{MilesPerDay: patch.Of(10)}, nil, true, 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, events, _ := fogFixture(PartyWhoScribes)
			repo.layer = tc.start
			res, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", tc.actor, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want an error")
				}
				if repo.travelSets != 0 || len(events.calls) != 0 || (tc.start == nil && repo.layer != nil) {
					t.Errorf("a refused change must not write or publish: sets=%d events=%d layer=%+v", repo.travelSets, len(events.calls), repo.layer)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.MilesPerHex != tc.wantHex || res.MilesPerDay != tc.wantDay {
				t.Errorf("result = %d / %d, want %d / %d", res.MilesPerHex, res.MilesPerDay, tc.wantHex, tc.wantDay)
			}
			if repo.layer.MilesPerHex != tc.wantHex || repo.layer.MilesPerDay != tc.wantDay || repo.travelSets != tc.wantSets {
				t.Errorf("stored = %+v sets = %d", repo.layer, repo.travelSets)
			}
			if len(events.calls) != 1 {
				t.Errorf("events = %d, want 1", len(events.calls))
			}
		})
	}
}

// A body that names only fog must leave the travel figures alone, and one that
// names only a travel figure must leave fog and the anchor alone.
func TestUpdateLayer_TravelIsIndependent(t *testing.T) {
	svc, repo, _, _ := fogFixture(PartyWhoScribes)
	repo.layer = &HexLayer{MapID: "map-1", FogEnabled: true, MilesPerHex: 9, MilesPerDay: 33}
	if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{FogEnabled: patch.Of(false)}); err != nil {
		t.Fatal(err)
	}
	if repo.travelSets != 0 || repo.layer.MilesPerHex != 9 || repo.layer.MilesPerDay != 33 {
		t.Errorf("a fog-only body touched travel: %+v sets=%d", repo.layer, repo.travelSets)
	}
	if _, err := svc.UpdateLayer(context.Background(), "camp-1", "map-1", actorOwner, UpdateHexLayerInput{MilesPerDay: patch.Of(20)}); err != nil {
		t.Fatal(err)
	}
	if repo.layer.FogEnabled || repo.layer.MilesPerHex != 9 || repo.anchorSets != 0 {
		t.Errorf("a travel-only body touched something else: %+v anchorSets=%d", repo.layer, repo.anchorSets)
	}
}

// The wire body must tell absent, null and a number apart, and refuse a figure
// that is not a whole number.
func TestHexLayerBody_TravelFields(t *testing.T) {
	type body struct {
		MilesPerHex patch.Field[int] `json:"miles_per_hex"`
		MilesPerDay patch.Field[int] `json:"miles_per_day"`
	}
	tests := []struct {
		name            string
		raw             string
		hexSet, hexNull bool
		daySet          bool
		wantErr         bool
	}{
		{"absent", `{}`, false, false, false, false},
		{"number", `{"miles_per_hex": 8}`, true, false, false, false},
		{"null", `{"miles_per_hex": null, "miles_per_day": 30}`, true, true, true, false},
		{"fraction", `{"miles_per_hex": 6.5}`, false, false, false, true},
		{"string", `{"miles_per_day": "far"}`, false, false, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var b body
			err := json.Unmarshal([]byte(tc.raw), &b)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v", err)
			}
			if tc.wantErr {
				return
			}
			if b.MilesPerHex.Present() != tc.hexSet || b.MilesPerHex.IsNull() != tc.hexNull || b.MilesPerDay.Present() != tc.daySet {
				t.Errorf("hex present=%v null=%v day present=%v", b.MilesPerHex.Present(), b.MilesPerHex.IsNull(), b.MilesPerDay.Present())
			}
		})
	}
}
