// import_bounds_test.go covers the upper bounds clampCalendarStructure holds
// every imported calendar to. Each test isolates exactly one bound — a
// count that must be refused, or a per-item value that must be
// truncated/clamped with a warning — and the format-path test proves the
// same bound applies no matter which of the four importers produced the
// *ImportResult, including Chronicle's own.
package calendar

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestClampCalendarStructure_RefusesOversizedCounts is table-driven over
// every count clampCalendarStructure refuses rather than truncates (see its
// own doc comment on why refusing beats silently dropping rows). Zero-valued
// slice elements are enough: checkCalendarImportLimits only ever looks at
// length, and it runs before anything touches a single element's fields.
func TestClampCalendarStructure_RefusesOversizedCounts(t *testing.T) {
	tests := []struct {
		name    string
		build   func() *ImportResult
		wantHas string
	}{
		{"too many months", func() *ImportResult {
			return &ImportResult{Months: make([]MonthInput, maxCalendarMonths+1)}
		}, "months"},
		{"too many weekdays", func() *ImportResult {
			return &ImportResult{Weekdays: make([]WeekdayInput, maxCalendarWeekdays+1)}
		}, "weekdays"},
		{"too many eras", func() *ImportResult {
			return &ImportResult{Eras: make([]EraInput, maxCalendarEras+1)}
		}, "eras"},
		{"too many seasons", func() *ImportResult {
			return &ImportResult{Seasons: make([]Season, maxCalendarSeasons+1)}
		}, "seasons"},
		{"too many moons", func() *ImportResult {
			return &ImportResult{Moons: make([]MoonInput, maxCalendarMoons+1)}
		}, "moons"},
		{"too many events", func() *ImportResult {
			return &ImportResult{Events: make([]ExportEvent, maxCalendarImportEvents+1)}
		}, "events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.build()
			err := clampCalendarStructure(result)
			if err == nil {
				t.Fatal("expected an error for an over-limit count, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantHas) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantHas)
			}
		})
	}
}

// TestClampCalendarStructure_AtTheLimitIsAccepted is the boundary control for
// the test above: exactly maxCalendarMonths must NOT be refused, only more
// than that — pinning a >, not a >=, in checkCalendarImportLimits.
func TestClampCalendarStructure_AtTheLimitIsAccepted(t *testing.T) {
	result := &ImportResult{Months: make([]MonthInput, maxCalendarMonths)}
	for i := range result.Months {
		result.Months[i] = MonthInput{Name: "M", Days: 30, SortOrder: i}
	}
	if err := clampCalendarStructure(result); err != nil {
		t.Fatalf("exactly the maximum (%d) must be accepted, got: %v", maxCalendarMonths, err)
	}
}

// TestClampCalendarStructure_ClampsOversizedMonthDays covers the one bound
// that is truncated-with-a-warning rather than refused (see
// clampCalendarStructure's doc comment): a single absurd month length
// shouldn't sink an otherwise-fine calendar the way an absurd COUNT would.
func TestClampCalendarStructure_ClampsOversizedMonthDays(t *testing.T) {
	result := &ImportResult{
		Months: []MonthInput{{Name: "Endless", Days: maxCalendarMonthDays + 500, SortOrder: 0}},
	}
	if err := clampCalendarStructure(result); err != nil {
		t.Fatalf("clampCalendarStructure: %v", err)
	}
	if result.Months[0].Days != maxCalendarMonthDays {
		t.Errorf("month days = %d, want clamped to %d", result.Months[0].Days, maxCalendarMonthDays)
	}
	found := false
	for _, w := range result.Warnings {
		if strings.Contains(w, "maximum") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a warning about the clamped month length, got %v", result.Warnings)
	}
}

// TestClampCalendarStructure_ShortensOversizedNamesAndText covers the
// length caps on names/descriptions/weather_effect — each is a narrower
// VARCHAR/TEXT column than the source format bothers to bound, so an
// oversized value must be shortened (with a warning), never handed to
// CalendarRepository.ApplyImport to fail on raw at the driver.
func TestClampCalendarStructure_ShortensOversizedNamesAndText(t *testing.T) {
	longShortName := strings.Repeat("x", maxCalendarShortNameLength+50)
	longEraName := strings.Repeat("y", apperror.MaxNameLength+50)
	longDesc := strings.Repeat("z", apperror.MaxDescriptionLength+50)
	longWeather := strings.Repeat("w", maxCalendarWeatherEffectLength+50)

	result := &ImportResult{
		Months:   []MonthInput{{Name: longShortName, Days: 30, SortOrder: 0}},
		Weekdays: []WeekdayInput{{Name: longShortName, SortOrder: 0}},
		Moons:    []MoonInput{{Name: longShortName}},
		Eras:     []EraInput{{Name: longEraName, StartYear: 1, Description: &longDesc}},
		Seasons: []Season{{
			Name: longShortName, StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30,
			Description: &longDesc, WeatherEffect: &longWeather,
		}},
	}
	if err := clampCalendarStructure(result); err != nil {
		t.Fatalf("clampCalendarStructure: %v", err)
	}

	if n := len(result.Months[0].Name); n > maxCalendarShortNameLength {
		t.Errorf("month name length = %d, want <= %d", n, maxCalendarShortNameLength)
	}
	if n := len(result.Weekdays[0].Name); n > maxCalendarShortNameLength {
		t.Errorf("weekday name length = %d, want <= %d", n, maxCalendarShortNameLength)
	}
	if n := len(result.Moons[0].Name); n > maxCalendarShortNameLength {
		t.Errorf("moon name length = %d, want <= %d", n, maxCalendarShortNameLength)
	}
	if n := len(result.Eras[0].Name); n > apperror.MaxNameLength {
		t.Errorf("era name length = %d, want <= %d", n, apperror.MaxNameLength)
	}
	if result.Eras[0].Description == nil || len(*result.Eras[0].Description) > apperror.MaxDescriptionLength {
		t.Errorf("era description was not shortened: %v", result.Eras[0].Description)
	}
	if n := len(result.Seasons[0].Name); n > maxCalendarShortNameLength {
		t.Errorf("season name length = %d, want <= %d", n, maxCalendarShortNameLength)
	}
	if result.Seasons[0].Description == nil || len(*result.Seasons[0].Description) > apperror.MaxDescriptionLength {
		t.Errorf("season description was not shortened: %v", result.Seasons[0].Description)
	}
	if result.Seasons[0].WeatherEffect == nil || len(*result.Seasons[0].WeatherEffect) > maxCalendarWeatherEffectLength {
		t.Errorf("season weather effect was not shortened: %v", result.Seasons[0].WeatherEffect)
	}
	if len(result.Warnings) == 0 {
		t.Error("expected at least one warning about shortened text")
	}
}

