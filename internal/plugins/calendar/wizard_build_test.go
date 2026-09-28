// wizard_build_test.go covers the "Build your own" structure editor's
// server side: the editor step itself, its preview round trip into the
// shared Review step, and WizardCreate's source="build" branch — including
// the same re-validation an uploaded file's import_json already gets
// (parseWizardImportJSON), proved end to end through the real HTTP handler
// rather than just the helper function wizard_import_json_test.go already
// covers in isolation.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestWizardBuildStep_GateAndContent(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)
	const path = "/campaigns/camp-build/calendars/wizard/build"

	if rec := doRequest(e, http.MethodGet, path, ""); !isLoginRedirect(rec) {
		t.Errorf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(e, http.MethodGet, path, "u-player"); rec.Code != http.StatusForbidden {
		t.Errorf("a Player must be forbidden from the wizard, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(e, http.MethodGet, path, "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner must be able to open the build step, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Build your own", "Months", "Weekdays", "Leap rule", "Moons", "Era", "Month 1", "Day 1"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the build step to mention %q, body:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `name="import_json"`) {
		t.Errorf("expected the hidden import_json field the editor serializes into, body:\n%s", body)
	}
}

// buildImportJSON returns the ImportResult JSON the "Build your own" editor
// would submit for a small, realistic hand-built calendar — standing in for
// Alpine's buildJSON() (see wizardBuildXData's own doc comment) so the
// server-side path can be tested without a browser.
func buildImportJSON(t *testing.T, monthCount int) string {
	t.Helper()
	months := make([]MonthInput, monthCount)
	for i := range months {
		leap := 0
		if i == len(months)-1 {
			leap = 1
		}
		months[i] = MonthInput{Name: "M", Days: 30, SortOrder: i, LeapYearDays: leap}
	}
	ir := ImportResult{
		Format:       FormatBuilt,
		CalendarName: "My calendar",
		Months:       months,
		Weekdays: []WeekdayInput{
			{Name: "Firstday", SortOrder: 0}, {Name: "Secondday", SortOrder: 1},
		},
		Moons: []MoonInput{{Name: "Luna", CycleDays: 29.5}},
		Eras:  []EraInput{{Name: "Age of Heroes", StartYear: 1}},
		Settings: ImportedSettings{
			Mode: ModeFantasy, HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
			LeapYearEvery: 4,
		},
		Today: ImportedToday{Year: 1},
	}
	raw, err := json.Marshal(ir)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(raw)
}

// TestWizardBuildPreview_ValidStructureReachesReviewStep proves the round
// trip from the editor's submitted structure to the shared Review step:
// source="build", the structure summary counts, and a back link to the
// editor rather than the import drop zone.
func TestWizardBuildPreview_ValidStructureReachesReviewStep(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("import_json", buildImportJSON(t, 3))

	rec := doHTMXFormRequest(e, "/campaigns/camp-build/calendars/wizard/build/preview", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="build"`) {
		t.Errorf("expected source=\"build\" carried into the Review step, body:\n%s", body)
	}
	if !strings.Contains(body, "wizard/build") {
		t.Errorf("expected the Review step's Back link to return to the editor, body:\n%s", body)
	}
	if !strings.Contains(body, "My calendar") {
		t.Errorf("expected the built calendar's name pre-filled, body:\n%s", body)
	}
	if !strings.Contains(body, "<b>3</b>") {
		t.Errorf("expected the structure summary to count the 3 built months, body:\n%s", body)
	}
}

// TestWizardBuildPreview_MalformedJSONReRendersEditor covers the (only
// realistically-tamper-reachable, per WizardBuildPreview's own doc comment)
// failure path: broken JSON re-renders the editor with an error rather than
// a raw 500 or a blank page.
func TestWizardBuildPreview_MalformedJSONReRendersEditor(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("import_json", "{not valid json")

	rec := doHTMXFormRequest(e, "/campaigns/camp-build/calendars/wizard/build/preview", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Build your own") {
		t.Errorf("expected the editor step to re-render, body:\n%s", body)
	}
	if !strings.Contains(body, `class="calv5-err"`) {
		t.Errorf("expected an inline error, body:\n%s", body)
	}
}

// TestWizardCreate_Build_OversizedStructureIsRefused is the "build" sibling
// of TestWizardCreate_TamperedImportJSONIsRefused: the exact same
// parseWizardImportJSON re-validation the import path gets, proved for the
// new source value too — an oversized hand-built calendar must be refused,
// not created.
func TestWizardCreate_Build_OversizedStructureIsRefused(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("source", "build")
	form.Set("import_json", buildImportJSON(t, maxCalendarMonths+1))
	form.Set("name", "Too Big")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/camp-build/calendars/wizard/create", "u-owner", form)
	if rec.Code == http.StatusOK && rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("an oversized built calendar must be refused, got %d, HX-Redirect=%q, body:\n%s",
			rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("must not redirect on a refused build, HX-Redirect=%q", rec.Header().Get("HX-Redirect"))
	}
	body := rec.Body.String()
	var ae struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ae); err == nil && ae.Message != "" {
		body = ae.Message
	}
	if !strings.Contains(body, "months") {
		t.Errorf("expected the response to mention the month-count limit, body:\n%s", body)
	}
}

// TestWizardCreate_Build_MalformedIsRefused: hand-crafted garbage in
// import_json for source="build" must be a clean 4xx (apperror.NewBadRequest,
// via parseWizardImportJSON), never an unhandled panic or a 500.
func TestWizardCreate_Build_MalformedIsRefused(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("source", "build")
	form.Set("import_json", "not json at all")
	form.Set("name", "Garbage")

	rec := doHTMXFormRequest(e, "/campaigns/camp-build/calendars/wizard/create", "u-owner", form)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for malformed import_json, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "read back") && !strings.Contains(rec.Body.String(), "error") {
		t.Errorf("expected a clean error body, got:\n%s", rec.Body.String())
	}
}

// TestWizardCreate_Build_RoundTrip is the direct test of "a round trip for
// a built calendar": a realistic hand-built structure (months with a leap
// day on the last one, weekdays, a moon, an era, a leap rule) survives
// source="build" all the way to CreateCalendarFromImport, and the calendar
// actually created carries that exact structure — proving the whole
// editor -> preview -> review -> create path, not just that create
// "succeeds".
func TestWizardCreate_Build_RoundTrip(t *testing.T) {
	var created *Calendar
	var appliedIR *ImportResult
	calRepo := &fakeCalendarRepo{
		createFn: func(_ context.Context, cal *Calendar) error {
			created = cal
			return nil
		},
		applyImportFn: func(_ context.Context, cal *Calendar, ir *ImportResult) error {
			appliedIR = ir
			return nil
		},
	}
	e, _ := newRealWorldTestRouter(calRepo)

	form := url.Values{}
	form.Set("source", "build")
	form.Set("import_json", buildImportJSON(t, 4))
	form.Set("name", "Hand-Built Calendar")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/camp-build/calendars/wizard/create", "u-owner", form)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("expected a redirect on success, got %d: %s", rec.Code, rec.Body.String())
	}
	if created == nil {
		t.Fatal("expected the calendar to be created")
	}
	if created.Name != "Hand-Built Calendar" {
		t.Errorf("Name = %q, want the owner's typed name from the Review step", created.Name)
	}
	if created.Mode != ModeFantasy {
		t.Errorf("Mode = %q, want %q", created.Mode, ModeFantasy)
	}
	if appliedIR == nil {
		t.Fatal("expected ApplyImport to be called")
	}
	if len(appliedIR.Months) != 4 {
		t.Fatalf("got %d months, want the 4 built", len(appliedIR.Months))
	}
	if appliedIR.Months[3].LeapYearDays != 1 {
		t.Errorf("last month's LeapYearDays = %d, want 1 (the built leap day)", appliedIR.Months[3].LeapYearDays)
	}
	if len(appliedIR.Weekdays) != 2 || appliedIR.Weekdays[0].Name != "Firstday" {
		t.Errorf("Weekdays = %+v, want the 2 built weekdays", appliedIR.Weekdays)
	}
	if len(appliedIR.Moons) != 1 || appliedIR.Moons[0].Name != "Luna" {
		t.Errorf("Moons = %+v, want the 1 built moon", appliedIR.Moons)
	}
	if len(appliedIR.Eras) != 1 || appliedIR.Eras[0].Name != "Age of Heroes" {
		t.Errorf("Eras = %+v, want the 1 built era", appliedIR.Eras)
	}
}
