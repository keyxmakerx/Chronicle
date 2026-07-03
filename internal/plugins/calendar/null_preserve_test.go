// null_preserve_test.go — pins the C-CAL-NULL-PRESERVE invariants on
// UpdateEvent, SetWeather, and UpdateCalendar.
//
// Three risk classes per the audit (reports/chronicle/2026-05-19-
// c-cal-null-preserve-audit.md):
//
//   1. UpdateEvent silently blanked 19 pointer-typed fields on a
//      partial save (title-only edit blanked description, color,
//      times, recurrence config, visibility rules). This file pins
//      the 18 fields the fix nil-guards.
//
//   2. UpdateEvent.EntityID is deliberately NOT in the fix (operator
//      direction 2026-05-19: pending C-ENTITY-LINK-DESIGN). The
//      "still clears on nil" regression test pins the deliberate
//      non-fix so a future contributor can't accidentally include
//      EntityID in a null-preserve sweep without revisiting the
//      design dispatch.
//
//   3. SetWeather did blind ON DUPLICATE KEY UPDATE — load-merge-write
//      is the fix. Preset-only edit must preserve wind/precip.
//
//   4. UpdateCalendar silently blanked Description + EpochName on a
//      date-only update.
//
// All four bugs match the class C-PERMISSIONS-INLINE-COMPONENT
// (chronicle#318) fixed for entities.IsPrivate via *bool.

package calendar

import (
	"context"
	"testing"
)

// strPtr / intPtr / floatPtr are pointer-builder helpers used by the
// table-driven Update/Set tests. Inline literals like `&"foo"` aren't
// valid Go; the helpers fold this nuisance.
func strPtr(s string) *string    { return &s }
func intPtr(i int) *int          { return &i }
func floatPtr(f float64) *float64 { return &f }

// seededEvent returns an Event with every nil-guarded field populated,
// so a test can call UpdateEvent with a sparse input and assert each
// field was preserved (not zeroed).
func seededEvent() *Event {
	return &Event{
		ID:                       "evt-1",
		CalendarID:               "cal-1",
		Name:                     "Original Name",
		Description:              strPtr("original description"),
		DescriptionHTML:          strPtr("<p>original</p>"),
		EntityID:                 strPtr("ent-1"),
		Year:                     1492,
		Month:                    7,
		Day:                      15,
		StartHour:                intPtr(14),
		StartMinute:              intPtr(30),
		EndYear:                  intPtr(1492),
		EndMonth:                 intPtr(7),
		EndDay:                   intPtr(18),
		EndHour:                  intPtr(22),
		EndMinute:                intPtr(0),
		IsRecurring:              true,
		RecurrenceType:           strPtr("yearly"),
		RecurrenceInterval:       intPtr(1),
		RecurrenceEndYear:        intPtr(1500),
		RecurrenceEndMonth:       intPtr(12),
		RecurrenceEndDay:         intPtr(31),
		RecurrenceMaxOccurrences: intPtr(10),
		Visibility:               "everyone",
		VisibilityRules:          strPtr(`{"allowed_users":["u1"]}`),
		Category:                 strPtr("festival"),
		Color:                    strPtr("#ff0000"),
		Icon:                     strPtr("star"),
		AllDay:                   true,
	}
}

