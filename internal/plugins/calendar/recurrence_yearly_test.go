// recurrence_yearly_test.go — C-CALV4-GAMEREADY §6, [GR-11] and [GR-12].
//
// THE FINDING THIS GUARDS, IN TWO HALVES THAT HAD TO BE FIXED TOGETHER.
//
//	THE ENGINE GAP. `OccursOn` expanded weekly / biweekly / monthly / custom and
//	sent everything else to `default: return onBase`. A festival, a holy day, a
//	birthday, a coronation anniversary — the single most common recurring thing
//	in a fantasy calendar — could not repeat. And it was a REGRESSION: the
//	pre-v4 calendar was yearly-ONLY.
//
//	THE SILENT 201. `CreateEventAPI` and `UpdateEventAPI` bound
//	`recurrence_type` and validated nothing, so "yearly", "daily", "hourly",
//	"WEEKLY" and "🐉" were all stored with a 201 and then fired exactly once.
//	Fixing only the engine would have left "daily" and "WEEKLY" still accepted
//	and still silent, which is why [GR-12] lands in the same commit and why the
//	accepted set is stated exactly once (model.go's Recurrence* block, read
//	through IsSupportedRecurrenceType by both handlers).
//
// WHY THERE IS A REAL DATABASE IN HERE. The recurrence engine has been proven
// against fakes for its whole life, and that is exactly how "every N months"
// survived: a predicate can be right in isolation while the row that reaches it
// is wrong. TestRecurrenceYearly_Integration writes a yearly event through the
// real repository and reads it back through the real projection, and
// TestRecurrenceType_UnknownIsRejected drives the real handlers so the 400 is
// proven at the wire and the 201 is proven to have actually persisted the type.
package calendar

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// --- 1. yearly expands, and a missing base day is SKIPPED --------------------

// TestRecurrence_YearlyFires is [GR-11]'s predicate guard.
//
// THE FOUR CLAIMS THE RULING NAMES, plus the one it rules hardest:
// it fires next year AND the year after; it does NOT fire next month or next
// day; MaxOccurrences counts years; and a base day absent from a later year is
// SKIPPED, never clamped.
func TestRecurrence_YearlyFires(t *testing.T) {
	cal := recurrenceCal() // 12 months, month 2 = 28 days + 1 leap day, leap every 4

	// Base: year 1520 (a leap year), month 6, day 15 — an ordinary 30-day month
	// so the ordinary cases are not entangled with the leap geometry.
	base := recurEvent(RecurrenceYearly, 1520, 6, 15)

	capped := recurEvent(RecurrenceYearly, 1520, 6, 15)
	capped.RecurrenceMaxOccurrences = ptr(3)

	ended := recurEvent(RecurrenceYearly, 1520, 6, 15)
	ended.RecurrenceEndYear, ended.RecurrenceEndMonth, ended.RecurrenceEndDay = ptr(1522), ptr(6), ptr(15)

	every4 := recurEvent(RecurrenceYearly, 1520, 6, 15)
	every4.RecurrenceInterval = ptr(4)

	for _, tc := range []struct {
		name    string
		ev      Event
		y, m, d int
		want    bool
	}{
		{"the base date itself", base, 1520, 6, 15, true},
		{"NEXT YEAR — the whole point of the section", base, 1521, 6, 15, true},
		{"and the year after that", base, 1522, 6, 15, true},
		{"and a decade later", base, 1530, 6, 15, true},
		{"NOT next month", base, 1520, 7, 15, false},
		{"NOT the same month next year but a different day", base, 1521, 6, 16, false},
		{"NOT next day", base, 1520, 6, 16, false},
		{"NOT a different month in a later year", base, 1523, 7, 15, false},
		{"NOT before the base year", base, 1519, 6, 15, false},

		// MaxOccurrences counts YEARS: 3 occurrences = 1520, 1521, 1522.
		{"cap: occurrence 1 (the base year)", capped, 1520, 6, 15, true},
		{"cap: occurrence 3 (the last allowed)", capped, 1522, 6, 15, true},
		{"cap: occurrence 4 is past the cap", capped, 1523, 6, 15, false},

		// The shared recurrence-end date applies to yearly like every other type.
		{"end date: on the end date", ended, 1522, 6, 15, true},
		{"end date: past it", ended, 1523, 6, 15, false},

		// The interval is applied, so a stored "every 4 years" is the rule the
		// author gets rather than an annual one wearing its label.
		{"every 4 years: the base", every4, 1520, 6, 15, true},
		{"every 4 years: +1 does NOT fire", every4, 1521, 6, 15, false},
		{"every 4 years: +4 fires", every4, 1524, 6, 15, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ev.OccursOn(cal, tc.y, tc.m, tc.d); got != tc.want {
				t.Errorf("OccursOn(%d-%d-%d) = %v, want %v", tc.y, tc.m, tc.d, got, tc.want)
			}
		})
	}
}

