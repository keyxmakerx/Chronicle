// wizard_generate_test.go covers the "Generate one" step's server side: the
// step's own gate/content, its Preview round trip into the shared Review
// step (reusing CalendarService.PreviewImport — the SAME Chronicle-format
// parser and clamp/limits an uploaded file already gets, never a bespoke
// check for this path), and WizardCreate's source="generate" branch,
// including rejection of an oversized or malformed generated payload. The
// engine itself (static/js/widgets/chronicle_gen.js) is covered by
// test/js/chronicle_gen.test.mjs — this file only proves the server-side
// half of the contract: a generated preset is held to the exact same rules
// an import or a hand-built calendar already is.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestParseGeneratedCalendarJSON_TableDriven is the direct, table-driven
// test of parseGeneratedCalendarJSON's two checks (the local size cap, then
// PreviewImport's own parse/clamp) in isolation from the HTTP layer.
func TestParseGeneratedCalendarJSON_TableDriven(t *testing.T) {
	svc := NewCalendarService(&fakeCalendarRepo{}, &fakeEventRepo{}, &fakeEventKindRepo{}, &fakeWeatherRepo{})
	oversizedMonths := make([]map[string]any, maxCalendarMonths+1)
	for i := range oversizedMonths {
		oversizedMonths[i] = map[string]any{"name": "M", "days": 30, "sort_order": i, "is_intercalary": false, "leap_year_days": 0}
	}
	oversizedCalendar, err := json.Marshal(map[string]any{
		"format": "chronicle-calendar-v1", "version": 2,
		"calendar": map[string]any{
			"name": "Too Big", "mode": "fantasy", "current_year": 1, "current_month": 1, "current_day": 1,
			"hours_per_day": 24, "minutes_per_hour": 60, "seconds_per_minute": 60,
			"leap_year_every": 0, "leap_year_offset": 0, "tracks_real_time": false,
			"months": oversizedMonths, "weekdays": []map[string]any{{"name": "Day", "sort_order": 0, "is_rest_day": false}},
		},
	})
	if err != nil {
		t.Fatalf("marshal oversized fixture: %v", err)
	}

	tests := []struct {
		name       string
		raw        string
		wantErr    bool
		wantErrHas string
		wantName   string
		wantMonths int
	}{
		{
			name:       "valid generated preset (a shipped preset standing in for the engine's own output)",
			raw:        generatedCalendarFixture(t),
			wantErr:    false,
			wantName:   "Dwarven Deep-count",
			wantMonths: 11,
		},
		{
			name:       "over the local size cap",
			raw:        strings.Repeat("a", maxGeneratedCalendarSize+1),
			wantErr:    true,
			wantErrHas: "too large",
		},
		{
			name:       "malformed JSON",
			raw:        "{not valid json",
			wantErr:    true,
		},
		{
			name:       "well-formed JSON that is not a calendar at all",
			raw:        `{"hello":"world"}`,
			wantErr:    true,
		},
		{
			name:       "structurally too large (over checkCalendarImportLimits, well under the byte cap)",
			raw:        string(oversizedCalendar),
			wantErr:    true,
			wantErrHas: "months",
		},
		{
			name:       "empty string",
			raw:        "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGeneratedCalendarJSON(context.Background(), svc, tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got a result: %+v", got)
				}
				if tt.wantErrHas != "" && !strings.Contains(err.Error(), tt.wantErrHas) {
					t.Errorf("error = %q, want it to mention %q", err.Error(), tt.wantErrHas)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.CalendarName != tt.wantName {
				t.Errorf("CalendarName = %q, want %q", got.CalendarName, tt.wantName)
			}
			if len(got.Months) != tt.wantMonths {
				t.Errorf("got %d months, want %d", len(got.Months), tt.wantMonths)
			}
		})
	}
}

