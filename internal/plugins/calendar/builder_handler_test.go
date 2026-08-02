// builder_handler_test.go — the route contract and the security review as
// executable assertions (C-CALV4-WIZARD-P13 §§7.2–7.4).
//
// A security review that lives only in a PR body is a paragraph. Each claim
// below is the same claim as a test.
package calendar

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// builderFormCtx builds an echo context carrying a urlencoded body.
func builderFormCtx(t *testing.T, form url.Values) echo.Context {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

// builderFormFor serialises a draft the way the rendered page does — through
// builderCarryFields — so the round-trip test exercises the SHIPPED pairing of
// writer and reader rather than a hand-written body.
func builderFormFor(d *builderDraft, step int, importer bool) url.Values {
	form := url.Values{}
	form.Set("step", builderStations[step].Key)
	form.Set("pv_month", "0")
	if importer {
		form.Set("importer", "1")
	}
	for _, f := range builderCarryFields(d, step, importer) {
		form.Add(f.Name, f.Value)
	}
	// The owning station's controls, which the carry deliberately omits. Either
	// month station emits the WHOLE ordered name list — visible for its own
	// family, hidden in place for the other — because the two lists are zipped
	// by index on the way back.
	if builderStationOwns(step, importer, "month-list") {
		for _, m := range d.Months {
			form.Add("m_name", m.Name)
		}
	}
	if builderStationOwns(step, importer, "weekday") {
		for _, w := range d.Weekdays {
			form.Add("wd", w)
		}
	}
	if builderStationOwns(step, importer, "moon") {
		for _, m := range d.Moons {
			form.Add("moon_name", m.Name)
			form.Add("moon_period", builderNum(m.Period))
			form.Add("moon_newat", builderNum(m.NewAt))
		}
	}
	if builderStationOwns(step, importer, "season") {
		for _, s := range d.Seasons {
			form.Add("season_name", s.Name)
		}
	}
	if builderStationOwns(step, importer, "era") {
		for _, e := range d.Eras {
			form.Add("era_name", e.Name)
			form.Add("era_code", e.Code)
			form.Add("era_year", builderNum(float64(e.StartYear)))
		}
	}
	if builderStationOwns(step, importer, "leap") {
		form.Set("leap_name", d.LeapName)
		form.Set("leap_after", d.LeapAfter)
	}
	if builderStationOwns(step, importer, "identity") {
		form.Set("cal_name", d.Name)
		form.Set("epoch", d.EpochName)
		form.Set("year", builderNum(float64(d.Year)))
		form.Set("tz", d.TimeZone)
	}
	return form
}

// TestBuilderForm_TheDraftRoundTripsThroughTheForm is what makes "no draft
// store" work. Every station must be able to carry the WHOLE declaration, or a
// field silently resets on a station change — which produces a calendar with
// the right month count and the wrong month names, and nothing to notice it by.
func TestBuilderForm_TheDraftRoundTripsThroughTheForm(t *testing.T) {
	want := fxBuilderDraft()
	want.Months = append(want.Months, builderMonth{Name: "Midwinter", Days: 1, Intercalary: true})

	for step := range builderStations {
		t.Run(builderStations[step].Key, func(t *testing.T) {
			c := builderFormCtx(t, builderFormFor(want, step, false))
			got, gotStep, importer, _, _, err := builderReadForm(c)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if gotStep != step || importer {
				t.Errorf("station = %d/%v, want %d/false", gotStep, importer, step)
			}
			builderAssertSameDraft(t, got, want)
		})
	}

	// And through the importer's mode flag, which owns no fields at all.
	c := builderFormCtx(t, builderFormFor(want, 0, true))
	got, _, importer, _, _, err := builderReadForm(c)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !importer {
		t.Error("the importer mode flag must survive the round trip")
	}
	builderAssertSameDraft(t, got, want)
}

func builderAssertSameDraft(t *testing.T, got, want *builderDraft) {
	t.Helper()
	if got.Name != want.Name || got.EpochName != want.EpochName || got.Year != want.Year {
		t.Errorf("scalars drifted: %q/%q/%d vs %q/%q/%d",
			got.Name, got.EpochName, got.Year, want.Name, want.EpochName, want.Year)
	}
	if got.Mode != want.Mode || got.Preset != want.Preset {
		t.Errorf("identity drifted: %q/%q", got.Mode, got.Preset)
	}
	if got.Hue != want.Hue || got.Pattern != want.Pattern || got.Letter != want.Letter {
		t.Errorf("the identity triple drifted: %q/%q/%q", got.Hue, got.Pattern, got.Letter)
	}
	if got.LeapEvery != want.LeapEvery || got.LeapAdd != want.LeapAdd {
		t.Errorf("the leap rule drifted: %d/%d", got.LeapEvery, got.LeapAdd)
	}
	if len(got.Months) != len(want.Months) {
		t.Fatalf("months: %d, want %d", len(got.Months), len(want.Months))
	}
	for i := range want.Months {
		if got.Months[i] != want.Months[i] {
			t.Errorf("month %d = %+v, want %+v — INDEX ALIGNMENT IS THE WHOLE RISK here",
				i, got.Months[i], want.Months[i])
		}
	}
	if strings.Join(got.Weekdays, "|") != strings.Join(want.Weekdays, "|") {
		t.Errorf("weekdays = %v, want %v", got.Weekdays, want.Weekdays)
	}
	if len(got.Moons) != len(want.Moons) {
		t.Fatalf("moons: %d, want %d", len(got.Moons), len(want.Moons))
	}
	for i := range want.Moons {
		if got.Moons[i] != want.Moons[i] {
			t.Errorf("moon %d = %+v, want %+v", i, got.Moons[i], want.Moons[i])
		}
	}
	if len(got.Seasons) != len(want.Seasons) {
		t.Fatalf("seasons: %d, want %d", len(got.Seasons), len(want.Seasons))
	}
	for i := range want.Seasons {
		if got.Seasons[i] != want.Seasons[i] {
			t.Errorf("season %d = %+v, want %+v", i, got.Seasons[i], want.Seasons[i])
		}
	}
	if len(got.Eras) != len(want.Eras) {
		t.Fatalf("eras: %d, want %d", len(got.Eras), len(want.Eras))
	}
	for i := range want.Eras {
		if got.Eras[i] != want.Eras[i] {
			t.Errorf("era %d = %+v, want %+v", i, got.Eras[i], want.Eras[i])
		}
	}
}

// TestBuilderForm_RejectsAnUnknownStation. §7.2's reject-don't-clamp, on the
// one query parameter and on the one form field that carry a station key.
func TestBuilderForm_RejectsAnUnknownStation(t *testing.T) {
	form := builderFormFor(fxBuilderDraft(), 1, false)
	form.Set("step", "elsewhere")
	if _, _, _, _, _, err := builderReadForm(builderFormCtx(t, form)); err == nil {
		t.Error("an unknown station key is a 400, not a clamp to Start")
	}

	form = builderFormFor(fxBuilderDraft(), 1, false)
	form.Set("wz_step", "elsewhere")
	if _, _, _, _, _, err := builderReadForm(builderFormCtx(t, form)); err == nil {
		t.Error("an unknown station BUTTON value is a 400 too")
	}

	form = builderFormFor(fxBuilderDraft(), 1, false)
	form.Set("wz_preset", "nonesuch")
	if _, _, _, _, _, err := builderReadForm(builderFormCtx(t, form)); err == nil {
		t.Error("an unknown preset is a 400 — never a silent substitution")
	}
}

// TestBuilderPreview_TakesNoCalendarIdentity is §7.3's W5a assertion, and it is
// the reason this route has no IDOR surface at all: it cannot be ASKED to
// render an existing calendar, because there is nothing in its input that names
// one. A body that smuggles the fields in is simply ignored.
func TestBuilderPreview_TakesNoCalendarIdentity(t *testing.T) {
	form := builderFormFor(fxBuilderDraft(), 1, false)
	for _, smuggled := range []string{"calendar_id", "calId", "entity_id", "id", "host"} {
		form.Set(smuggled, "cal-someone-elses")
	}
	got, _, _, _, _, err := builderReadForm(builderFormCtx(t, form))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	block := builderPreviewBlock(got, 0)
	if block.CalendarID != builderDraftID || block.CalendarSlug != builderDraftID {
		t.Fatalf("the preview resolved an identity from the body: id=%q slug=%q",
			block.CalendarID, block.CalendarSlug)
	}

	// The DRAFT slug is also what namespaces the ANSWER keys, which is what
	// makes keeping the Block's data-day emission safe: no real calendar's keys
	// can collide with a draft's, because no calendar row has this id.
	if !strings.HasPrefix(block.CalendarSlug, "draft") {
		t.Errorf("the ANSWER-key namespace is the draft slug; got %q", block.CalendarSlug)
	}
}

// TestBuilderPreview_EscapesAuthoredNames. §7.3 requires this asserted rather
// than relied on silently. templ escapes by default; this is the test that says
// the wizard is actually using that default on the field an author controls.
func TestBuilderPreview_EscapesAuthoredNames(t *testing.T) {
	const payload = `<script>alert('x')</script>`

	d := fxBuilderDraft()
	d.Months[0].Name = payload
	d.Weekdays[0] = payload
	d.Moons[0].Name = payload
	d.Eras[0].Name = payload
	d.Name = payload

	data := builderView("camp-1", "tok", d, 1, false, 0)
	var sb strings.Builder
	if err := BuilderShellFragment(data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	if strings.Contains(html, "<script>alert") {
		t.Fatal("an authored month name reached the DOM as markup — every name on this " +
			"surface is operator-authored and is rendered as TEXT")
	}
	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("the escaped form is missing — the name did not render at all, which is " +
			"a different bug and not a fix")
	}
}

// TestBuilderView_TheFragmentsAreTheTwoTheRouteAnswersWith.
//
// One route, two fragment shapes, chosen by what the caller did — which is what
// keeps the route budget at THREE rather than four. A data change must move the
// preview ONLY, or the input being typed into loses its focus and its caret on
// every debounce.
func TestBuilderView_TheFragmentsAreTheTwoTheRouteAnswersWith(t *testing.T) {
	data := builderView("camp-1", "tok", fxBuilderDraft(), 1, false, 0)

	var shell, preview strings.Builder
	if err := BuilderShellFragment(data).Render(context.Background(), &shell); err != nil {
		t.Fatal(err)
	}
	if err := BuilderPreviewFragment(data).Render(context.Background(), &preview); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(shell.String(), `id="wz-shell"`) {
		t.Error("the shell fragment must carry the id its swap targets")
	}
	if !strings.Contains(preview.String(), `id="wz-live"`) {
		t.Error("the preview fragment must carry the id its swap targets")
	}
	if strings.Contains(preview.String(), `id="wz-shell"`) {
		t.Error("the preview fragment must NOT carry the shell — swapping the shell on " +
			"every keystroke would blow away the focused input")
	}
	if !strings.Contains(shell.String(), `id="wz-live"`) {
		t.Error("the shell contains the preview, so a station change repaints both")
	}
}

// TestBuilderForm_ActionsCannotStepPastTheValidator. A control that can produce
// a draft the validator then refuses is a control that produces an error page
// out of a click on a plus sign.
func TestBuilderForm_ActionsCannotStepPastTheValidator(t *testing.T) {
	d := fxBuilderDraft()

	// The day stepper stops at the SUBMIT ceiling, not at an arbitrary one.
	d.Months[0].Days = builderLimits.MaxMonthDays
	builderApplyAct(d, builderActValue("mdays", 0, 1), 0)
	if d.Months[0].Days != builderLimits.MaxMonthDays {
		t.Errorf("the day stepper stepped past the ceiling: %d", d.Months[0].Days)
	}
	d.Months[0].Days = 0
	builderApplyAct(d, builderActValue("mdays", 0, -1), 0)
	if d.Months[0].Days != 0 {
		t.Errorf("the day stepper went below zero: %d — zero IS the fault state and the "+
			"floor", d.Months[0].Days)
	}

	// The week stepper grows names as it grows the week, so a longer week is
	// never a week with unnamed days.
	d = fxBuilderDraft()
	builderApplyAct(d, builderActValue("week", -1, 1), 0)
	if len(d.Weekdays) != 11 || d.Weekdays[10] == "" {
		t.Errorf("a grown week must arrive named: %v", d.Weekdays)
	}
	for i := 0; i < 40; i++ {
		builderApplyAct(d, builderActValue("week", -1, 1), 0)
	}
	if len(d.Weekdays) != builderLimits.MaxWeekdays {
		t.Errorf("the week stepper stepped past the ceiling: %d", len(d.Weekdays))
	}
	for i := 0; i < 60; i++ {
		builderApplyAct(d, builderActValue("week", -1, -1), 0)
	}
	if len(d.Weekdays) != builderLimits.MinWeekdays {
		t.Errorf("the week stepper went below the floor: %d", len(d.Weekdays))
	}

	// THE LAST MONTH IS NEVER REMOVED by a delete button. A monthless calendar
	// is a state the Block draws honestly, but not one a click should produce.
	d = fxBuilderDraft()
	d.Months = d.Months[:1]
	builderApplyAct(d, builderActValue("month-del", 0, 0), 0)
	if len(d.Months) != 1 {
		t.Error("the delete button emptied the calendar")
	}

	// Every add stops at its own list bound.
	d = fxBuilderDraft()
	for i := 0; i < builderLimits.MaxMoons+5; i++ {
		builderApplyAct(d, "moon-add", 0)
	}
	if len(d.Moons) > builderLimits.MaxMoons {
		t.Errorf("moons: %d past the %d bound", len(d.Moons), builderLimits.MaxMoons)
	}

	// And whatever any of them produced still validates.
	if err := validateBuilderDraft(d); err != nil {
		t.Errorf("a draft built entirely by clicking must validate: %v", err)
	}
}

// TestBuilderForm_ThePagerWrapsAndSurvivesAShorterStructure. The pager cursor is
// a UI position, not authored data: shortening the structure under it must move
// it, never 400 the preview.
func TestBuilderForm_ThePagerWrapsAndSurvivesAShorterStructure(t *testing.T) {
	d := fxBuilderDraft()
	if got := builderApplyAct(d, "pv-prev", 0); got != len(d.Months)-1 {
		t.Errorf("the pager wraps backwards to the last month; got %d", got)
	}
	if got := builderApplyAct(d, "pv-next", len(d.Months)-1); got != 0 {
		t.Errorf("the pager wraps forwards to the first month; got %d", got)
	}
	d.Months = d.Months[:2]
	if got := builderApplyAct(d, "", 9); got != 0 {
		t.Errorf("a cursor past the end of a shortened structure clamps; got %d", got)
	}
}

// TestBuilderPage_RendersTheNineStationsAndTheRealBlock is the DOM-level pin
// that the page is the surface the dispatch describes — and, critically, that
// there is NO SECOND MONTH RENDERER in it.
func TestBuilderPage_RendersTheNineStationsAndTheRealBlock(t *testing.T) {
	data := builderView("camp-1", "tok", fxBuilderDraft(), 0, false, 0)
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Imix"}}

	var sb strings.Builder
	if err := BuilderPage(cc, data).Render(context.Background(), &sb); err != nil {
		t.Fatalf("render: %v", err)
	}
	html := sb.String()

	for _, s := range builderStations {
		if !strings.Contains(html, `data-step-pick="`+s.Key+`"`) {
			t.Errorf("station %q has no rail control — every station is a real focusable "+
				"button and navigation is non-linear from station one", s.Key)
		}
	}
	// Guard B3: a control ends in -pick and never reuses an <html> state-marker
	// noun. `data-step` would have been exactly that mistake.
	if strings.Contains(html, `data-step=`) {
		t.Error("guard B3: a control attribute must be data-step-pick, never data-step")
	}

	// THE PREVIEW IS THE SHIPPED BLOCK.
	if !strings.Contains(html, "cal-block-host") {
		t.Error("the real Block did not render inside the preview column")
	}
	// …and there is NO second month renderer in the diff.
	for _, forbidden := range []string{"pv-grid", "grid-template-columns:repeat(var(--week-len"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("a second month renderer (%q) reached the DOM", forbidden)
		}
	}

	// The route strings the page posts to are the three declared ones.
	if !strings.Contains(html, "/campaigns/camp-1/calendars/builder/preview") {
		t.Error("the preview route is not wired")
	}
	if !strings.Contains(html, `action="/campaigns/camp-1/calendars/builder"`) {
		t.Error("the create route is not the form's action")
	}
	// The stylesheet is linked through AssetURL, never as a bare /static/ href.
	if !strings.Contains(html, "calendar-builder.css") {
		t.Error("the wizard's sheet is not linked")
	}
}