// TestRecurrence_YearlySkipsAMissingBaseDayRatherThanClamping is the ruling's
// hardest half, and it is asserted POSITIVELY rather than by the absence of a
// crash.
//
// THE SETUP IS THE ASSERTION. Month 2 of recurrenceCal carries 28 base days and
// one leap day, with leap years every 4 — so a festival authored on 2/29 in
// 1520 has no 29th to land on in 1521, 1522 or 1523. The test first PROVES the
// day is genuinely absent from those years (via MonthDays, the same geometry
// the branch consults) and only then asserts that the event does not occur.
//
// AND IT PROVES THE SKIP IS A SKIP. A clamp would have put the holy day on
// 2/28, or on 3/1, in every non-leap year — silently, on a day the GM did not
// author. Both of those are asserted false, because "does not occur on the
// 29th" alone is equally true of an implementation that moved it.
func TestRecurrence_YearlySkipsAMissingBaseDayRatherThanClamping(t *testing.T) {
	cal := recurrenceCal()
	ev := recurEvent(RecurrenceYearly, 1520, 2, 29) // 1520 % 4 == 0 → leap

	if got := cal.MonthDays(1, 1520); got != 29 {
		t.Fatalf("fixture broken: month 2 of the leap year 1520 has %d days, want 29", got)
	}
	if got := cal.MonthDays(1, 1521); got != 28 {
		t.Fatalf("fixture broken: month 2 of the common year 1521 has %d days, want 28", got)
	}
	if !ev.OccursOn(cal, 1520, 2, 29) {
		t.Fatal("the base occurrence must fire in its own leap year")
	}
	if !ev.OccursOn(cal, 1524, 2, 29) {
		t.Error("the next LEAP year has a 29th and the festival must fire on it")
	}

	for _, y := range []int{1521, 1522, 1523} {
		if ev.OccursOn(cal, y, 2, 29) {
			t.Errorf("year %d has no 2/29 yet the event claimed to occur on it", y)
		}
		if ev.OccursOn(cal, y, 2, 28) {
			t.Errorf("year %d: the occurrence was CLAMPED BACK to 2/28 — the ruling is SKIP, never clamp", y)
		}
		if ev.OccursOn(cal, y, 3, 1) {
			t.Errorf("year %d: the occurrence was CLAMPED FORWARD to 3/1 — the ruling is SKIP, never clamp", y)
		}
	}
}

// TestRecurrence_UnsupportedTypeStillExpandsOnce pins the OTHER side of the
// switch, so widening the accepted set stays a deliberate act. An unknown or
// legacy string keeps its pre-slice behaviour — one occurrence at its stored
// date — because existing rows carrying a bad type must not start firing.
// [GR-12] refuses a NEW one at the door; this refuses to reinterpret an OLD one.
func TestRecurrence_UnsupportedTypeStillExpandsOnce(t *testing.T) {
	cal := recurrenceCal()
	for _, rt := range []string{"", "daily", "hourly", "WEEKLY", "🐉"} {
		ev := recurEvent(rt, 1520, 6, 15)
		if !ev.OccursOn(cal, 1520, 6, 15) {
			t.Errorf("%q: the stored date must still occur", rt)
		}
		if ev.OccursOn(cal, 1521, 6, 15) {
			t.Errorf("%q: expanded into a later year — an unsupported type is ONE occurrence", rt)
		}
	}
}

