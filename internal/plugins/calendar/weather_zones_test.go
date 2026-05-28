// weather_zones_test.go covers V2 Wave 0 PR 3 (C-CAL-WEATHER-ZONES)
// service-layer behavior: zone-id slug validation, payload shape
// validation, active-zone catalog cross-check, REPLACE semantics, and
// the WS event dispatch pair (catalog edit vs active-zone change).

package calendar

import (
	"context"
	"testing"
)

// hasEvent returns true if the publisher saw an event of the given
// type for the given calendar resourceID.
func (m *mockWSPublisher) hasEvent(eventType, calendarID string) bool {
	for _, e := range m.publishedEvents {
		if e.eventType == eventType && e.resourceID == calendarID {
			return true
		}
	}
	return false
}

// TestSetWeatherZones_HappyPath_ReplaceCatalog — valid catalog +
// active_zone pointing at one of the zones: repo.ApplyWeatherZones is
// called with the supplied set, SetActiveWeatherZone is called with the
// matching name, and both calendar.weather.zones.changed and
// calendar.weather.changed are published.
func TestSetWeatherZones_HappyPath_ReplaceCatalog(t *testing.T) {
	var captured []WeatherZone
	var activeID, activeName string
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
		applyWeatherZonesFn: func(ctx context.Context, calendarID string, zones []WeatherZone) error {
			captured = zones
			return nil
		},
		setActiveWeatherZoneFn: func(ctx context.Context, calendarID, zoneID, zoneName string) error {
			activeID, activeName = zoneID, zoneName
			return nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		ActiveZone: "temperate",
		Zones: []WeatherZone{
			{ZoneID: "temperate", Name: "Temperate", Payload: map[string]any{}},
			{ZoneID: "arctic", Name: "Arctic", Payload: map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("SetWeatherZones: %v", err)
	}
	if len(captured) != 2 {
		t.Fatalf("expected 2 zones written; got %d", len(captured))
	}
	if captured[0].CalendarID != "cal-1" {
		t.Errorf("calendar_id not stamped onto zones; got %q", captured[0].CalendarID)
	}
	if activeID != "temperate" || activeName != "Temperate" {
		t.Errorf("SetActiveWeatherZone got (%q, %q); want (temperate, Temperate)", activeID, activeName)
	}
	if !bus.hasEvent("calendar.weather.zones.changed", "cal-1") {
		t.Error("expected calendar.weather.zones.changed event")
	}
	if !bus.hasEvent("calendar.weather.changed", "cal-1") {
		t.Error("expected calendar.weather.changed event (active zone changed)")
	}
}

// TestSetWeatherZones_CatalogOnly_NoActiveEvent — empty ActiveZone
// means catalog-only edit; only zones.changed is emitted, NOT
// weather.changed, and SetActiveWeatherZone is not called.
func TestSetWeatherZones_CatalogOnly_NoActiveEvent(t *testing.T) {
	activeCalled := false
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
		setActiveWeatherZoneFn: func(ctx context.Context, calendarID, zoneID, zoneName string) error {
			activeCalled = true
			return nil
		},
	}
	svc, bus := newSvcWithBus(repo)

	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}},
	})
	if err != nil {
		t.Fatalf("SetWeatherZones: %v", err)
	}
	if activeCalled {
		t.Error("SetActiveWeatherZone should not be called when ActiveZone is empty")
	}
	if !bus.hasEvent("calendar.weather.zones.changed", "cal-1") {
		t.Error("expected calendar.weather.zones.changed event")
	}
	if bus.hasEvent("calendar.weather.changed", "cal-1") {
		t.Error("did not expect calendar.weather.changed (catalog-only edit)")
	}
}

// TestSetWeatherZones_ActiveZoneNotInCatalog_ValidationError — pointing
// active_zone at a zone not in the supplied list rejects.
func TestSetWeatherZones_ActiveZoneNotInCatalog_ValidationError(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		ActiveZone: "tropical",
		Zones:      []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}},
	})
	if err == nil {
		t.Fatal("expected validation error for active_zone not in zones")
	}
}

// TestSetWeatherZones_InvalidZoneID_ValidationError — uppercase /
// special characters in zone_id reject.
func TestSetWeatherZones_InvalidZoneID_ValidationError(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "Bad Zone!", Name: "Bad"}},
	})
	if err == nil {
		t.Fatal("expected validation error for malformed zone_id")
	}
}

// TestSetWeatherZones_DuplicateZoneID_ValidationError — duplicate
// zone_id within a single catalog rejects.
func TestSetWeatherZones_DuplicateZoneID_ValidationError(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{
			{ZoneID: "temperate", Name: "A"},
			{ZoneID: "temperate", Name: "B"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for duplicate zone_id")
	}
}

// TestSetWeatherZones_EmptyName_ValidationError — empty zone name
// rejects (mirrors other settings PUTs).
func TestSetWeatherZones_EmptyName_ValidationError(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "temperate", Name: ""}},
	})
	if err == nil {
		t.Fatal("expected validation error for empty zone name")
	}
}

// TestSetWeatherZones_PayloadPresetsMissingLabel — presets entry
// missing label rejects.
func TestSetWeatherZones_PayloadPresetsMissingLabel(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "temperate", Name: "Temperate", Payload: map[string]any{
			"presets": []any{
				map[string]any{"temperature": 20.0}, // no label
			},
		}}},
	})
	if err == nil {
		t.Fatal("expected validation error for preset missing label")
	}
}

// TestSetWeatherZones_PayloadPresetsValid — presets with label +
// numeric temperature pass validation.
func TestSetWeatherZones_PayloadPresetsValid(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
	}
	svc, _ := newSvcWithBus(repo)
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "temperate", Name: "Temperate", Payload: map[string]any{
			"presets": []any{
				map[string]any{"label": "Clear", "temperature": 22.0},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("expected valid preset payload to pass; got %v", err)
	}
}

