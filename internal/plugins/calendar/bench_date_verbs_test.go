// bench_date_verbs_test.go — C-CALV4-GAMEREADY §2, [GR-SIGN-A] SIGNED and
// [GR-4].
//
// THE FINDING. Advancing the in-world date is the GM's most frequent action of
// a whole session, and v4 did not have it: the rendered Bench carried zero
// occurrences of "advance", "Set date" or "current date", and the five verbs
// plus Set date lived only in `calendar_v2_gmpanel.templ`, on a page
// `C-CALV4-V2SUNSET` [VS-1] (SIGNED BY THE OPERATOR) has committed to deleting.
// The Bench's only route to them was the Block head's "Open calendar →" link —
// into that same doomed page.
//
// WHAT THIS FILE PINS, IN THE ORDER THAT MATTERS:
//
//  1. THE NINE-ROW AUDIENCE MATRIX ([GR-4]). Owner / co-DM-grantee / Scribe /
//     Player / anonymous × {controls in the DOM, write accepted, write
//     rejected}. No non-holder SEES the verbs and no non-holder's write
//     SUCCEEDS — and the two halves are asserted separately, because a control
//     that is merely hidden is a permission model made of CSS.
//  2. TWO VERBS AND NO MORE. The three clock verbs, Set time, weather, the
//     dm_only trigger, clear, mood, reset-sky and pause are `C-CALV4-GM-
//     CONSOLE`'s eight remaining families and must not appear here.
//  3. THE TWO FLOORS DIFFER, DELIBERATELY. A co-DM steps and does not set.
//  4. THE MEASUREMENT THAT FORCED THE JS. A form-encoded PUT to the world-state
//     endpoint binds NOTHING, so the row cannot be a plain <form> or a bare
//     hx-put — it would answer 200 and change no date.
package calendar

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// benchVerbFxData renders a Bench for one audience row.
//
// The four inputs are the four independent facts the row's two floors read, and
// they are passed as themselves rather than derived from a single "role"
// number: CanControlWorldState is `cc.CanControlWorldState()` (Owner OR
// DM-grantee) and IsOwner is `cc.MemberRole >= RoleOwner`. Collapsing them
// would make the co-DM row untestable, and the co-DM row is the whole point of
// the asymmetry.
func benchVerbFxData(t *testing.T, isGM, isOwner, canControl bool) BenchData {
	t.Helper()
	data := benchFxData(isGM, isOwner)
	cals := benchFxAll()
	primary, _, _ := benchClassify(cals, "cal-harptos")
	if primary == nil {
		t.Fatal("fixture lost its primary calendar")
	}
	if data.Primary != nil {
		in := benchInput{
			Campaign:             &campaigns.Campaign{ID: "camp-1", Name: "Imix"},
			IsOwner:              isOwner,
			CanControlWorldState: canControl,
			CanAuthorDmOnly:      canControl,
			CSRFToken:            "fx-csrf",
		}
		data.Primary.Verbs = benchDateVerbs(primary, in)
		// THE CONSOLE IS FED FROM THE SAME benchInput AS THE VERB ROW
		// (C-CALV4-GM-CONSOLE), which is the whole point of this fixture: the
		// two surfaces now sit on ONE page and share a floor decision, so a row
		// of this matrix has to render both or it is measuring a page that does
		// not exist. benchFxData's own console is keyed to isOwner and cannot
		// express the co-DM row; this one can, and does.
		data.GMConsole = benchGMConsole(primary, in)
	}
	return data
}

// --- the matrix: what is in the DOM -----------------------------------------

const (
	benchVerbStepMarker = `data-bench-date-step`
	benchVerbSetMarker  = `data-bench-date-set`
	benchVerbRowMarker  = `data-bench-date-verbs`
)

func TestBenchDateVerbs_AudienceMatrixDOM(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		isGM, isOwner, canControl  bool
		wantRow, wantStep, wantSet bool
	}{
		// Owner: both floors.
		{"owner", true, true, true, true, true, true},
		// Co-DM grantee: CanControlWorldState WITHOUT Owner. Steps, does not
		// set — [GR-SIGN-A](b)'s named, deliberate asymmetry.
		{"co-DM grantee", true, false, true, true, true, false},
		// Scribe: GM visibility, no capability, not Owner. NOTHING.
		{"scribe", true, false, false, false, false, false},
		{"player", false, false, false, false, false, false},
		{"anonymous", false, false, false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := renderBench(t, benchVerbFxData(t, tc.isGM, tc.isOwner, tc.canControl))
			if got := strings.Contains(html, benchVerbRowMarker); got != tc.wantRow {
				t.Errorf("verb ROW present = %v, want %v — permission is ABSENCE, "+
					"so a viewer below both floors gets no element at all", got, tc.wantRow)
			}
			if got := strings.Contains(html, benchVerbStepMarker); got != tc.wantStep {
				t.Errorf("step verbs present = %v, want %v", got, tc.wantStep)
			}
			if got := strings.Contains(html, benchVerbSetMarker); got != tc.wantSet {
				t.Errorf("Set date present = %v, want %v", got, tc.wantSet)
			}
			// NEVER A DISABLED GHOST. Whatever this viewer does not hold must
			// be missing, not present-and-inert.
			if !tc.wantStep && strings.Contains(html, `disabled`) && strings.Contains(html, benchVerbStepMarker) {
				t.Error("a non-holder was rendered a disabled step verb; permission is absence")
			}
		})
	}
}