// TestIsSupportedRecurrenceType pins the shared vocabulary predicate directly,
// including the empty string, which is ACCEPTED and means "not recurring" —
// calendar_daycard.js's `once` branch writes exactly that on purpose.
func TestIsSupportedRecurrenceType(t *testing.T) {
	for _, ok := range []string{"", "weekly", "biweekly", "monthly", "custom", "yearly"} {
		if !IsSupportedRecurrenceType(ok) {
			t.Errorf("%q must be accepted", ok)
		}
	}
	for _, bad := range []string{"daily", "hourly", "WEEKLY", "Monthly", "🐉", " weekly", "weekly "} {
		if IsSupportedRecurrenceType(bad) {
			t.Errorf("%q must be rejected — the comparison is exact and case-sensitive", bad)
		}
	}
	if len(RecurrenceTypes) != 5 {
		t.Errorf("the accepted set has %d members, want 5 — widening it is a wire-contract change",
			len(RecurrenceTypes))
	}
}

// --- 2. the same claim, against a real MariaDB -------------------------------

// TestRecurrenceYearly_Integration proves yearly recurrence end to end: a row
// written by the real repository, read back by the real block projection, on a
// real database.
//
// WHY A FAKE IS NOT ENOUGH HERE. `OccursOn` is a pure predicate and a fake can
// prove it in isolation — which is exactly how "every N months" survived for
// months. What a fake cannot see is the ROW: `recurrence_type` is a
// VARCHAR(20) that the repository has to write and read back, `is_recurring` is
// a TINYINT the projection's candidate query filters on, and the projection
// only asks OccursOn about events it decided to load. A yearly festival that
// expands perfectly in memory and never reaches the month it belongs to is the
// bug this test can fail on and a fake cannot.
func TestRecurrenceYearly_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("yearly-recurrence integration test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID, cal := calTestSeedYearlyCalendar(t, db)

	spine := NewBlockService(NewBlockRepository(db))
	viewer := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	// monthIdx is 0-based on the wire the Bench uses; month 2 is index 1.
	for _, tc := range []struct {
		name        string
		year, month int
		want        []string
		absent      []string
	}{
		{"the base year carries both festivals", 1520, 2,
			[]string{"Midwinter Crowning", "The Leap Vigil"}, nil},
		{"NEXT YEAR still carries the annual festival", 1521, 2,
			[]string{"Midwinter Crowning"}, []string{"The Leap Vigil"}},
		{"and the year after", 1522, 2,
			[]string{"Midwinter Crowning"}, []string{"The Leap Vigil"}},
		{"the next LEAP year carries the leap-day vigil again", 1524, 2,
			[]string{"Midwinter Crowning", "The Leap Vigil"}, nil},
		{"a yearly event does NOT bleed into the next month", 1521, 3,
			nil, []string{"Midwinter Crowning", "The Leap Vigil"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := spine.Block(ctx, BlockRequest{
				CalendarID: cal.ID, CampaignID: campaignID, Viewer: viewer,
				View: BlockDate{Year: tc.year, Month: tc.month},
			})
			if err != nil {
				t.Fatalf("Block(%d/%d): %v", tc.year, tc.month, err)
			}
			marks := benchNavFxMarkNames(d)
			for _, want := range tc.want {
				if !marks[want] {
					t.Errorf("%d/%d did not carry %q — the row never reached the month it recurs into",
						tc.year, tc.month, want)
				}
			}
			for _, gone := range tc.absent {
				if marks[gone] {
					t.Errorf("%d/%d carried %q, which must not occur there", tc.year, tc.month, gone)
				}
			}
		})
	}

	// THE SKIP, PROVEN ON THE DATABASE AND PROVEN TO BE A SKIP. 1521 has no
	// 2/29, so the vigil is absent from the whole month — and specifically
	// absent from the two days a clamp would have moved it to.
	t.Run("the leap-day vigil is SKIPPED, not clamped, in a common year", func(t *testing.T) {
		d, err := spine.Block(ctx, BlockRequest{
			CalendarID: cal.ID, CampaignID: campaignID, Viewer: viewer,
			View: BlockDate{Year: 1521, Month: 2},
		})
		if err != nil {
			t.Fatalf("Block(1521/2): %v", err)
		}
		for _, row := range d.Month.Rows {
			for _, cell := range row.Cells {
				for _, m := range cell.Marks {
					if m.Title == "The Leap Vigil" {
						t.Errorf("The Leap Vigil surfaced on day %d of a year with no 29th — clamped, not skipped",
							cell.Day)
					}
				}
			}
		}
	})
}