func TestWizardGenerateStep_GateAndContent(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)
	const path = "/campaigns/camp-gen/calendars/wizard/generate"

	if rec := doRequest(e, http.MethodGet, path, ""); !isLoginRedirect(rec) {
		t.Errorf("expected a login redirect for an unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := doRequest(e, http.MethodGet, path, "u-player"); rec.Code != http.StatusForbidden {
		t.Errorf("a Player must be forbidden from the wizard, got %d: %s", rec.Code, rec.Body.String())
	}

	rec := doRequest(e, http.MethodGet, path, "u-owner")
	if rec.Code != http.StatusOK {
		t.Fatalf("Owner must be able to open the generate step, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Generate a calendar", "Recipe", "Familiar", "Work it out from the sky", "Naming culture", "Reroll all"} {
		if !strings.Contains(body, want) {
			t.Errorf("expected the generate step to mention %q, body:\n%s", want, body)
		}
	}
	if !strings.Contains(body, `name="generated_json"`) {
		t.Errorf("expected the hidden generated_json field the engine serializes into, body:\n%s", body)
	}
	if strings.Contains(body, "fa-solid fa-dice") || strings.Contains(body, "fa-solid fa-reroll") {
		t.Errorf("the generate step must carry no decorative icons, body:\n%s", body)
	}
}

// generatedCalendarFixture returns a real Chronicle-format export (one of
// the shipped presets) as raw JSON — standing in for what the browser's
// ChronicleGen.cal.toPreset() would produce: a generated calendar's preset
// is, by design, indistinguishable in SHAPE from a shipped export, so this
// is a realistic fixture, not a synthetic one.
func generatedCalendarFixture(t *testing.T) string {
	t.Helper()
	raw, err := presetFS.ReadFile("presets/dwarven.json")
	if err != nil {
		t.Fatalf("read fixture preset: %v", err)
	}
	return string(raw)
}

// TestWizardGeneratePreview_ValidGeneratedCalendarReachesReviewStep proves
// the round trip from the engine's output to the shared Review step:
// source="generate", the structure summary counts, and a back link to the
// Generate step rather than the plain import drop zone.
func TestWizardGeneratePreview_ValidGeneratedCalendarReachesReviewStep(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("generated_json", generatedCalendarFixture(t))

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/generate/preview", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `value="generate"`) {
		t.Errorf("expected source=\"generate\" carried into the Review step, body:\n%s", body)
	}
	if !strings.Contains(body, "wizard/generate") {
		t.Errorf("expected the Review step's Back link to return to the Generate step, body:\n%s", body)
	}
	if !strings.Contains(body, "<b>11</b>") {
		t.Errorf("expected the structure summary to count the dwarven preset's 11 months, body:\n%s", body)
	}
}

// TestWizardGeneratePreview_MalformedJSONReRendersGenerateStep covers the
// tamper-only failure path (mirrors WizardBuildPreview's own precedent): the
// engine itself never produces broken JSON, so this only fires for a
// hand-crafted request, and it must re-render the Generate step with an
// error rather than a raw 500 or blank page.
func TestWizardGeneratePreview_MalformedJSONReRendersGenerateStep(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("generated_json", "{not valid json")

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/generate/preview", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Generate a calendar") {
		t.Errorf("expected the generate step to re-render, body:\n%s", body)
	}
	if !strings.Contains(body, `class="calv5-err"`) {
		t.Errorf("expected an inline error, body:\n%s", body)
	}
}

// TestWizardGeneratePreview_OversizedPayloadIsRefused proves the local size
// cap (parseGeneratedCalendarJSON) actually fires before parseChronicle
// runs — this route is deliberately not in app.go's calendarImportPathPattern
// skip list, so Echo's own global body limit is the outer bound, but the
// handler itself must still refuse a payload under that with a clear error.
func TestWizardGeneratePreview_OversizedPayloadIsRefused(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("generated_json", strings.Repeat("a", maxGeneratedCalendarSize+1))

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/generate/preview", "u-owner", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Generate a calendar") {
		t.Errorf("expected the generate step to re-render, body:\n%s", body)
	}
	if !strings.Contains(strings.ToLower(body), "too large") {
		t.Errorf("expected the response to mention the size limit, body:\n%s", body)
	}
}