// chronicleTooManyMonthsFixture, simpleCalTooManyMonthsFixture,
// calendariaTooManyMonthsFixture and fantasyCalTooManyMonthsFixture each
// build a minimal, valid-shaped file for their format with one too many
// months — plain map[string]interface{} payloads (not this package's own
// unexported parser structs) so each fixture is only as faithful to its
// format as a real uploaded file would be.

func chronicleTooManyMonthsFixture(t *testing.T) []byte {
	t.Helper()
	months := make([]map[string]any, maxCalendarMonths+1)
	for i := range months {
		months[i] = map[string]any{"name": "M", "days": 30, "sort_order": i}
	}
	raw, err := json.Marshal(map[string]any{
		"format":  "chronicle-calendar-v1",
		"version": 1,
		"calendar": map[string]any{
			"name":   "Too Big",
			"months": months,
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

func simpleCalTooManyMonthsFixture(t *testing.T) []byte {
	t.Helper()
	months := make([]map[string]any, maxCalendarMonths+1)
	for i := range months {
		months[i] = map[string]any{"name": "M", "numericRepresentation": i, "numberOfDays": 30}
	}
	raw, err := json.Marshal(map[string]any{
		"calendar": map[string]any{
			"name":   "Too Big",
			"year":   map[string]any{"numericRepresentation": 1},
			"months": months,
		},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

func calendariaTooManyMonthsFixture(t *testing.T) []byte {
	t.Helper()
	months := make(map[string]any, maxCalendarMonths+1)
	for i := 0; i <= maxCalendarMonths; i++ {
		months["m"+strconv.Itoa(i)] = map[string]any{"name": "M", "days": 30, "ordinal": i}
	}
	raw, err := json.Marshal(map[string]any{
		"name":   "Too Big",
		"months": months,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

func fantasyCalTooManyMonthsFixture(t *testing.T) []byte {
	t.Helper()
	timespans := make([]map[string]any, maxCalendarMonths+1)
	for i := range timespans {
		timespans[i] = map[string]any{"name": "M", "type": "month", "length": 30}
	}
	raw, err := json.Marshal(map[string]any{
		"name": "Too Big",
		"static_data": map[string]any{
			"year_data": map[string]any{"timespans": timespans},
			"clock":     map[string]any{"hours": 24, "minutes": 60},
		},
		"dynamic_data": map[string]any{"year": 1, "timespan": 0, "day": 0},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return raw
}

// TestDetectAndParse_RefusesOversizedMonthsPerFormat proves
// checkCalendarImportLimits runs for every import format, Chronicle's own
// included — detectFormat trusts any file that merely claims
// "format":"chronicle-calendar-v1", so that path needs the same bound as
// the three external formats, not an exemption.
func TestDetectAndParse_RefusesOversizedMonthsPerFormat(t *testing.T) {
	tests := []struct {
		name       string
		raw        func(t *testing.T) []byte
		wantFormat ImportFormat
	}{
		{"chronicle", chronicleTooManyMonthsFixture, FormatChronicle},
		{"simple calendar", simpleCalTooManyMonthsFixture, FormatSimpleCal},
		{"calendaria", calendariaTooManyMonthsFixture, FormatCalendaria},
		{"fantasy-calendar", fantasyCalTooManyMonthsFixture, FormatFantasyCal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.raw(t)
			if got := detectFormat(raw); got != tt.wantFormat {
				t.Fatalf("detectFormat = %q, want %q (fixture doesn't exercise the format under test)", got, tt.wantFormat)
			}
			_, err := DetectAndParse(raw)
			if err == nil {
				t.Fatal("expected an error for an over-limit month count, got nil")
			}
			if !strings.Contains(err.Error(), "months") {
				t.Errorf("error = %q, want it to mention months", err.Error())
			}
		})
	}
}