// TestBenchDateVerbs_ExactlyTwoStepVerbs pins [GR-4]'s count and its direction:
// `+1 day` and `−1 day`, and no third.
//
// `−1 day` IS NOT SYMMETRY FOR ITS OWN SAKE. It is the UNDO for a fat-finger
// `+1` on a surface where the write is immediate and unconfirmed; without it
// the only repair is the Owner-only settings page, which a co-DM cannot reach.
func TestBenchDateVerbs_ExactlyTwoStepVerbs(t *testing.T) {
	html := renderBench(t, benchVerbFxData(t, true, true, true))
	if got := strings.Count(html, benchVerbStepMarker+`=`); got != 2 {
		t.Fatalf("step verb count = %d, want exactly 2 (+1 day and −1 day)", got)
	}
	for _, want := range []string{`data-bench-date-step="1"`, `data-bench-date-step="-1"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
}

// TestBenchDateVerbs_TheOtherEightFamiliesAreAbsent is the boundary with
// `C-CALV4-GM-CONSOLE`, asserted rather than promised.
//
// Its [GC-3] triage marks the advance and set-date families ALREADY LANDED and
// keeps the remaining EIGHT. Each token below is one of them; a hit here means
// this slice quietly took scope it agreed not to, including the whole sensitive
// half whose audience [GC-2] exists to rule.
//
// ── RE-POINTED, NOT LOOSENED (C-CALV4-GM-CONSOLE) ───────────────────────────
//
// THE BOUNDARY MOVED BECAUSE THE CONSOLE ARRIVED. This assertion used to read
// the WHOLE Bench page, and it could, because the eight families had no home on
// it: `C-CALV4-GM-CONSOLE` was unbuilt and the only mount was the doomed V2
// shell. That slice has now landed and the eight families are on this page ON
// PURPOSE, in benchGMConsoleView. A page-wide denial would therefore now be
// asserting the opposite of the shipped design, and the two honest options were
// to delete it or to re-point it. It is re-pointed.
//
// WHAT IT MEASURES NOW IS THE SAME CLAIM, EXACTLY: §2's verb row is a NARROW
// SUBSET. The scope is the `[data-bench-date-verbs]` ELEMENT rather than the
// document, which is the element the claim was always about — the console
// carrying a weather catalog says nothing about whether the verb row grew one.
//
// AND IT IS STRICTLY STRONGER THAN THE OLD FORM, because the second half is
// new: each family is now asserted PRESENT in the console. The old test could
// have passed on a page that had lost the families altogether; this one cannot.
func TestBenchDateVerbs_TheOtherEightFamiliesAreAbsent(t *testing.T) {
	html := renderBench(t, benchVerbFxData(t, true, true, true))
	row := benchVerbRowHTML(t, html)
	for _, forbidden := range []string{
		"weather", "Weather", // weather today / weather for a day
		"moodTint", "mood-tint", "data-gm-mood", // mood tint
		"dm_only-trigger", "trigger-event", "data-gm-event-tile", // world/celestial triggers
		"reset-sky", "resetSky", "data-gm-reset", // reset sky
		"data-bench-date-hours", "data-bench-date-minutes", // the clock verbs
		"Rest 8h", "+10m", "+1h", "Set time", "data-gm-advance", // ditto, by label
	} {
		if strings.Contains(row, forbidden) {
			t.Errorf("%q reached the §2 verb ROW — that family is C-CALV4-GM-CONSOLE's, "+
				"and §2 is a NARROW SUBSET by written agreement", forbidden)
		}
	}
	// THE OTHER HALF OF THE BOUNDARY: the families are not absent from the
	// PRODUCT, they are somewhere else on the page. Without this, the test above
	// would go green on a Bench that had simply lost the console.
	// `data-gm-event-clear` (the per-event × chip) is deliberately NOT in this
	// list: those chips are built by the driver from the world-state seed, so
	// the server renders only the container they land in. Their commit is
	// covered in test/js/gm_panel_bench.test.mjs, where the DOM they live in
	// actually exists.
	for _, want := range []string{
		"data-gm-weather-tile", "data-gm-event-tile", "data-gm-mood",
		"data-gm-mood-clear", "data-gm-events-clear", "data-gm-active-events",
		"data-gm-reset", "data-gm-advance",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("%q is on neither surface — C-CALV4-GM-CONSOLE rehoused these onto "+
				"the Bench, so their absence is a lost control and not a clean boundary", want)
		}
	}
}

// benchVerbRowHTML returns the `[data-bench-date-verbs]` element's own markup.
//
// The row nests no <div> of its own — spans, buttons, labels and inputs only —
// so the first `</div>` after the marker closes it. The bounds are CHECKED
// rather than sliced blind: a bare strings.Index bound panics on a markup change
// instead of failing with a sentence (COMMON §3).
func benchVerbRowHTML(t *testing.T, html string) string {
	t.Helper()
	i := strings.Index(html, benchVerbRowMarker)
	if i < 0 {
		t.Fatalf("no %s element in the rendered Bench — the fixture lost the verb row, "+
			"and a scope test with nothing in scope proves nothing", benchVerbRowMarker)
	}
	open := strings.LastIndex(html[:i], "<div")
	if open < 0 {
		t.Fatalf("%s is not inside a <div> — the row's shape changed and this "+
			"extractor must change with it", benchVerbRowMarker)
	}
	end := strings.Index(html[open:], "</div>")
	if end < 0 {
		t.Fatalf("%s's element is unterminated", benchVerbRowMarker)
	}
	return html[open : open+end]
}

// --- the matrix: what the SERVER accepts ------------------------------------

// TestBenchDateVerbs_AudienceMatrixWrite is the other half of the nine rows,
// and it is the half that actually protects anything.
//
// It composes the EXACT middleware `routes.go:318-320` registers —
// `campaigns.RequireCapability((*campaigns.CampaignContext).CanControlWorldState, …)`
// — around a probe handler, so this is the shipped gate being exercised and not
// a re-statement of it. A control that is merely absent from a template is a
// permission model made of HTML; the server is where a permission lives.
func TestBenchDateVerbs_AudienceMatrixWrite(t *testing.T) {
	gate := campaigns.RequireCapability(
		(*campaigns.CampaignContext).CanControlWorldState,
		"world-state control requires Owner or co-DM access")

	reached := false
	handler := gate(func(c echo.Context) error {
		reached = true
		return c.NoContent(http.StatusOK)
	})

	for _, tc := range []struct {
		name       string
		role       campaigns.Role
		dmGranted  bool
		wantAccept bool
	}{
		{"owner", campaigns.RoleOwner, false, true},
		{"co-DM grantee (Player role + grant)", campaigns.RolePlayer, true, true},
		{"scribe", campaigns.RoleScribe, false, false},
		{"player", campaigns.RolePlayer, false, false},
		{"anonymous (no membership)", campaigns.RoleNone, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached = false
			e := echo.New()
			req := httptest.NewRequest(http.MethodPut, "/campaigns/camp-1/calendar/world-state",
				strings.NewReader(`{"advance":{"days":1}}`))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
			c := e.NewContext(req, httptest.NewRecorder())
			c.Set("campaign_context", &campaigns.CampaignContext{
				Campaign:    &campaigns.Campaign{ID: "camp-1"},
				MemberRole:  tc.role,
				IsDmGranted: tc.dmGranted,
			})

			err := handler(c)
			if tc.wantAccept {
				if err != nil || !reached {
					t.Fatalf("a capability holder's write was refused (err=%v, reached=%v)", err, reached)
				}
				return
			}
			if err == nil || reached {
				t.Fatalf("a NON-holder's write was accepted (err=%v, reached=%v) — "+
					"the row's absence from the DOM is not the protection", err, reached)
			}
		})
	}
}

// --- the write, against a real MariaDB --------------------------------------

// TestBenchDateVerbs_AdvanceIntegration is the persistence half of §2.
//
// EVERYTHING ABOVE IS ABOUT WHO MAY WRITE. This is about whether the write
// LANDS — and it is the half a fake cannot answer, because the verb's whole
// value is that the stored in-world date afterwards is the one the GM meant.
// The rollover case is included on purpose: the client sends only
// `{advance:{days:1}}`, so a day-30 → month-2-day-1 step is entirely the
// server's arithmetic, and if it were wrong the two step verbs would be a
// worse-than-nothing control.
func TestBenchDateVerbs_AdvanceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("date-verb integration test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	campaignID, cal := calTestSeedNavCalendar(t, db)
	svc := NewCalendarService(NewCalendarRepository(db))
	h := NewHandler(svc)

	// The fixture stands on day 14 of a 30-day month 1 of 3.
	put := func(t *testing.T, body string, role campaigns.Role, dmGranted bool) error {
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
			Campaign:    &campaigns.Campaign{ID: campaignID, Name: "Imix"},
			MemberRole:  role,
			IsDmGranted: dmGranted,
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

	if y, m, d := date(t); y != 1523 || m != 1 || d != 14 {
		t.Fatalf("fixture start = %d/%d/%d, want 1523/1/14", y, m, d)
	}

	t.Run("+1 day advances the STORED date", func(t *testing.T) {
		if err := put(t, `{"advance":{"days":1,"hours":0,"minutes":0}}`, campaigns.RoleOwner, false); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		if y, m, d := date(t); y != 1523 || m != 1 || d != 15 {
			t.Fatalf("after +1 day the calendar reads %d/%d/%d, want 1523/1/15", y, m, d)
		}
	})

	t.Run("−1 day is a real undo, back to where it started", func(t *testing.T) {
		if err := put(t, `{"advance":{"days":-1,"hours":0,"minutes":0}}`, campaigns.RoleOwner, false); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		if y, m, d := date(t); y != 1523 || m != 1 || d != 14 {
			t.Fatalf("after −1 day the calendar reads %d/%d/%d, want 1523/1/14", y, m, d)
		}
	})

	t.Run("the month rollover is the SERVER's, and it works", func(t *testing.T) {
		// Day 14 → day 30 is sixteen steps; the seventeenth is the rollover.
		for i := 0; i < 17; i++ {
			if err := put(t, `{"advance":{"days":1,"hours":0,"minutes":0}}`, campaigns.RoleOwner, false); err != nil {
				t.Fatalf("PutWorldState: %v", err)
			}
		}
		if y, m, d := date(t); y != 1523 || m != 2 || d != 1 {
			t.Fatalf("stepping past day 30 of month 1 reads %d/%d/%d, want 1523/2/1 — "+
				"the client sends only {advance:{days:1}}, so this arithmetic is entirely the server's",
				y, m, d)
		}
	})

	t.Run("Set date writes the absolute coordinates", func(t *testing.T) {
		if err := put(t, `{"time":{"year":1524,"month":3,"day":9}}`, campaigns.RoleOwner, false); err != nil {
			t.Fatalf("PutWorldState: %v", err)
		}
		if y, m, d := date(t); y != 1524 || m != 3 || d != 9 {
			t.Fatalf("after Set date the calendar reads %d/%d/%d, want 1524/3/9", y, m, d)
		}
	})
}

// --- the measurement that decided the row's mechanics -----------------------

// TestWorldStatePut_FormEncodedBindsNothing is why the verb row is driven by
// the plugin's registry-mounted JS instead of by a plain <form> or a bare
// `hx-put`, and it is recorded as a MEASUREMENT so nobody re-derives it.
//
// `putWorldStateBody`'s `advance` and `time` members are POINTERS TO ANONYMOUS
// STRUCTS carrying `json` tags and no `form` tags. Echo's form binder skips a
// tagless pointer-to-struct field, so a form-encoded submit binds NOTHING: the
// handler would answer 200 having changed no date, which at a table is worse
// than a missing control because the GM believes it worked.
//
// The fix is NOT to give the struct form tags. That endpoint is V2's console's
// and the sync path's, and this slice's Bounds freeze its wire contract.
func TestWorldStatePut_FormEncodedBindsNothing(t *testing.T) {
	e := echo.New()

	form := httptest.NewRequest(http.MethodPut, "/x",
		strings.NewReader("advance.days=1&advance[days]=1&days=1"))
	form.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	var got putWorldStateBody
	if err := e.NewContext(form, httptest.NewRecorder()).Bind(&got); err != nil {
		t.Fatalf("binding a form-encoded body errored rather than silently doing nothing: %v", err)
	}
	if got.Advance != nil {
		t.Fatal("a form-encoded advance DID bind — if this is now true the verb row " +
			"could be a plain <form>; re-read benchDateVerbRow's header before changing it")
	}

	// The JSON shape the row actually sends binds, which is the positive
	// control: this test must fail if the payload key ever drifts.
	js := httptest.NewRequest(http.MethodPut, "/x", strings.NewReader(`{"advance":{"days":-1}}`))
	js.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	var ok putWorldStateBody
	if err := e.NewContext(js, httptest.NewRecorder()).Bind(&ok); err != nil {
		t.Fatalf("binding the row's own JSON payload: %v", err)
	}
	if ok.Advance == nil || ok.Advance.Days != -1 {
		t.Fatalf("the row's JSON payload did not bind: %+v", ok.Advance)
	}
}