// TestUpdateEvent_PartialUpdatePreservesUnsentFields pins the
// C-CAL-NULL-PRESERVE fix: a partial-save (here, a title-only edit)
// keeps every other in-scope pointer field at its seeded value rather
// than blanking it to nil. Without the fix, the operator's
// description, color, entity link, category, icon, times, recurrence
// config, and visibility rules would all clear.
//
// Mirrors entities/permissions_inline_component_test.go's
// TestUpdate_PreservesIsPrivateWhenInputNil pattern.
func TestUpdateEvent_PartialUpdatePreservesUnsentFields(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return seededEvent(), nil
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			written = evt
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Title-only partial input. All other pointer fields nil →
	// service must preserve the seeded values. Required value-typed
	// fields are sent (Year/Month/Day) because UpdateEvent's contract
	// treats those as always-present.
	input := UpdateEventInput{
		Name:       "Renamed Title",
		Year:       1492,
		Month:      7,
		Day:        15,
		Visibility: "everyone",
	}
	if err := svc.UpdateEvent(context.Background(), "evt-1", input); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written == nil {
		t.Fatal("repo.UpdateEvent not called")
	}

	// Name updated.
	if written.Name != "Renamed Title" {
		t.Errorf("Name = %q, want renamed", written.Name)
	}

	// Each pointer field must still be the seeded pointer's value.
	checks := []struct {
		name string
		got  *string
		want string
	}{
		{"Description", written.Description, "original description"},
		{"DescriptionHTML", written.DescriptionHTML, "<p>original</p>"},
		{"RecurrenceType", written.RecurrenceType, "yearly"},
		{"VisibilityRules", written.VisibilityRules, `{"allowed_users":["u1"]}`},
		{"Category", written.Category, "festival"},
		{"Color", written.Color, "#ff0000"},
		{"Icon", written.Icon, "star"},
	}
	for _, c := range checks {
		if c.got == nil {
			t.Errorf("%s was blanked to nil; C-CAL-NULL-PRESERVE guard regressed", c.name)
			continue
		}
		if *c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, *c.got, c.want)
		}
	}

	intChecks := []struct {
		name string
		got  *int
		want int
	}{
		{"StartHour", written.StartHour, 14},
		{"StartMinute", written.StartMinute, 30},
		{"EndYear", written.EndYear, 1492},
		{"EndMonth", written.EndMonth, 7},
		{"EndDay", written.EndDay, 18},
		{"EndHour", written.EndHour, 22},
		{"EndMinute", written.EndMinute, 0},
		{"RecurrenceInterval", written.RecurrenceInterval, 1},
		{"RecurrenceEndYear", written.RecurrenceEndYear, 1500},
		{"RecurrenceEndMonth", written.RecurrenceEndMonth, 12},
		{"RecurrenceEndDay", written.RecurrenceEndDay, 31},
		{"RecurrenceMaxOccurrences", written.RecurrenceMaxOccurrences, 10},
	}
	for _, c := range intChecks {
		if c.got == nil {
			t.Errorf("%s was blanked to nil; C-CAL-NULL-PRESERVE guard regressed", c.name)
			continue
		}
		if *c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, *c.got, c.want)
		}
	}
}

// TestUpdateEvent_EntityIDStillClearsOnNil pins the deliberate skip:
// EntityID is NOT in the null-preserve fix. Operator direction
// 2026-05-19 wants a holistic entity-link rework (C-ENTITY-LINK-DESIGN
// in BACKLOG) rather than a band-aid. Until that lands, nil EntityID
// continues to clear the link as it did pre-fix. A future contributor
// who sweeps "all pointer fields" without re-reading the design
// dispatch will break this test — that's the intent.
func TestUpdateEvent_EntityIDStillClearsOnNil(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return seededEvent(), nil // EntityID = "ent-1"
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			written = evt
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// EntityID intentionally nil on input. Today this clears.
	input := UpdateEventInput{
		Name:       "Renamed",
		Year:       1492,
		Month:      7,
		Day:        15,
		Visibility: "everyone",
		EntityID:   nil,
	}
	if err := svc.UpdateEvent(context.Background(), "evt-1", input); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written == nil {
		t.Fatal("repo.UpdateEvent not called")
	}
	if written.EntityID != nil {
		t.Errorf("EntityID = %q, want nil. EntityID was NOT supposed to be added to the C-CAL-NULL-PRESERVE sweep — pending C-ENTITY-LINK-DESIGN. See cordinator/plans/BACKLOG.md before changing this test.",
			*written.EntityID)
	}
}

