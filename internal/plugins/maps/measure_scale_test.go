package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// The scale is stored exactly as drawn or not at all: an out-of-range point or
// length is refused rather than clamped, because a clamped value would be a
// different scale from the one the person measured.
func TestNormalizeMeasure(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
		want    *MeasureDisplay
	}{
		{
			name: "a line across the map in miles",
			in:   `{"a":{"x":10,"y":20},"b":{"x":60,"y":20},"length":12.5,"unit":"miles"}`,
			want: &MeasureDisplay{A: MeasurePoint{10, 20}, B: MeasurePoint{60, 20}, Length: 12.5, Unit: "miles"},
		},
		{
			name: "corners of the map are on the map",
			in:   `{"a":{"x":0,"y":0},"b":{"x":100,"y":100},"length":100000,"unit":"km"}`,
			want: &MeasureDisplay{A: MeasurePoint{0, 0}, B: MeasurePoint{100, 100}, Length: 100000, Unit: "km"},
		},
		{name: "every unit on the list", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":5,"unit":"leagues"}`,
			want: &MeasureDisplay{A: MeasurePoint{1, 1}, B: MeasurePoint{2, 2}, Length: 5, Unit: "leagues"}},
		{name: "feet", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":30,"unit":"feet"}`,
			want: &MeasureDisplay{A: MeasurePoint{1, 1}, B: MeasurePoint{2, 2}, Length: 30, Unit: "feet"}},
		{name: "unknown unit", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":5,"unit":"parsecs"}`, wantErr: true},
		{name: "missing unit", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":5}`, wantErr: true},
		{name: "zero length", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":0,"unit":"km"}`, wantErr: true},
		{name: "negative length", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":-3,"unit":"km"}`, wantErr: true},
		{name: "length past the cap", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":100000.5,"unit":"km"}`, wantErr: true},
		{name: "missing length", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"unit":"km"}`, wantErr: true},
		{name: "length as text", in: `{"a":{"x":1,"y":1},"b":{"x":2,"y":2},"length":"10","unit":"km"}`, wantErr: true},
		{name: "a point off the right edge", in: `{"a":{"x":1,"y":1},"b":{"x":100.1,"y":2},"length":5,"unit":"km"}`, wantErr: true},
		{name: "a point above the top", in: `{"a":{"x":1,"y":-0.5},"b":{"x":2,"y":2},"length":5,"unit":"km"}`, wantErr: true},
		{name: "missing end", in: `{"a":{"x":1,"y":1},"length":5,"unit":"km"}`, wantErr: true},
		{name: "both ends on one spot", in: `{"a":{"x":40,"y":40},"b":{"x":40,"y":40},"length":5,"unit":"km"}`, wantErr: true},
		{name: "ends too close together", in: `{"a":{"x":40,"y":40},"b":{"x":40.05,"y":40},"length":5,"unit":"km"}`, wantErr: true},
		{name: "not an object", in: `"ten miles"`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeMeasure(json.RawMessage(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected a validation error, got %+v", got)
				}
				assertAppError(t, err, http.StatusUnprocessableEntity)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if *got != *tc.want {
				t.Fatalf("got %+v, want %+v", *got, *tc.want)
			}
		})
	}
}

// Who may write the scale, and that a write touches only the measure group:
// the owner or a member with DM access may set or remove it; everyone else is
// refused before the map is even read, and a map from another campaign reads
// as missing.
func TestSetMeasureScale_GateAndMerge(t *testing.T) {
	const good = `{"a":{"x":10,"y":10},"b":{"x":30,"y":10},"length":4,"unit":"km"}`
	stored := func() *Map {
		m := storedMapWithDisplay()
		m.Display.Measure = &MeasureDisplay{A: MeasurePoint{1, 1}, B: MeasurePoint{5, 1}, Length: 2, Unit: "miles"}
		return m
	}
	cases := []struct {
		name        string
		canSet      bool
		campaignID  string
		scale       string // "" sends nothing (remove)
		wantCode    int
		wantWritten bool
		wantMeasure *MeasureDisplay
	}{
		{name: "owner or DM sets a scale", canSet: true, campaignID: "camp-1", scale: good, wantWritten: true,
			wantMeasure: &MeasureDisplay{A: MeasurePoint{10, 10}, B: MeasurePoint{30, 10}, Length: 4, Unit: "km"}},
		{name: "owner or DM removes the scale", canSet: true, campaignID: "camp-1", scale: "", wantWritten: true},
		{name: "explicit null removes the scale", canSet: true, campaignID: "camp-1", scale: "null", wantWritten: true},
		{name: "a player or scribe is refused", canSet: false, campaignID: "camp-1", scale: good, wantCode: http.StatusForbidden},
		{name: "a player cannot remove it either", canSet: false, campaignID: "camp-1", scale: "", wantCode: http.StatusForbidden},
		{name: "a map in another campaign is not found", canSet: true, campaignID: "camp-2", scale: good, wantCode: http.StatusNotFound},
		{name: "a bad scale is refused and nothing is written", canSet: true, campaignID: "camp-1",
			scale: `{"a":{"x":10,"y":10},"b":{"x":30,"y":10},"length":4,"unit":"cubits"}`, wantCode: http.StatusUnprocessableEntity},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var written *Map
			reads := 0
			repo := &mockMapRepo{
				getMapFn: func(context.Context, string) (*Map, error) {
					reads++
					m := stored()
					m.CampaignID = "camp-1"
					return m, nil
				},
				updateMapFn: func(_ context.Context, m *Map) error { written = m; return nil },
			}
			var raw json.RawMessage
			if tc.scale != "" {
				raw = json.RawMessage(tc.scale)
			}
			got, err := newTestMapService(repo).SetMeasureScale(context.Background(), tc.campaignID, "map-1", tc.canSet, raw)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
				if written != nil {
					t.Fatal("a refused request must not write")
				}
				if tc.wantCode == http.StatusForbidden && reads != 0 {
					t.Fatal("the gate must refuse before reading the map")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.wantWritten || written == nil {
				t.Fatal("expected a write")
			}
			// Every other group survives the scale write.
			if written.Display == nil || written.Display.Pins == nil || written.Display.Pins.Style != PinStyleFlag ||
				written.Display.Draw == nil || written.Display.Draw.Who != DrawWhoOwners {
				t.Fatalf("other groups were touched: %+v", written.Display)
			}
			if tc.wantMeasure == nil {
				if written.Display.Measure != nil || got != nil {
					t.Fatalf("scale should be gone, stored %+v returned %+v", written.Display.Measure, got)
				}
				return
			}
			if written.Display.Measure == nil || *written.Display.Measure != *tc.wantMeasure || got == nil || *got != *tc.wantMeasure {
				t.Fatalf("stored %+v returned %+v, want %+v", written.Display.Measure, got, tc.wantMeasure)
			}
		})
	}
}

// A settings save from the sheet never names "measure", so it must keep the
// scale a DM set; the resolved display carries it to every viewer.
func TestMeasureScale_SurvivesOtherSavesAndResolves(t *testing.T) {
	cur := &DisplaySettings{Measure: &MeasureDisplay{A: MeasurePoint{1, 1}, B: MeasurePoint{9, 1}, Length: 3, Unit: "leagues"}}
	next, err := MergeDisplaySettings(cur, json.RawMessage(`{"grid":{"type":"square"},"pins":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if next == nil || next.Measure == nil || next.Measure.Unit != "leagues" {
		t.Fatalf("scale lost on an unrelated save: %+v", next)
	}
	r := ResolveDisplay(&Map{Display: next}, "")
	if r.Measure == nil || r.Measure.Length != 3 {
		t.Fatalf("resolved display lost the scale: %+v", r.Measure)
	}
	if ResolveDisplay(&Map{}, "").Measure != nil {
		t.Fatal("a map with no scale must resolve to null")
	}
}