// calTestSeedYearlyCalendar builds the §6 fixture: a calendar whose second
// month is 28 days plus one leap day (leap every 4), one ordinary yearly
// festival, and one authored ON the leap day so the skip has something to skip.
func calTestSeedYearlyCalendar(t *testing.T, db *sql.DB) (string, *Calendar) {
	t.Helper()
	ctx := context.Background()
	campaignID := calTestSeedCampaign(t, db)
	repo := NewCalendarRepository(db)

	cal := &Calendar{
		ID:         calTestID(t),
		CampaignID: campaignID, Name: "Harptos of Imix", Mode: ModeFantasy,
		IsDefault: true, CurrentYear: 1520, CurrentMonth: 2, CurrentDay: 1,
		LeapYearEvery: 4,
		HoursPerDay:   24, MinutesPerHour: 60, SecondsPerMinute: 60,
	}
	if err := repo.Create(ctx, cal); err != nil {
		t.Fatalf("create calendar: %v", err)
	}
	if err := repo.SetMonths(ctx, cal.ID, []MonthInput{
		{Name: "Deepwinter", Days: 30},
		{Name: "Thawrun", Days: 28, LeapYearDays: 1},
		{Name: "Sunfall", Days: 30},
	}); err != nil {
		t.Fatalf("set months: %v", err)
	}
	if err := repo.SetWeekdays(ctx, cal.ID, []WeekdayInput{
		{Name: "Sar"}, {Name: "Mol"}, {Name: "Zor"}, {Name: "Wir"},
		{Name: "Nym"}, {Name: "Lyr"}, {Name: "Tam"},
	}); err != nil {
		t.Fatalf("set weekdays: %v", err)
	}

	yearly := RecurrenceYearly
	for _, e := range []*Event{
		{CalendarID: cal.ID, Name: "Midwinter Crowning", Year: 1520, Month: 2, Day: 12,
			IsRecurring: true, RecurrenceType: &yearly, Visibility: storageVisibilityEveryone},
		{CalendarID: cal.ID, Name: "The Leap Vigil", Year: 1520, Month: 2, Day: 29,
			IsRecurring: true, RecurrenceType: &yearly, Visibility: storageVisibilityEveryone},
	} {
		e.ID = calTestID(t)
		if err := repo.CreateEvent(ctx, e); err != nil {
			t.Fatalf("create event %q: %v", e.Name, err)
		}
	}

	// The type must have survived the round trip through VARCHAR(20) before any
	// projection assertion below means anything.
	stored, err := repo.ListAllEvents(ctx, cal.ID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	for _, e := range stored {
		if e.RecurrenceType == nil || *e.RecurrenceType != RecurrenceYearly {
			t.Fatalf("event %q came back with recurrence_type %v, want %q",
				e.Name, e.RecurrenceType, RecurrenceYearly)
		}
	}
	return campaignID, cal
}

// --- 3. the silent 201, closed at both handlers ------------------------------

// TestRecurrenceType_UnknownIsRejected is [GR-12].
//
// THE TABLE IS THE RULING. The accepted set is {"", weekly, biweekly, monthly,
// custom, yearly} → 201; everything else → 400, in BOTH handlers, EXACT and
// CASE-SENSITIVE. The emoji stays in the table deliberately: it is the audit's
// own probe input and it documents that this is about an unvalidated string
// rather than a list of plausible typos. "  weekly" is there for the same
// reason from the other end — the near miss, not the absurd one.
//
// IT RUNS ON A REAL DATABASE because a 201 that does not persist the type is
// the same defect wearing a success code: the accepted rows are read back and
// their stored `recurrence_type` compared.
func TestRecurrenceType_UnknownIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("recurrence-type validation test requires a database; skipped under -short")
	}
	db := newCalendarScratchSchema(t)
	ctx := context.Background()
	campaignID, cal := calTestSeedYearlyCalendar(t, db)
	h := NewHandler(NewCalendarService(NewCalendarRepository(db)))

	accepted := []string{"", "weekly", "biweekly", "monthly", "custom", "yearly"}
	rejected := []string{"daily", "hourly", "WEEKLY", "🐉", "  weekly"}

	t.Run("CreateEventAPI", func(t *testing.T) {
		for _, rt := range accepted {
			t.Run("201 "+recurLabel(rt), func(t *testing.T) {
				rec, err := calTestCreateEvent(t, h, campaignID, cal.ID, rt)
				if err != nil {
					t.Fatalf("create with recurrence_type %q: %v", rt, err)
				}
				if rec.Code != http.StatusCreated {
					t.Fatalf("status = %d, want 201", rec.Code)
				}
				var got Event
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decoding the created event: %v", err)
				}
				stored, err := h.svc.GetEvent(ctx, got.ID)
				if err != nil || stored == nil {
					t.Fatalf("re-reading the created event: %v", err)
				}
				// A 201 that stored something else is the same silent lie in a
				// success code, so the row is read back rather than the response.
				if rt != "" && (stored.RecurrenceType == nil || *stored.RecurrenceType != rt) {
					t.Errorf("stored recurrence_type = %v, want %q", stored.RecurrenceType, rt)
				}
			})
		}
		for _, rt := range rejected {
			t.Run("400 "+recurLabel(rt), func(t *testing.T) {
				rec, err := calTestCreateEvent(t, h, campaignID, cal.ID, rt)
				calTestWant400(t, rt, rec, err)
				if calTestCountEventsNamed(t, db, "Probe "+rt) != 0 {
					t.Errorf("recurrence_type %q was rejected AND persisted — the 400 must be a refusal, not a warning", rt)
				}
			})
		}
	})

	t.Run("UpdateEventAPI", func(t *testing.T) {
		// One event to edit, created clean so every case starts from the same row.
		rec, err := calTestCreateEvent(t, h, campaignID, cal.ID, "weekly")
		if err != nil {
			t.Fatalf("seeding the event to update: %v", err)
		}
		var seed Event
		if err := json.Unmarshal(rec.Body.Bytes(), &seed); err != nil {
			t.Fatalf("decoding the seed event: %v", err)
		}

		for _, rt := range accepted {
			t.Run("accepted "+recurLabel(rt), func(t *testing.T) {
				if _, err := calTestUpdateEvent(t, h, campaignID, seed.ID, `"`+rt+`"`); err != nil {
					t.Fatalf("update with recurrence_type %q: %v", rt, err)
				}
				stored, err := h.svc.GetEvent(ctx, seed.ID)
				if err != nil || stored == nil {
					t.Fatalf("re-reading the updated event: %v", err)
				}
				if rt != "" && (stored.RecurrenceType == nil || *stored.RecurrenceType != rt) {
					t.Errorf("stored recurrence_type = %v, want %q", stored.RecurrenceType, rt)
				}
			})
		}
		for _, rt := range rejected {
			t.Run("400 "+recurLabel(rt), func(t *testing.T) {
				r, err := calTestUpdateEvent(t, h, campaignID, seed.ID, `"`+rt+`"`)
				calTestWant400(t, rt, r, err)
			})
		}

		// THE PARTIAL-UPDATE CONTRACT IS NOT COLLATERAL DAMAGE. RecurrenceType
		// is a patch.Field: an ABSENT key preserves and an explicit null clears.
		// Neither is a value, so neither may be rejected — a guard that failed
		// an absent key would 400 every partial PUT that does not mention
		// recurrence, which is most of them.
		t.Run("an ABSENT recurrence_type is not a value and is not rejected", func(t *testing.T) {
			body := `{"name":"Renamed by a narrow PUT"}`
			if _, err := calTestUpdateEventRaw(t, h, campaignID, seed.ID, body); err != nil {
				t.Fatalf("a PUT that never mentions recurrence_type must not 400: %v", err)
			}
		})
		t.Run("an EXPLICIT null recurrence_type is not a value and is not rejected", func(t *testing.T) {
			if _, err := calTestUpdateEvent(t, h, campaignID, seed.ID, `null`); err != nil {
				t.Fatalf("an explicit null must clear rather than 400: %v", err)
			}
		})
	})
}