// TestUpdateEvent_ExplicitFieldsStillWrite pins the positive side:
// when a pointer field IS provided on the input, the value is written.
// Without this, the guard would be too aggressive — every edit would
// be a no-op except for Name.
func TestUpdateEvent_ExplicitFieldsStillWrite(t *testing.T) {
	var written *Event
	repo := &mockCalendarRepo{
		getEventFn: func(_ context.Context, _ string) (*Event, error) {
			return seededEvent(), nil
		},
		updateEventFn: func(_ context.Context, evt *Event) error {
			written = evt
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	input := UpdateEventInput{
		Name:        "Renamed",
		Year:        1492,
		Month:       7,
		Day:         15,
		Visibility:  "dm_only",
		Description: strPtr("NEW description"),
		Color:       strPtr("#00ff00"),
		Category:    strPtr("birthday"),
	}
	if err := svc.UpdateEvent(context.Background(), "evt-1", input); err != nil {
		t.Fatalf("UpdateEvent: %v", err)
	}
	if written.Description == nil || *written.Description != "NEW description" {
		t.Errorf("Description not written through; got %v", written.Description)
	}
	if written.Color == nil || *written.Color != "#00ff00" {
		t.Errorf("Color not written through; got %v", written.Color)
	}
	if written.Category == nil || *written.Category != "birthday" {
		t.Errorf("Category not written through; got %v", written.Category)
	}
	// Negative: untouched pointer field (Icon) preserved from seed.
	if written.Icon == nil || *written.Icon != "star" {
		t.Errorf("Icon should be preserved when input.Icon=nil; got %v", written.Icon)
	}
}

// TestSetWeather_PartialUpdatePreservesUnsentFields pins the load-
// merge-write fix on SetWeather. Audit's concrete example: edit
// PresetID only → wind_speed_kph must stay at 15.5. Pre-fix, ON
// DUPLICATE KEY UPDATE wrote nil into wind_speed_kph, blanking it.
func TestSetWeather_PartialUpdatePreservesUnsentFields(t *testing.T) {
	existing := &Weather{
		ID:                 1,
		CalendarID:         "cal-1",
		PresetID:           strPtr("old-preset"),
		TemperatureCelsius: floatPtr(18.0),
		Wind: &Wind{
			SpeedKPH:         floatPtr(15.5),
			SpeedTier:        strPtr("moderate"),
			Direction:        strPtr("NW"),
			DirectionDegrees: intPtr(315),
		},
		Precipitation: &Precipitation{
			Type:      strPtr("rain"),
			Intensity: floatPtr(0.6),
		},
		ZoneID:      strPtr("temperate"),
		Description: strPtr("Steady rain"),
	}
	// Post-seam (cordinator#53) the merged state lands on the CURRENT
	// day's calendar_day_weather row via SetDayWeatherRich; the legacy
	// single-row write is no longer the persistence path. The merge
	// semantics under test are unchanged.
	var written WeatherInput
	var wroteType string
	var wroteY, wroteMo, wroteD int
	var mirrored *WeatherInput
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1", CurrentYear: 1492, CurrentMonth: 6, CurrentDay: 15}, nil
		},
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) {
			return existing, nil
		},
		setDayWeatherRichFn: func(_ context.Context, _ string, year, month, day int, weatherType string, in WeatherInput) error {
			written, wroteType = in, weatherType
			wroteY, wroteMo, wroteD = year, month, day
			return nil
		},
		setWeatherFn: func(_ context.Context, _ string, in WeatherInput) error {
			cp := in
			mirrored = &cp
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Preset-only partial input. The merge layer must overlay this
	// on top of `existing` and write the result.
	partial := WeatherInput{
		PresetID: strPtr("new-preset"),
	}
	if err := svc.SetWeather(context.Background(), "cal-1", partial); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}

	// New field landed — and weather_type carries the preset id.
	if written.PresetID == nil || *written.PresetID != "new-preset" {
		t.Errorf("PresetID not updated; got %v", written.PresetID)
	}
	if wroteType != "new-preset" {
		t.Errorf("weather_type = %q, want new-preset", wroteType)
	}
	// The write targets the calendar's current date.
	if wroteY != 1492 || wroteMo != 6 || wroteD != 15 {
		t.Errorf("day-weather write targeted %d-%d-%d; want current 1492-6-15", wroteY, wroteMo, wroteD)
	}
	// The merged state mirrors onto calendar_weather (the rolling fallback
	// snapshot), zone pointer included — preserved, not churned.
	if mirrored == nil {
		t.Fatal("merged state was not mirrored to calendar_weather")
	}
	if mirrored.PresetID == nil || *mirrored.PresetID != "new-preset" {
		t.Errorf("mirror PresetID = %v, want new-preset", mirrored.PresetID)
	}
	if mirrored.ZoneID == nil || *mirrored.ZoneID != "temperate" {
		t.Errorf("mirror ZoneID = %v, want preserved temperate", mirrored.ZoneID)
	}
	// Untouched fields preserved from existing.
	if written.TemperatureCelsius == nil || *written.TemperatureCelsius != 18.0 {
		t.Errorf("TemperatureCelsius was blanked; C-CAL-NULL-PRESERVE SetWeather merge regressed. Got %v", written.TemperatureCelsius)
	}
	if written.WindSpeedKPH == nil || *written.WindSpeedKPH != 15.5 {
		t.Errorf("WindSpeedKPH was blanked; SetWeather merge regressed. Got %v", written.WindSpeedKPH)
	}
	if written.WindDirection == nil || *written.WindDirection != "NW" {
		t.Errorf("WindDirection was blanked; got %v", written.WindDirection)
	}
	if written.PrecipitationType == nil || *written.PrecipitationType != "rain" {
		t.Errorf("PrecipitationType was blanked; got %v", written.PrecipitationType)
	}
	if written.PrecipitationIntensity == nil || *written.PrecipitationIntensity != 0.6 {
		t.Errorf("PrecipitationIntensity was blanked; got %v", written.PrecipitationIntensity)
	}
	if written.ZoneID == nil || *written.ZoneID != "temperate" {
		t.Errorf("ZoneID was blanked; got %v", written.ZoneID)
	}
}