// TestWizardCreate_Generate_OversizedStructureIsRefused is the "generate"
// sibling of TestWizardCreate_Build_OversizedStructureIsRefused: a
// generated calendar that (however it got there) carries more months than
// any calendar needs must be refused at Create, not just at Preview —
// parseWizardImportJSON re-validates the round-tripped ImportResult exactly
// like "import"/"build" already do.
func TestWizardCreate_Generate_OversizedStructureIsRefused(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	months := make([]MonthInput, maxCalendarMonths+1)
	for i := range months {
		months[i] = MonthInput{Name: "M", Days: 30, SortOrder: i}
	}
	importJSON, err := json.Marshal(&ImportResult{
		Format: FormatChronicle, CalendarName: "Too Big", Months: months,
		Today: ImportedToday{Year: 1, Month: intPtrForTest(1), Day: intPtrForTest(1)},
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	form := url.Values{}
	form.Set("source", "generate")
	form.Set("import_json", string(importJSON))
	form.Set("name", "Too Big")
	form.Set("current_year", "1")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/create", "u-owner", form)
	if rec.Code == http.StatusOK && rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("an oversized generated calendar must be refused, got %d, HX-Redirect=%q, body:\n%s",
			rec.Code, rec.Header().Get("HX-Redirect"), rec.Body.String())
	}
	if rec.Header().Get("HX-Redirect") != "" {
		t.Fatalf("must not redirect on a refused generate, HX-Redirect=%q", rec.Header().Get("HX-Redirect"))
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

// TestWizardCreate_Generate_MalformedIsRefused: hand-crafted garbage for
// source="generate" must be a clean 4xx, never an unhandled panic or a 500.
func TestWizardCreate_Generate_MalformedIsRefused(t *testing.T) {
	e, _ := newRealWorldTestRouter(nil)

	form := url.Values{}
	form.Set("source", "generate")
	form.Set("import_json", "not json at all")
	form.Set("name", "Garbage")

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/create", "u-owner", form)
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("expected a 4xx for malformed import_json, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestWizardCreate_Generate_RoundTrip is the direct test of "a generated
// preset round trips to a created calendar": a realistic generated preset
// (the dwarven fixture, standing in for the engine's own output) survives
// Preview -> Review -> Create and the calendar actually created carries
// that exact structure, proving the whole path end to end rather than just
// that create "succeeds".
func TestWizardCreate_Generate_RoundTrip(t *testing.T) {
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

	// Preview: the engine's raw chronicle-calendar-v1 output.
	previewForm := url.Values{}
	previewForm.Set("generated_json", generatedCalendarFixture(t))
	previewRec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/generate/preview", "u-owner", previewForm)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview: got %d: %s", previewRec.Code, previewRec.Body.String())
	}
	// Pull the round-tripped import_json straight out of the Review step's
	// hidden field, exactly as the browser's own form submit would.
	previewBody := previewRec.Body.String()
	const marker = `name="import_json" value="`
	start := strings.Index(previewBody, marker)
	if start < 0 {
		t.Fatalf("expected a hidden import_json field in the Review step, body:\n%s", previewBody)
	}
	start += len(marker)
	end := strings.Index(previewBody[start:], `"`)
	if end < 0 {
		t.Fatalf("could not find the end of the import_json field, body:\n%s", previewBody)
	}
	importJSON := unescapeHTMLAttrForTest(previewBody[start : start+end])

	form := url.Values{}
	form.Set("source", "generate")
	form.Set("import_json", importJSON)
	form.Set("name", "Generated Calendar")
	form.Set("current_year", "1500")
	form.Set("current_month", "1")
	form.Set("current_day", "1")

	rec := doHTMXFormRequest(e, "/campaigns/camp-gen/calendars/wizard/create", "u-owner", form)
	if rec.Code != http.StatusNoContent || rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("expected a redirect on success, got %d: %s", rec.Code, rec.Body.String())
	}
	if created == nil {
		t.Fatal("expected the calendar to be created")
	}
	if created.Name != "Generated Calendar" {
		t.Errorf("Name = %q, want the owner's typed name from the Review step", created.Name)
	}
	if appliedIR == nil {
		t.Fatal("expected ApplyImport to be called")
	}
	if len(appliedIR.Months) != 11 {
		t.Errorf("got %d months, want the dwarven preset's 11", len(appliedIR.Months))
	}
}

// unescapeHTMLAttrForTest undoes templ's attribute HTML-escaping (&#34; for
// ", &amp; for &) on the hidden import_json field's value, so the test can
// feed it back in as an ordinary form value the way a browser reading its
// own DOM would.
func unescapeHTMLAttrForTest(s string) string {
	s = strings.ReplaceAll(s, "&#34;", `"`)
	s = strings.ReplaceAll(s, "&#39;", `'`)
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	return s
}