// recurLabel names the empty string and the whitespace case readably in subtest
// output, where a bare "" or "  weekly" is unreadable.
func recurLabel(rt string) string {
	switch rt {
	case "":
		return "(empty: not recurring)"
	case "  weekly":
		return "(leading whitespace)"
	}
	return rt
}

// calTestWant400 asserts the handler refused with a 400 apperror rather than
// any other failure — a 404 or a 500 would pass a naive "it errored" check
// while proving nothing about the validation.
func calTestWant400(t *testing.T, rt string, rec *httptest.ResponseRecorder, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("recurrence_type %q was ACCEPTED (status %d) — an unsupported type must be a 400, not a 201",
			rt, rec.Code)
	}
	var app *apperror.AppError
	if !errors.As(err, &app) {
		t.Fatalf("recurrence_type %q failed with %T (%v), want an *apperror.AppError", rt, err, err)
	}
	if app.Code != http.StatusBadRequest {
		t.Fatalf("recurrence_type %q → status %d, want 400", rt, app.Code)
	}
	if !strings.Contains(app.Message, rt) && rt != "" {
		t.Errorf("the 400 for %q does not name the offending value (%q) — an integration debugging this needs it",
			rt, app.Message)
	}
}

func calTestCreateEvent(t *testing.T, h *Handler, campaignID, calID, rtype string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name": "Probe " + rtype, "year": 1520, "month": 1, "day": 3,
		"is_recurring": rtype != "", "recurrence_type": rtype,
	})
	if err != nil {
		t.Fatalf("marshalling the probe body: %v", err)
	}
	rec, c := calTestEventCtx(t, http.MethodPost,
		"/campaigns/"+campaignID+"/calendars/"+calID+"/events", string(body), campaignID)
	c.SetParamNames("id", "calId")
	c.SetParamValues(campaignID, calID)
	return rec, h.CreateEventAPI(c)
}