// TestSetWeather_FirstWriteSeedsRow pins the no-existing-row path:
// when GetWeather returns nil (no row yet), SetWeather writes the
// input as-is — the merge is a no-op for the first write.
func TestSetWeather_FirstWriteSeedsRow(t *testing.T) {
	var written WeatherInput
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{ID: "cal-1", CampaignID: "camp-1", CurrentYear: 1492, CurrentMonth: 6, CurrentDay: 15}, nil
		},
		getWeatherFn: func(_ context.Context, _ string) (*Weather, error) {
			return nil, nil
		},
		setDayWeatherRichFn: func(_ context.Context, _ string, _, _, _ int, _ string, in WeatherInput) error {
			written = in
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	input := WeatherInput{
		PresetID:           strPtr("rain"),
		TemperatureCelsius: floatPtr(12),
	}
	if err := svc.SetWeather(context.Background(), "cal-1", input); err != nil {
		t.Fatalf("SetWeather: %v", err)
	}
	if written.PresetID == nil || *written.PresetID != "rain" {
		t.Errorf("PresetID = %v, want rain", written.PresetID)
	}
	if written.WindSpeedKPH != nil {
		t.Errorf("WindSpeedKPH = %v, want nil (no existing row to merge from)", written.WindSpeedKPH)
	}
}

// TestUpdateCalendar_PartialUpdatePreservesDescription pins the
// UpdateCalendar nil-preserve fix. Operator-flavoured scenario: an
// advance-date UI handler that only knows about the date fields
// submits a partial UpdateCalendarInput. Pre-fix, the calendar's
// Description and EpochName silently cleared on every such save.
func TestUpdateCalendar_PartialUpdatePreservesDescription(t *testing.T) {
	var written *Calendar
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) {
			return &Calendar{
				ID:               "cal-1",
				CampaignID:       "camp-1",
				Mode:             ModeFantasy,
				Name:             "Calendar of Therin",
				Description:      strPtr("Operator-authored setup description"),
				EpochName:        strPtr("DR"),
				CurrentYear:      100,
				CurrentMonth:     1,
				CurrentDay:       1,
				CurrentHour:      0,
				CurrentMinute:    0,
				HoursPerDay:      24,
				MinutesPerHour:   60,
				SecondsPerMinute: 60,
			}, nil
		},
		updateFn: func(_ context.Context, cal *Calendar) error {
			written = cal
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Partial input — only the date moved. Description + EpochName
	// nil on input must NOT blank the stored values.
	input := UpdateCalendarInput{
		Name:             "Calendar of Therin",
		CurrentYear:      100,
		CurrentMonth:     2,
		CurrentDay:       15,
		CurrentHour:      0,
		CurrentMinute:    0,
		HoursPerDay:      24,
		MinutesPerHour:   60,
		SecondsPerMinute: 60,
	}
	if err := svc.UpdateCalendar(context.Background(), "cal-1", input); err != nil {
		t.Fatalf("UpdateCalendar: %v", err)
	}
	if written == nil {
		t.Fatal("repo.Update not called")
	}
	if written.Description == nil || *written.Description != "Operator-authored setup description" {
		t.Errorf("Description was blanked; UpdateCalendar nil-preserve guard regressed. Got %v", written.Description)
	}
	if written.EpochName == nil || *written.EpochName != "DR" {
		t.Errorf("EpochName was blanked; UpdateCalendar nil-preserve guard regressed. Got %v", written.EpochName)
	}
	// Positive: date fields did write through.
	if written.CurrentMonth != 2 || written.CurrentDay != 15 {
		t.Errorf("date fields not written through; got %d/%d, want 2/15", written.CurrentMonth, written.CurrentDay)
	}
}