// TestSetWeatherZones_PayloadSeasonOverridesNotObject — season_overrides
// must be an object/map.
func TestSetWeatherZones_PayloadSeasonOverridesNotObject(t *testing.T) {
	svc, _ := newSvcWithBus(&mockCalendarRepo{})
	err := svc.SetWeatherZones(context.Background(), "cal-1", WeatherZonesState{
		Zones: []WeatherZone{{ZoneID: "temperate", Name: "Temperate", Payload: map[string]any{
			"season_overrides": []any{"not", "an", "object"},
		}}},
	})
	if err == nil {
		t.Fatal("expected validation error for season_overrides as array")
	}
}

// TestGetWeatherZones_BundlesActiveZone — repo returns 2 zones + a
// calendar_weather row with zone_id pointer; service merges the two
// into a WeatherZonesState response.
func TestGetWeatherZones_BundlesActiveZone(t *testing.T) {
	activeZoneID := "arctic"
	repo := &mockCalendarRepo{
		getWeatherZonesFn: func(ctx context.Context, calendarID string) ([]WeatherZone, error) {
			return []WeatherZone{
				{ZoneID: "temperate", Name: "Temperate"},
				{ZoneID: "arctic", Name: "Arctic"},
			}, nil
		},
		getWeatherFn: func(ctx context.Context, calendarID string) (*Weather, error) {
			return &Weather{ZoneID: &activeZoneID}, nil
		},
	}
	svc, _ := newSvcWithBus(repo)
	state, err := svc.GetWeatherZones(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("GetWeatherZones: %v", err)
	}
	if state.ActiveZone != "arctic" {
		t.Errorf("ActiveZone = %q; want arctic", state.ActiveZone)
	}
	if len(state.Zones) != 2 {
		t.Errorf("expected 2 zones; got %d", len(state.Zones))
	}
}

// TestGetWeatherZones_NoActiveZone — no calendar_weather row → empty
// ActiveZone string, zones still returned.
func TestGetWeatherZones_NoActiveZone(t *testing.T) {
	repo := &mockCalendarRepo{
		getWeatherZonesFn: func(ctx context.Context, calendarID string) ([]WeatherZone, error) {
			return []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}}, nil
		},
		// getWeatherFn returns nil, nil (no weather row yet).
	}
	svc, _ := newSvcWithBus(repo)
	state, err := svc.GetWeatherZones(context.Background(), "cal-1")
	if err != nil {
		t.Fatalf("GetWeatherZones: %v", err)
	}
	if state.ActiveZone != "" {
		t.Errorf("ActiveZone = %q; want empty string", state.ActiveZone)
	}
	if len(state.Zones) != 1 {
		t.Errorf("expected 1 zone; got %d", len(state.Zones))
	}
}

// TestSetActiveWeatherZone_ZoneInCatalog — happy path: zone exists,
// active reference is updated, calendar.weather.changed published.
func TestSetActiveWeatherZone_ZoneInCatalog(t *testing.T) {
	var activeID, activeName string
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
		getWeatherZonesFn: func(ctx context.Context, calendarID string) ([]WeatherZone, error) {
			return []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}}, nil
		},
		setActiveWeatherZoneFn: func(ctx context.Context, calendarID, zoneID, zoneName string) error {
			activeID, activeName = zoneID, zoneName
			return nil
		},
	}
	svc, bus := newSvcWithBus(repo)
	if err := svc.SetActiveWeatherZone(context.Background(), "cal-1", "temperate"); err != nil {
		t.Fatalf("SetActiveWeatherZone: %v", err)
	}
	if activeID != "temperate" || activeName != "Temperate" {
		t.Errorf("repo got (%q, %q); want (temperate, Temperate)", activeID, activeName)
	}
	if !bus.hasEvent("calendar.weather.changed", "cal-1") {
		t.Error("expected calendar.weather.changed event")
	}
}

// TestSetActiveWeatherZone_ZoneNotInCatalog_ValidationError — supplying
// a zoneID with no matching catalog entry rejects.
func TestSetActiveWeatherZone_ZoneNotInCatalog_ValidationError(t *testing.T) {
	repo := &mockCalendarRepo{
		getWeatherZonesFn: func(ctx context.Context, calendarID string) ([]WeatherZone, error) {
			return []WeatherZone{{ZoneID: "temperate", Name: "Temperate"}}, nil
		},
	}
	svc, _ := newSvcWithBus(repo)
	if err := svc.SetActiveWeatherZone(context.Background(), "cal-1", "nonexistent"); err == nil {
		t.Fatal("expected validation error for nonexistent zone")
	}
}

// TestSetActiveWeatherZone_EmptyClearsReference — empty zoneID clears
// the reference without a catalog lookup.
func TestSetActiveWeatherZone_EmptyClearsReference(t *testing.T) {
	called := false
	var gotID, gotName string
	repo := &mockCalendarRepo{
		getByIDFn: stubGetByIDForCalendar("cal-1", "camp-1"),
		setActiveWeatherZoneFn: func(ctx context.Context, calendarID, zoneID, zoneName string) error {
			called = true
			gotID, gotName = zoneID, zoneName
			return nil
		},
	}
	svc, _ := newSvcWithBus(repo)
	if err := svc.SetActiveWeatherZone(context.Background(), "cal-1", ""); err != nil {
		t.Fatalf("SetActiveWeatherZone: %v", err)
	}
	if !called {
		t.Fatal("expected SetActiveWeatherZone repo call")
	}
	if gotID != "" || gotName != "" {
		t.Errorf("expected ('', ''); got (%q, %q)", gotID, gotName)
	}
}