func calTestUpdateEvent(t *testing.T, h *Handler, campaignID, eventID, rawType string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	return calTestUpdateEventRaw(t, h, campaignID, eventID,
		`{"name":"Probe update","recurrence_type":`+rawType+`}`)
}

func calTestUpdateEventRaw(t *testing.T, h *Handler, campaignID, eventID, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	rec, c := calTestEventCtx(t, http.MethodPut,
		"/campaigns/"+campaignID+"/calendar/events/"+eventID, body, campaignID)
	c.SetParamNames("id", "eid")
	c.SetParamValues(campaignID, eventID)
	return rec, h.UpdateEventAPI(c)
}

func calTestEventCtx(t *testing.T, method, target, body, campaignID string) (*httptest.ResponseRecorder, echo.Context) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set("campaign_context", &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: campaignID, Name: "Imix"},
		MemberRole: campaigns.RoleOwner,
	})
	c.Set("auth_user_id", "u-gm")
	return rec, c
}

// calTestCountEventsNamed counts rows by name with a BINARY comparison.
//
// THE COLLATION MATTERS HERE AND IT COST A FALSE FAILURE. The schema is
// utf8mb4_unicode_ci, so a plain `name = 'Probe WEEKLY'` also matches the
// "Probe weekly" row the ACCEPTED half of this table created moments earlier —
// and the rejection assertion then reported a persisted row that the handler
// had correctly refused. The case-sensitivity this test exists to prove is
// exactly the case-sensitivity the default collation does not have.
func calTestCountEventsNamed(t *testing.T, db *sql.DB, name string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM calendar_events WHERE name = ? COLLATE utf8mb4_bin`, name).Scan(&n); err != nil {
		t.Fatalf("counting events named %q: %v", name, err)
	}
	return n
}
