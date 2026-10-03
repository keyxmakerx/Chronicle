package maps

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
)

// storedMapWithDisplay is storedMapRow plus display settings, the row every
// display_settings case starts from.
func storedMapWithDisplay() *Map {
	m := storedMapRow()
	m.Display = &DisplaySettings{
		Pins: &PinDisplay{Style: PinStyleFlag},
		Draw: &DrawDisplay{Who: DrawWhoOwners},
	}
	return m
}

func runDisplayUpdate(t *testing.T, input UpdateMapInput) (*Map, error) {
	t.Helper()
	var written *Map
	repo := &mockMapRepo{
		getMapFn:    func(context.Context, string) (*Map, error) { return storedMapWithDisplay(), nil },
		updateMapFn: func(_ context.Context, m *Map) error { written = m; return nil },
	}
	err := newTestMapService(repo).UpdateMap(context.Background(), "map-1", input)
	return written, err
}

// The partial-update contract on display_settings: absent preserves, null
// clears, an object replaces the groups it names. A rename-only PUT is the
// push that must never touch the draw gate.
func TestUpdateMap_DisplaySettings_ThreeDirections(t *testing.T) {
	cases := []struct {
		name      string
		input     UpdateMapInput
		wantPins  *PinDisplay
		wantDraw  *DrawDisplay
		wantWrite bool
	}{
		{
			name:     "absent preserves the whole document (a rename-only PUT)",
			input:    UpdateMapInput{Name: "Renamed"},
			wantPins: &PinDisplay{Style: PinStyleFlag}, wantDraw: &DrawDisplay{Who: DrawWhoOwners},
			wantWrite: true,
		},
		{
			name:     "a present object replaces only the groups it names",
			input:    UpdateMapInput{Name: "Old Map Name", DisplaySettings: patch.Of(json.RawMessage(`{"pins":{"style":"seal"}}`))},
			wantPins: &PinDisplay{Style: PinStyleSeal}, wantDraw: &DrawDisplay{Who: DrawWhoOwners},
			wantWrite: true,
		},
		{
			name:     "a group set to null clears just that group",
			input:    UpdateMapInput{Name: "Old Map Name", DisplaySettings: patch.Of(json.RawMessage(`{"draw":null}`))},
			wantPins: &PinDisplay{Style: PinStyleFlag}, wantDraw: nil,
			wantWrite: true,
		},
		{
			name:     "explicit null clears every group",
			input:    UpdateMapInput{Name: "Old Map Name", DisplaySettings: patch.Null[json.RawMessage]()},
			wantPins: nil, wantDraw: nil,
			wantWrite: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runDisplayUpdate(t, tc.input)
			if err != nil {
				t.Fatalf("UpdateMap: %v", err)
			}
			if got == nil {
				t.Fatal("nothing written")
			}
			var gotPins *PinDisplay
			var gotDraw *DrawDisplay
			if got.Display != nil {
				gotPins, gotDraw = got.Display.Pins, got.Display.Draw
			}
			if (gotPins == nil) != (tc.wantPins == nil) || (gotPins != nil && *gotPins != *tc.wantPins) {
				t.Errorf("pins = %+v, want %+v", gotPins, tc.wantPins)
			}
			if (gotDraw == nil) != (tc.wantDraw == nil) || (gotDraw != nil && *gotDraw != *tc.wantDraw) {
				t.Errorf("draw = %+v, want %+v", gotDraw, tc.wantDraw)
			}
		})
	}
}

// A bad display_settings document is refused before anything is written.
func TestUpdateMap_DisplaySettings_InvalidWritesNothing(t *testing.T) {
	got, err := runDisplayUpdate(t, UpdateMapInput{
		Name:            "Old Map Name",
		DisplaySettings: patch.Of(json.RawMessage(`{"draw":{"who":"everyone"}}`)),
	})
	var ae *apperror.AppError
	if !errors.As(err, &ae) || ae.Type != "validation_error" {
		t.Fatalf("want a validation error, got %v", err)
	}
	if got != nil {
		t.Errorf("a refused update still wrote: %+v", got)
	}
}

