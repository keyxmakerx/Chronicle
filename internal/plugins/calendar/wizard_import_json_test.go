// wizard_import_json_test.go covers parseWizardImportJSON: the wizard's
// Review->Create step re-validates the browser-submitted import_json field
// — both its size and its structure — the same way an uploaded file is
// validated, rather than trusting a client-round-tripped value outright.
package calendar

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	emw "github.com/labstack/echo/v4/middleware"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestParseWizardImportJSON_ValidRoundTripSucceeds(t *testing.T) {
	ir := &ImportResult{
		Format:       FormatChronicle,
		CalendarName: "Round Trip",
		Months:       []MonthInput{{Name: "Firstmonth", Days: 30, SortOrder: 0}},
	}
	raw, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	got, err := parseWizardImportJSON(string(raw))
	if err != nil {
		t.Fatalf("parseWizardImportJSON: %v", err)
	}
	if got.CalendarName != "Round Trip" {
		t.Errorf("CalendarName = %q, want %q", got.CalendarName, "Round Trip")
	}
	if len(got.Months) != 1 {
		t.Errorf("got %d months, want 1", len(got.Months))
	}
}

// TestParseWizardImportJSON_OverLimitIsRefused is the direct test of the
// size cap: app.go's isCalendarImportPath skips the global body-limit
// middleware for the wizard/create route on the assumption this function
// enforces maxCalendarImportSize itself.
func TestParseWizardImportJSON_OverLimitIsRefused(t *testing.T) {
	huge := strings.Repeat("a", maxCalendarImportSize+1)
	_, err := parseWizardImportJSON(huge)
	if err == nil {
		t.Fatal("expected an error for an over-limit import_json, got nil")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want it to mention the size limit", err.Error())
	}
}

// TestParseWizardImportJSON_TamperedStructureGetsSameValidationAsUpload
// proves a hand-crafted import_json (well under the size cap, but carrying
// more months than any calendar needs) is refused for the same reason an
// upload with the same structure would be — it goes through
// clampCalendarStructure, not a bare json.Unmarshal.
func TestParseWizardImportJSON_TamperedStructureGetsSameValidationAsUpload(t *testing.T) {
	months := make([]MonthInput, maxCalendarMonths+1)
	for i := range months {
		months[i] = MonthInput{Name: "M", Days: 30, SortOrder: i}
	}
	raw, err := json.Marshal(&ImportResult{CalendarName: "Tampered", Months: months})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	_, err = parseWizardImportJSON(string(raw))
	if err == nil {
		t.Fatal("expected the tampered import_json (too many months) to be refused")
	}
	if !strings.Contains(err.Error(), "months") {
		t.Errorf("error = %q, want it to mention months", err.Error())
	}
}

// TestWizardCreate_TamperedImportJSONIsRefused is the end-to-end sibling of
// the tests above: a real POST to /wizard/create with source=import and a
// tampered import_json must be refused by the handler, proving
// parseWizardImportJSON is actually wired into WizardCreate and not just
// correct in isolation.
// TestParseWizardImportJSON_NormalizesColors: colors arriving through the
// wizard's browser-submitted data get the same check an upload's colors get
// in each parser, so an unchecked value never reaches the store.
func TestParseWizardImportJSON_NormalizesColors(t *testing.T) {
	ir := &ImportResult{
		Format:       FormatChronicle,
		CalendarName: "Colors",
		Months:       []MonthInput{{Name: "Firstmonth", Days: 30}},
		Moons:        []MoonInput{{Name: "Luna", CycleDays: 28, Color: "red;x:y"}},
		Seasons:      []Season{{Name: "Spring", StartMonth: 1, StartDay: 1, EndMonth: 1, EndDay: 30, Color: "abc"}},
		Eras:         []EraInput{{Name: "First Age", StartYear: 1, Color: "url(x)"}},
	}
	raw, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	got, err := parseWizardImportJSON(string(raw))
	if err != nil {
		t.Fatalf("parseWizardImportJSON: %v", err)
	}
	if c := got.Moons[0].Color; c != "#808080" {
		t.Errorf("moon color = %q, want the #808080 fallback", c)
	}
	if c := got.Seasons[0].Color; c != "#aabbcc" {
		t.Errorf("season color = %q, want #aabbcc", c)
	}
	if c := got.Eras[0].Color; c != "#808080" {
		t.Errorf("era color = %q, want the #808080 fallback", c)
	}
}

func TestWizardCreate_TamperedImportJSONIsRefused(t *testing.T) {
	const campaignID = "camp-wizard-tampered-import"
	e := echo.New()
	e.Use(emw.Recover())
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		if ae, ok := err.(*apperror.AppError); ok {
			_ = c.JSON(ae.Code, map[string]string{"error": ae.Type, "message": ae.Message})
			return
		}
		_ = c.NoContent(http.StatusInternalServerError)
	}
	svc := NewCalendarService(&fakeCalendarRepo{}, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	h := NewHandler(svc)
	roles := map[string]campaigns.Role{"u-owner": campaigns.RoleOwner}
	RegisterRoutes(e, h, guardCampaignSvc{roles: roles}, guardAuthSvc{}, guardAddonSvc{enabled: true})

	months := make([]MonthInput, maxCalendarMonths+1)
	for i := range months {
		months[i] = MonthInput{Name: "M", Days: 30, SortOrder: i}
	}
	importJSON, err := json.Marshal(&ImportResult{
		CalendarName: "Tampered",
		Months:       months,
		Today:        ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	form := url.Values{}
	form.Set("source", "import")
	form.Set("import_json", string(importJSON))
	form.Set("name", "My Calendar")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/"+campaignID+"/calendars/wizard/create", "u-owner", form)

	if rec.Code == http.StatusOK || rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("a tampered import_json (too many months) must be refused, got %d, HX-Redirect=%q, body:\n%s",
			rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "months") {
		t.Errorf("expected the response to mention the month-count limit, body:\n%s", rec.Body.String())
	}
}