// TestBuilderPage_TheNeedsBackendChipsAreOwnerOnlyByConstruction.
//
// The chip is GM-facing honesty copy that never renders to a player. On this
// surface that is satisfied by the ROLE FLOOR rather than by a branch — so the
// thing to assert is that the surface HAS no role branch to get wrong, and that
// the chips it does render are only where the text is literally about a backend
// gap (LAYERS-P9 §10's rule).
func TestBuilderPage_TheNeedsBackendChipsAreOwnerOnlyByConstruction(t *testing.T) {
	d, err := builderPresetDraft("gregorian")
	if err != nil {
		t.Fatal(err)
	}
	data := builderView("camp-1", "tok", d, len(builderStations)-1, false, 0)

	var sb strings.Builder
	if err := BuilderShellFragment(data).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	html := sb.String()

	// Every `wz-need` chip reads "needs backend" in full and nothing else.
	for _, frag := range strings.Split(html, `wz-badge wz-need`)[1:] {
		cut := frag
		if i := strings.Index(cut, "</span>"); i >= 0 {
			cut = cut[:i]
		}
		if !strings.Contains(cut, "needs backend") {
			t.Errorf("a wz-need chip reads %q — the SIGNED chip is reserved for genuine "+
				"backend gaps and its text is literally 'needs backend'", strings.TrimSpace(cut))
		}
	}
	// And the two the wave actually ships are both present: the un-storable leap
	// clause, and the identity triple that has no columns.
	if n := strings.Count(html, "wz-badge wz-need"); n < 2 {
		t.Errorf("expected the leap-exception and identity chips; found %d wz-need chips", n)
	}
}