// Background colour is rendered into an inline style, so a non-hex value is
// refused; an empty value still clears the override.
func TestUpdateMap_BackgroundColor(t *testing.T) {
	str := func(s string) *string { return &s }
	cases := []struct {
		name    string
		bg      *string
		wantErr bool
		wantBg  *string
	}{
		{"hex is accepted", str("#abc123"), false, str("#abc123")},
		{"empty clears", str(""), false, nil},
		{"absent preserves", nil, false, str("#101010")},
		{"css injection is refused", str("red;position:fixed"), true, nil},
		{"a url is refused", str("url(javascript:1)"), true, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var written *Map
			repo := &mockMapRepo{
				getMapFn:    func(context.Context, string) (*Map, error) { return storedMapRow(), nil },
				updateMapFn: func(_ context.Context, m *Map) error { written = m; return nil },
			}
			err := newTestMapService(repo).UpdateMap(context.Background(), "map-1", UpdateMapInput{Name: "N", BackgroundColor: tc.bg})
			if tc.wantErr {
				if err == nil || written != nil {
					t.Fatalf("want a refusal with no write, got err=%v written=%v", err, written)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertMarkerPtr(t, "BackgroundColor", written.BackgroundColor, tc.wantBg)
		})
	}
}

func TestCampaignFrame(t *testing.T) {
	t.Run("a campaign that never chose gets atlas", func(t *testing.T) {
		got, err := newTestMapService(&mockMapRepo{}).GetCampaignFrame(context.Background(), "c1")
		if err != nil || got != "atlas" {
			t.Errorf("got %q, %v", got, err)
		}
	})
	t.Run("a stored frame is returned", func(t *testing.T) {
		got, _ := newTestMapService(&mockMapRepo{campaignFrame: "gilded"}).GetCampaignFrame(context.Background(), "c1")
		if got != "gilded" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("a stored value that is no longer a frame reads as atlas", func(t *testing.T) {
		got, _ := newTestMapService(&mockMapRepo{campaignFrame: "retired"}).GetCampaignFrame(context.Background(), "c1")
		if got != "atlas" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("a repository error is surfaced", func(t *testing.T) {
		if _, err := newTestMapService(&mockMapRepo{getFrameErr: errors.New("db down")}).GetCampaignFrame(context.Background(), "c1"); err == nil {
			t.Error("want an error")
		}
	})
	setCases := []struct {
		name, campaign, frame string
		wantErr               bool
	}{
		{"valid", "c1", "old", false},
		{"unknown frame", "c1", "steampunk", true},
		{"empty frame", "c1", "", true},
		{"no campaign", "", "old", true},
	}
	for _, tc := range setCases {
		t.Run("set "+tc.name, func(t *testing.T) {
			called := false
			repo := &mockMapRepo{setCampaignFrame: func(context.Context, string, string) error { called = true; return nil }}
			err := newTestMapService(repo).SetCampaignFrame(context.Background(), tc.campaign, tc.frame)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if called == tc.wantErr {
				t.Errorf("repo called = %v, but wantErr = %v", called, tc.wantErr)
			}
		})
	}
}

// ResolveDisplay degrades to the default frame, with the error, when the
// campaign lookup fails, so the page still renders.
func TestResolveDisplay_LookupFailureStillResolves(t *testing.T) {
	r, err := newTestMapService(&mockMapRepo{getFrameErr: errors.New("db down")}).ResolveDisplay(context.Background(), &Map{CampaignID: "c1"})
	if err == nil {
		t.Error("want the error surfaced")
	}
	if r.Frame != "atlas" || len(r.Kinds) != 5 {
		t.Errorf("want usable defaults, got %+v", r)
	}
}
