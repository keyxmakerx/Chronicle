// year_absent_test.go — "an emptied Year field moved the world to year zero."
//
// ── THE MEASUREMENT ────────────────────────────────────────────────────────
//
// A driven parity sweep cleared the Year input on the GM's Set-date control and
// submitted. The world moved to year 0. HTTP 200, stored, and the calendar then
// rendered year 0 everywhere. It reproduced in BOTH writers — the legacy V2
// console (`gm_panel.js`) and the v4 Bench date-verb row shipped by
// C-CALV4-GAMEREADY (`calendar_daycard.js`) — because both read the coordinate
// with `parseInt(el.value, 10)` and a fallback of **0**. Every other coordinate
// was range-checked (month 99 and month 0 are both refused by
// `SetWorldState`); year alone was not.
//
// ── IS YEAR 0 LEGITIMATE? YES — SO ZERO IS NOT WHAT GETS BANNED ────────────
//
// This codebase has already ruled, twice, that year 0 is a real year:
//
//   - worldstate_service.go's BuildWorldStateSeed header: "year 0 is a valid
//     year so it is only defaulted when month/day are also unset."
//   - the weather-on-date bounds check: "The year is deliberately
//     unconstrained: fantasy calendars may use year 0 or negative era years."
//     calendar_weather_on_date_test.go exercises a NEGATIVE year end-to-end.
//
// So the defect is not "zero was accepted". The defect is that **ABSENT and
// ZERO were the same value** by the time the server saw them. The fix keeps
// zero settable and makes absence expressible, refused, or preserved — never
// silently written.
//
// ── WHERE EACH HALF OF THE FIX LIVES ───────────────────────────────────────
//
//  1. `PUT /api/v1/…/calendar/date` (api_handler.go) is the SERVER-SIDE half,
//     and it is the half no client fix can reach: `apiDate.Year` was a plain
//     `int`, so a body that simply omits `year` decodes to 0 and the handler
//     writes year 0. Month and day were already guarded (`< 1`); year was not.
//     This is live for the Foundry module, whose four push sites all send
//     `year: date.year` — a `undefined` from a calendar adapter is dropped by
//     JSON.stringify and arrives as an absent key.
//
//  2. `PUT /campaigns/:id/calendar/world-state` (worldstate_handler.go) already
//     distinguished absent from zero on the wire (`*int`), so its hole was the
//     one the browsers demonstrated: a coordinate that arrives EMPTY. It is now
//     expressible and refused BY NAME instead of collapsing into a generic
//     "invalid request" — or, worse, into a number the GM never typed.
//
//  3. Both browser writers now refuse to submit an emptied coordinate rather
//     than substituting one. That is measured here in a real headless Chromium
//     over the real templates and the real shipped JS, because a `parseInt`
//     fallback is exactly the kind of defect that reads as correct in source.
package calendar

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ── 1. the server-side half: PUT /api/v1/…/calendar/date ───────────────────

// TestPutDate_AbsentYearDoesNotMoveTheWorld is the RED case, server-side.
//
// Before the fix this body answered 200 and wrote CurrentYear 0. There is no
// client-side change that helps here: the caller is the Foundry module (or any
// token holder), and an absent key is indistinguishable from a zero one once
// the struct field is a plain `int`.
func TestPutDate_AbsentYearDoesNotMoveTheWorld(t *testing.T) {
	h, svc, _ := newTestHandler(t)
	before := svc.calendar.CurrentYear // 1492

	rec := invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date",
		"camp-1", "", "ok-token", []byte(`{"month":3,"day":15,"hour":9,"minute":45}`))

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Fatalf("a body with NO year answered %d — an absent year must be refused, "+
			"not written as year 0; body=%s", rec.Code, rec.Body.String())
	}
	if svc.calendar.CurrentYear != before {
		t.Fatalf("the stored year moved from %d to %d on a body that never mentioned a year",
			before, svc.calendar.CurrentYear)
	}
	var errBody map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, rec.Body.String())
	}
	if !strings.Contains(strings.ToLower(errBody["message"]), "year") {
		t.Errorf("the refusal does not name the field: %q — a GM cannot fix an "+
			"error that does not say which coordinate was missing", errBody["message"])
	}
}