// TestBuilderShow_RejectsAnUnknownStationOnTheQuery closes a hole a mutation
// found: replacing ShowBuilder's `return apperror.NewBadRequest("unknown wizard
// step")` with `step = 0` left the ENTIRE package suite green.
//
// TestBuilderForm_RejectsAnUnknownStation exercises builderReadForm — the POST
// body's station key — and nothing covered the GET's `?step=`, which is the
// half a bookmark, a link or a hand-typed URL actually reaches. §7.2 is
// explicit that this route "accepts ?step= only, validated against the nine
// station keys — reject an unknown value with a 400, do not clamp it silently"
// ([LYR-4]'s reject-don't-drop pattern), and behaviour that is correct but
// unpinned is behaviour one refactor from being wrong.
func TestBuilderShow_RejectsAnUnknownStationOnTheQuery(t *testing.T) {
	h := &Handler{}
	show := func(query string) (int, error) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/calendars/builder"+query, nil)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("campaign_context", &campaigns.CampaignContext{
			Campaign: &campaigns.Campaign{ID: "camp-1", Name: "Imix"}})
		return rec.Code, h.ShowBuilder(c)
	}

	// Every one of the nine station keys is accepted, and so is no key at all.
	keys := []string{""}
	for _, s := range builderStations {
		keys = append(keys, s.Key)
	}
	if len(keys) != 10 {
		t.Fatalf("nine stations (Start + 8) plus the no-key case; got %d", len(keys))
	}
	for _, key := range keys {
		q := ""
		if key != "" {
			q = "?step=" + key
		}
		code, err := show(q)
		if err != nil {
			t.Errorf("station %q is one of the nine and must render; got %v", key, err)
		}
		if code != http.StatusOK {
			t.Errorf("station %q rendered %d, want 200", key, code)
		}
	}

	// And anything else is a 400 — never a clamp to Start.
	// NB "%20" decodes to a trailing space, which is NOT trimmed into a match —
	// only a wholly blank value means "no station named, open at Start".
	for _, bad := range []string{"notastation", "9", "-1", "Start", "review%20", "moons/"} {
		code, err := show("?step=" + bad)
		if err == nil {
			t.Errorf("?step=%q rendered %d instead of rejecting — a wrong bookmark must "+
				"not become a right-looking page", bad, code)
			continue
		}
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) || appErr.Code != http.StatusBadRequest {
			t.Errorf("?step=%q must be a 400; got %v", bad, err)
		}
	}
}