// TestPutDate_ExplicitYearZeroIsStillSettable is the OTHER direction, and it is
// the reason the fix is not `if year == 0 { reject }`.
//
// A fantasy calendar may genuinely stand in year 0 (see this file's header).
// Banning the value would trade a silent data loss for a loud one.
func TestPutDate_ExplicitYearZeroIsStillSettable(t *testing.T) {
	h, svc, _ := newTestHandler(t)

	rec := invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date",
		"camp-1", "", "ok-token", []byte(`{"year":0,"month":3,"day":15,"hour":9,"minute":45}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("an EXPLICIT year 0 answered %d, want 200 — year 0 is a real year "+
			"on a fantasy calendar; body=%s", rec.Code, rec.Body.String())
	}
	if svc.calendar.CurrentYear != 0 {
		t.Fatalf("stored year = %d after an explicit year:0, want 0", svc.calendar.CurrentYear)
	}
}

// TestPutDate_NegativeYearIsStillSettable pins the era-before-epoch case the
// weather-on-date bounds check already relies on.
func TestPutDate_NegativeYearIsStillSettable(t *testing.T) {
	h, svc, _ := newTestHandler(t)

	rec := invoke(h, http.MethodPut, "/api/v1/campaigns/camp-1/calendar/date",
		"camp-1", "", "ok-token", []byte(`{"year":-120,"month":3,"day":15,"hour":0,"minute":0}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("a negative year answered %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if svc.calendar.CurrentYear != -120 {
		t.Fatalf("stored year = %d, want -120", svc.calendar.CurrentYear)
	}
}

// ── 2. the world-state PUT: absent preserves, blank is refused BY NAME ─────

// TestPutWorldState_YearCoordinateAbsentBlankZero drives all four states of the
// year coordinate through the REAL handler against a REAL MariaDB, because the
// question ("what is stored afterwards") is the one a fake cannot answer.
func TestPutWorldState_YearCoordinateAbsentBlankZero(t *testing.T) {
	if testing.Short() {
		t.Skip("world-state year test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID, cal := calTestSeedNavCalendar(t, db)
	svc := NewCalendarService(NewCalendarRepository(db))
	h := NewHandler(svc)

	put := func(t *testing.T, body string) error {
		t.Helper()
		e := echo.New()
		req := httptest.NewRequest(http.MethodPut,
			"/campaigns/"+campaignID+"/calendar/world-state?calendarId="+cal.ID,
			strings.NewReader(body))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		c := e.NewContext(req, httptest.NewRecorder())
		c.SetParamNames("id")
		c.SetParamValues(campaignID)
		c.Set("campaign_context", &campaigns.CampaignContext{
			Campaign:   &campaigns.Campaign{ID: campaignID, Name: "Imix"},
			MemberRole: campaigns.RoleOwner,
		})
		c.Set("auth_user_id", "u-gm")
		return h.PutWorldState(c)
	}
	date := func(t *testing.T) (int, int, int) {
		t.Helper()
		got, err := svc.GetCalendarByID(t.Context(), cal.ID)
		if err != nil {
			t.Fatalf("re-reading the calendar: %v", err)
		}
		return got.CurrentYear, got.CurrentMonth, got.CurrentDay
	}

	// The fixture stands on 1523/1/14.
	if y, _, _ := date(t); y != 1523 {
		t.Fatalf("fixture year = %d, want 1523", y)
	}

	t.Run("an ABSENT year preserves the stored year", func(t *testing.T) {
		if err := put(t, `{"time":{"month":2,"day":3}}`); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		if y, m, d := date(t); y != 1523 || m != 2 || d != 3 {
			t.Fatalf("after a year-less set the calendar reads %d/%d/%d, want 1523/2/3 — "+
				"absent must PRESERVE, never write 0", y, m, d)
		}
	})

	t.Run("a BLANK year is refused by name and moves nothing", func(t *testing.T) {
		err := put(t, `{"time":{"year":"","month":3,"day":9}}`)
		if err == nil {
			t.Fatal("a blank year was ACCEPTED — this is the browser measurement, " +
				"server-side: an empty field must never resolve to a number")
		}
		if !isAppErrorType(err, "validation_error") {
			t.Fatalf("a blank year produced %T (%v), want a validation_error naming the "+
				"field — a generic bad-request cannot tell the GM which box was empty",
				err, err)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "year") {
			t.Errorf("the refusal does not name the coordinate: %q", err.Error())
		}
		if y, m, d := date(t); y != 1523 || m != 2 || d != 3 {
			t.Fatalf("the refused write still moved the date to %d/%d/%d", y, m, d)
		}
	})

	t.Run("an EXPLICIT year 0 is still settable", func(t *testing.T) {
		if err := put(t, `{"time":{"year":0,"month":1,"day":1}}`); err != nil {
			t.Fatalf("an explicit year 0 was refused: %v — year 0 is a real year", err)
		}
		if y, m, d := date(t); y != 0 || m != 1 || d != 1 {
			t.Fatalf("after an explicit year:0 the calendar reads %d/%d/%d, want 0/1/1", y, m, d)
		}
	})

	t.Run("a legitimately-intended year is still settable", func(t *testing.T) {
		if err := put(t, `{"time":{"year":1524,"month":3,"day":9}}`); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		if y, m, d := date(t); y != 1524 || m != 3 || d != 9 {
			t.Fatalf("after Set date the calendar reads %d/%d/%d, want 1524/3/9", y, m, d)
		}
	})

	t.Run("a numeric STRING coordinate is honoured, not silently dropped", func(t *testing.T) {
		// An HTML input's value is a string. A client that forwards the raw
		// field rather than pre-parsing it must not be answered with a generic
		// bind failure — that is the shape that pushed both writers into
		// parseInt-with-a-fallback in the first place.
		if err := put(t, `{"time":{"year":"1525","month":"2","day":"4"}}`); err != nil {
			t.Fatalf("a numeric-string coordinate was refused: %v", err)
		}
		if y, m, d := date(t); y != 1525 || m != 2 || d != 4 {
			t.Fatalf("after a string-coordinate set the calendar reads %d/%d/%d, want 1525/2/4", y, m, d)
		}
	})

	t.Run("an UNPARSEABLE year is refused, not rounded to something", func(t *testing.T) {
		err := put(t, `{"time":{"year":"eleventy","month":1,"day":1}}`)
		if err == nil || !isAppErrorType(err, "validation_error") {
			t.Fatalf("an unparseable year produced %v, want a validation_error", err)
		}
		if y := mustYear(t, svc, cal.ID); y != 1525 {
			t.Fatalf("the refused write moved the year to %d", y)
		}
	})
}

func mustYear(t *testing.T, svc CalendarService, calID string) int {
	t.Helper()
	got, err := svc.GetCalendarByID(t.Context(), calID)
	if err != nil {
		t.Fatalf("re-reading the calendar: %v", err)
	}
	return got.CurrentYear
}

// ── 3. the two browser writers, in a real browser ──────────────────────────

var yearProbeRe = regexp.MustCompile(`(?s)<pre id="yp">(.*?)</pre>`)

// yearProbeArm is one recorded arm of the probe: what the writer SENT (the
// captured PUT bodies) and what it SAID.
type yearProbeArm struct {
	Arm  string   `json:"arm"`
	Sent []string `json:"sent"`
	Say  string   `json:"say"`
}

// yearProbeDriver is the arm script both writers share. It empties the year
// box, clicks Set, and records whether a PUT left the page — then repeats with
// an explicit 0 and with a real year, so the probe proves the refusal did not
// simply break the control.
const yearProbeDriver = `
var CAP = [];
window.Chronicle = window.Chronicle || {};
window.Chronicle.apiFetch = function (url, opts) {
  CAP.push(JSON.stringify((opts && opts.body) || null));
  // Reject so the writer's own catch re-enables the button; a resolved
  // response would send the v4 row into window.location.reload().
  return Promise.reject(new Error('captured by the probe'));
};
window.Chronicle.notify = function (msg) { window.__notified = String(msg || ''); };
function yp_out(o) {
  var pre = document.getElementById('yp');
  var all = JSON.parse(pre.textContent || '[]');
  all.push(o);
  pre.textContent = JSON.stringify(all);
}
function yp_arm(name, value) {
  var y = document.querySelector(YEAR_SEL);
  var b = document.querySelector(SET_SEL);
  window.__notified = '';
  y.value = value;
  b.disabled = false;
  CAP.length = 0;
  b.click();
  var sayEl = document.querySelector(SAY_SEL);
  var said = sayEl ? (sayEl.textContent || '') : '';
  yp_out({ arm: name, sent: CAP.slice(), say: said + ' ' + (window.__notified || '') });
}
setTimeout(function () {
  yp_arm('empty', '');
  yp_arm('zero', '0');
  yp_arm('year', '1524');
}, 120);
`

// runYearProbe loads one page in headless Chromium and returns its arms.
func runYearProbe(t *testing.T, chrome, page string) []yearProbeArm {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome,
		"--headless", "--no-sandbox", "--disable-gpu", "--hide-scrollbars",
		"--window-size=1200,900", "--virtual-time-budget=4000",
		"--dump-dom", "file://"+page,
	)
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("chromium dump-dom: %v", err)
	}
	m := yearProbeRe.FindSubmatch(raw)
	if m == nil {
		t.Fatalf(`no <pre id="yp"> in the dump:\n%s`, string(raw))
	}
	body := string(m[1])
	for _, sub := range [][2]string{{"&quot;", `"`}, {"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"}, {"&#39;", "'"}} {
		body = strings.ReplaceAll(body, sub[0], sub[1])
	}
	var arms []yearProbeArm
	if err := json.Unmarshal([]byte(body), &arms); err != nil {
		t.Fatalf("probe payload is not JSON: %v\n%s", err, body)
	}
	if len(arms) != 3 {
		t.Fatalf("expected 3 arms, got %d — the page stopped answering:\n%s", len(arms), body)
	}
	return arms
}

// assertYearProbe is the shared verdict for both writers.
func assertYearProbe(t *testing.T, writer string, arms []yearProbeArm) {
	t.Helper()
	byName := map[string]yearProbeArm{}
	for _, a := range arms {
		byName[a.Arm] = a
	}

	empty := byName["empty"]
	if len(empty.Sent) != 0 {
		t.Errorf("%s: an EMPTIED year field still sent a write: %v\n"+
			"This is the measured defect — the GM cleared the box and the world moved.",
			writer, empty.Sent)
	}
	if !strings.Contains(strings.ToLower(empty.Say), "year") {
		t.Errorf("%s: refusing the write said %q — it must name the empty field, "+
			"or the control just looks broken", writer, strings.TrimSpace(empty.Say))
	}

	zero := byName["zero"]
	if len(zero.Sent) != 1 || !strings.Contains(zero.Sent[0], `"year":0`) {
		t.Errorf("%s: an EXPLICIT year 0 did not send year:0 — got %v. Year 0 is a "+
			"real year; the fix distinguishes absent from zero, it does not ban zero",
			writer, zero.Sent)
	}

	year := byName["year"]
	if len(year.Sent) != 1 || !strings.Contains(year.Sent[0], `"year":1524`) {
		t.Errorf("%s: a legitimately-intended year did not reach the wire — got %v",
			writer, year.Sent)
	}
}

// TestYearProbe_BenchDateVerbRow drives the v4 writer (C-CALV4-GAMEREADY §2)
// over the real rendered Bench and the real shipped calendar_daycard.js.
func TestYearProbe_BenchDateVerbRow(t *testing.T) {
	chrome := findChromium()
	if chrome == "" {
		t.Skip("year probe (v4 Bench date-verb row): no Chromium binary found (set CHROMIUM_BIN) — " +
			"the emptied-year refusal in calendar_daycard.js was NOT measured in this run")
	}
	data := benchVerbFxData(t, true, true, true)
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/calendar_daycard.js")

	page := `<!doctype html><html><head><meta charset="utf-8"></head><body>` +
		`<pre id="yp">[]</pre>` +
		benchStripLinks(renderBench(t, data)) +
		`<script>` + mod + `</script>` +
		`<script>var YEAR_SEL='[data-bench-date-year]',SET_SEL='[data-bench-date-set]',` +
		`SAY_SEL='[data-bench-date-say]';` + yearProbeDriver + `</script>` +
		`</body></html>`

	file := filepath.Join(t.TempDir(), "year-probe-bench.html")
	if err := os.WriteFile(file, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	assertYearProbe(t, "v4 Bench date-verb row", runYearProbe(t, chrome, file))
}

// TestYearProbe_V2GMConsole drives the legacy writer over the real rendered
// GM console and the real shipped gm_panel.js. Both writers are measured
// because the sweep reproduced the defect in both, and a fix applied to one is
// a fix a GM can still walk around.
func TestYearProbe_V2GMConsole(t *testing.T) {
	chrome := findChromium()
	if chrome == "" {
		t.Skip("year probe (V2 GM console): no Chromium binary found (set CHROMIUM_BIN) — " +
			"the emptied-year refusal in gm_panel.js was NOT measured in this run")
	}
	cal := gmTestCalendar()
	var panel strings.Builder
	if err := gmControlPanelV2(CalendarV2ViewData{
		ActiveCalendar: cal, CanControlWorldState: true,
	}).Render(context.Background(), &panel); err != nil {
		t.Fatalf("render GM console: %v", err)
	}
	mod := readRepoFile(t, "internal/plugins/calendar/static/js/gm_panel.js")

	page := `<!doctype html><html><head><meta charset="utf-8"></head><body>` +
		`<pre id="yp">[]</pre>` +
		`<div data-cal-v2-root data-cal-v2-campaign-id="camp-1" ` +
		`data-cal-v2-calendar-id="cal-1" data-cal-v2-csrf-token="fx">` +
		panel.String() + `</div>` +
		`<script>var YEAR_SEL='[data-gm-date-year]',SET_SEL='[data-gm-set-date]',` +
		`SAY_SEL='#no-such-say';` + yearProbeDriver + `</script>` +
		`<script>` + mod + `</script>` +
		`</body></html>`

	file := filepath.Join(t.TempDir(), "year-probe-v2.html")
	if err := os.WriteFile(file, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	assertYearProbe(t, "V2 GM console", runYearProbe(t, chrome, file))
}

// ── 4. the source ratchet, which runs even with no browser ─────────────────

// TestDateWriters_NoZeroFallbackForACoordinate is the guard that runs on every
// machine, browser or not: neither writer may reintroduce a numeric fallback
// for a date coordinate read out of an input.
//
// It is a source assertion and it knows it — that is why the browser probes
// above exist too. But a machine with no Chromium would otherwise carry no
// coverage at all for the exact line that caused this, and "the probe skipped"
// is how this class of defect stays fixed for one release.
func TestDateWriters_NoZeroFallbackForACoordinate(t *testing.T) {
	for _, w := range []struct{ file, note string }{
		{"internal/plugins/calendar/static/js/gm_panel.js", "the legacy V2 console"},
		{"internal/plugins/calendar/static/js/calendar_daycard.js", "the v4 Bench date-verb row"},
	} {
		src := readRepoFile(t, w.file)
		for _, banned := range []string{
			"num('[data-gm-date-year]', 0)",
			"fieldInt('[data-bench-date-year]', 0)",
		} {
			if strings.Contains(src, banned) {
				t.Errorf("%s (%s) still reads the year with %s — an emptied box "+
					"becomes year 0 and the GM loses the campaign date with no error",
					w.file, w.note, banned)
			}
		}
		if !strings.Contains(src, "coordOrNull") {
			t.Errorf("%s (%s) does not use coordOrNull — the shared refusal is what "+
				"keeps an empty coordinate from being substituted", w.file, w.note)
		}
	}
}

// TestPutDateContract_YearIsRequired pins the wire decision in the type itself,
// so a future "simplify the struct" cannot quietly restore the hole.
func TestPutDateContract_YearIsRequired(t *testing.T) {
	var d apiDate
	if err := json.Unmarshal([]byte(`{"month":1,"day":1}`), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Year != nil {
		t.Fatalf("apiDate.Year decoded to %v from a body with no year — it must stay "+
			"nil so the handler can tell absent from zero", *d.Year)
	}
	if err := json.Unmarshal([]byte(`{"year":0,"month":1,"day":1}`), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Year == nil || *d.Year != 0 {
		t.Fatalf("apiDate.Year = %v from an explicit year:0, want a non-nil 0", d.Year)
	}
}

// TestWorldStateCoord_AbsentBlankZero pins the decode type directly. The three
// states this defect confused are three distinct results here.
func TestWorldStateCoord_AbsentBlankZero(t *testing.T) {
	for _, tc := range []struct {
		body             string
		wantSet, wantBad bool
		wantVal          int
	}{
		{`{}`, false, false, 0},
		{`{"year":null}`, false, false, 0},
		{`{"year":0}`, true, false, 0},
		{`{"year":-120}`, true, false, -120},
		{`{"year":1524}`, true, false, 1524},
		{`{"year":"1524"}`, true, false, 1524},
		{`{"year":""}`, false, true, 0},
		{`{"year":"   "}`, false, true, 0},
		{`{"year":"eleventy"}`, false, true, 0},
		{`{"year":true}`, false, true, 0},
	} {
		var got struct {
			Year worldStateCoord `json:"year"`
		}
		if err := json.Unmarshal([]byte(tc.body), &got); err != nil {
			t.Fatalf("%s: decode errored (%v) — a bad coordinate must be REPORTABLE, "+
				"not collapse the whole body into a generic bind failure", tc.body, err)
		}
		if got.Year.Set != tc.wantSet || got.Year.Blank != tc.wantBad || got.Year.Value != tc.wantVal {
			t.Errorf("%s → %+v, want set=%v blank=%v value=%d",
				tc.body, got.Year, tc.wantSet, tc.wantBad, tc.wantVal)
		}
	}
}

// TestYearProbe_BenchGMConsole drives the SAME writer over the REHOUSED console
// (C-CALV4-GM-CONSOLE): the real `benchGMConsoleView` markup, the real shipped
// `gm_panel.js`, and a real headless Chromium — with NO `[data-cal-v2-root]`
// anywhere on the page.
//
// IT IS NOT A THIRD COPY OF THE SAME MEASUREMENT. The two probes above pin the
// year refusal in two DIFFERENT writers; this one pins it in the same writer on
// a DIFFERENT MOUNT, and the mount is the thing that was nearly wrong. The
// driver used to read its endpoint, calendar and CSRF token off the V2 shell's
// page root and RETURN when it was absent — so on this page it would have
// rendered a console that never wrote at all. Every string assertion in Go and
// every stub-DOM assertion in test/js/ is written independently of the markup
// or independently of the driver; only this probe holds BOTH real ends at once,
// which is where a mistyped `data-gm-*` on either side would hide.
//
// Registered in tools/check-browser-probes.sh's PROBES_CALENDAR census, so a
// machine that HAS a browser is required to run it.
func TestYearProbe_BenchGMConsole(t *testing.T) {
	chrome := findChromium()
	if chrome == "" {
		t.Skip("year probe (Bench GM console): no Chromium binary found (set CHROMIUM_BIN) — " +
			"the rehoused console's write path was NOT measured in this run")
	}
	cal := gmTestCalendar()
	var console strings.Builder
	if err := benchGMConsoleView(benchGMConsole(cal, benchInput{
		Campaign:             &campaigns.Campaign{ID: "camp-1", Name: "Imix"},
		IsOwner:              true,
		CanControlWorldState: true,
		CanAuthorDmOnly:      true,
		CSRFToken:            "fx",
	})).Render(context.Background(), &console); err != nil {
		t.Fatalf("render Bench GM console: %v", err)
	}
	if !strings.Contains(console.String(), "data-gm-set-date") {
		t.Fatal("the fixture rendered no Set-date control — the probe would measure nothing")
	}

	page := `<!doctype html><html><head><meta charset="utf-8"></head><body>` +
		`<pre id="yp">[]</pre>` +
		// A .cal-bench wrapper and NOTHING ELSE. No [data-cal-v2-root]: that is
		// the whole point of the probe.
		`<div class="cal-bench">` + console.String() + `</div>` +
		`<script>var YEAR_SEL='[data-gm-date-year]',SET_SEL='[data-gm-set-date]',` +
		`SAY_SEL='#no-such-say';` + yearProbeDriver + `</script>` +
		`<script>` + readRepoFile(t, "internal/plugins/calendar/static/js/gm_panel.js") + `</script>` +
		`</body></html>`

	file := filepath.Join(t.TempDir(), "year-probe-bench-console.html")
	if err := os.WriteFile(file, []byte(page), 0o644); err != nil {
		t.Fatalf("write probe page: %v", err)
	}
	assertYearProbe(t, "Bench GM console", runYearProbe(t, chrome, file))
}
