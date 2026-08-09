// partial_update_test.go — sweep R4, the sessions half of the
// absent-means-preserve contract.
//
// Reproduced before the fix: the "Mark Complete" button sends {name,status}
// only and UpdateSession assigned every field unguarded, so one click erased
// the schedule, the summary, the in-world date and the entire recurrence
// config — and because the next-occurrence generator reads the STORED
// IsRecurring, it stopped firing at the same moment, silently. The Edit
// modal had the same hole on calendar_year/month/day and
// recurrence_day_of_week, which it has no inputs for and therefore never
// sends.
//
// The three directions of the contract are pinned here on the endpoint, and
// on the primitive in internal/patch/patch_test.go:
//
//	absent preserves · present replaces · explicit null clears
package sessions

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// storedSession is the fully-populated row every case below starts from: a
// scheduled, summarised, in-world-dated, weekly-recurring planned session.
func storedSession() *Session {
	summary := "the party reached the vault"
	date := "2026-08-15"
	clock := "19:00"
	year, month, day := 1492, 3, 11
	dow := 6
	end := "2026-12-31"
	weekly := RecurrenceWeekly
	return &Session{
		ID:                  "sess-1",
		CampaignID:          "camp-1",
		Name:                "Session 12",
		Summary:             &summary,
		ScheduledDate:       &date,
		ScheduledTime:       &clock,
		CalendarYear:        &year,
		CalendarMonth:       &month,
		CalendarDay:         &day,
		Status:              StatusPlanned,
		IsRecurring:         true,
		RecurrenceType:      &weekly,
		RecurrenceInterval:  1,
		RecurrenceDayOfWeek: &dow,
		RecurrenceEndDate:   &end,
		CreatedBy:           "user-1",
	}
}

// runUpdateBody drives the REAL wire binder and the REAL service with the
// literal JSON body a client sends, and returns the row that reached the
// repository plus the auto-generated next occurrence (nil if none).
func runUpdateBody(t *testing.T, body string) (written *Session, next *Session, err error) {
	t.Helper()
	var req updateSessionRequest
	if decErr := json.Unmarshal([]byte(body), &req); decErr != nil {
		t.Fatalf("decode %s: %v", body, decErr)
	}
	repo := &mockSessionRepo{
		findByIDFn: func(_ context.Context, _ string) (*Session, error) { return storedSession(), nil },
		updateFn:   func(_ context.Context, s *Session) error { written = s; return nil },
		createFn: func(_ context.Context, _ string, s *Session) error {
			next = s
			return nil
		},
		listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) { return nil, nil },
	}
	_, err = newTestSessionService(repo).UpdateSession(context.Background(), "sess-1", req.toInput())
	return written, next, err
}

// THE headline regression: the exact body the Mark Complete button sends
// must complete the session, change NOTHING else, and still generate the
// next occurrence.
func TestMarkComplete_PreservesEverythingAndStillGeneratesNextOccurrence(t *testing.T) {
	written, next, err := runUpdateBody(t, `{"status":"completed"}`)
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if written == nil {
		t.Fatal("nothing was written")
	}
	want := storedSession()

	if written.Status != StatusCompleted {
		t.Errorf("Status = %q, want completed — the one field the button means", written.Status)
	}
	if written.Name != want.Name {
		t.Errorf("Name = %q, want %q preserved", written.Name, want.Name)
	}
	assertStrPtr(t, "Summary", written.Summary, want.Summary)
	assertStrPtr(t, "ScheduledDate", written.ScheduledDate, want.ScheduledDate)
	assertStrPtr(t, "ScheduledTime", written.ScheduledTime, want.ScheduledTime)
	assertIntPtr(t, "CalendarYear", written.CalendarYear, want.CalendarYear)
	assertIntPtr(t, "CalendarMonth", written.CalendarMonth, want.CalendarMonth)
	assertIntPtr(t, "CalendarDay", written.CalendarDay, want.CalendarDay)
	assertIntPtr(t, "RecurrenceDayOfWeek", written.RecurrenceDayOfWeek, want.RecurrenceDayOfWeek)
	assertStrPtr(t, "RecurrenceEndDate", written.RecurrenceEndDate, want.RecurrenceEndDate)
	assertStrPtr(t, "RecurrenceType", written.RecurrenceType, want.RecurrenceType)
	if !written.IsRecurring {
		t.Error("IsRecurring = false, want true preserved — this is the field whose loss silently killed the generator")
	}
	if written.RecurrenceInterval != want.RecurrenceInterval {
		t.Errorf("RecurrenceInterval = %d, want %d", written.RecurrenceInterval, want.RecurrenceInterval)
	}

	if next == nil {
		t.Fatal("no next occurrence was generated — Mark Complete must keep the recurring series running")
	}
	if next.ScheduledDate == nil || *next.ScheduledDate != "2026-08-22" {
		t.Errorf("next occurrence date = %v, want 2026-08-22 (weekly +7 from the PRESERVED schedule)", next.ScheduledDate)
	}
	if next.Status != StatusPlanned {
		t.Errorf("next occurrence status = %q, want planned", next.Status)
	}
}

// The Edit modal's real body: it has no inputs for the in-world date or the
// day-of-week, so it never sends those keys — and absent must preserve them.
func TestEditModalBody_PreservesInWorldDateAndDayOfWeek(t *testing.T) {
	body := `{"name":"Session 12 (renamed)","summary":"new notes","scheduled_date":"2026-09-01",` +
		`"scheduled_time":"20:00","status":"planned","is_recurring":true,` +
		`"recurrence_type":"weekly","recurrence_interval":1,"recurrence_end_date":null}`
	written, _, err := runUpdateBody(t, body)
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	want := storedSession()
	assertIntPtr(t, "CalendarYear", written.CalendarYear, want.CalendarYear)
	assertIntPtr(t, "CalendarMonth", written.CalendarMonth, want.CalendarMonth)
	assertIntPtr(t, "CalendarDay", written.CalendarDay, want.CalendarDay)
	assertIntPtr(t, "RecurrenceDayOfWeek", written.RecurrenceDayOfWeek, want.RecurrenceDayOfWeek)
	// …and the keys it DOES send still take effect, including the explicit
	// null the user produced by emptying the end-date input.
	if written.Name != "Session 12 (renamed)" {
		t.Errorf("Name = %q, want the edited value", written.Name)
	}
	if written.RecurrenceEndDate != nil {
		t.Errorf("RecurrenceEndDate = %q, want nil — an explicit null must CLEAR", *written.RecurrenceEndDate)
	}
}

// The contract, all three directions, on one nullable field.
func TestUpdateSession_AbsentPreserves_PresentReplaces_NullClears(t *testing.T) {
	cases := []struct {
		name string
		body string
		want *string // nil means "must end up NULL"
	}{
		{"absent preserves", `{"status":"planned"}`, strp("the party reached the vault")},
		{"present replaces", `{"summary":"a new summary"}`, strp("a new summary")},
		{"explicit null clears", `{"summary":null}`, nil},
		{"present empty string is a value, not an absence", `{"summary":""}`, strp("")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			written, _, err := runUpdateBody(t, tc.body)
			if err != nil {
				t.Fatalf("UpdateSession: %v", err)
			}
			assertStrPtr(t, "Summary", written.Summary, tc.want)
		})
	}
}

// An absent name is "I am not editing the name"; a PRESENT empty name is
// still the validation error it always was.
func TestUpdateSession_NameRequiredOnlyWhenSent(t *testing.T) {
	if _, _, err := runUpdateBody(t, `{"status":"completed"}`); err != nil {
		t.Errorf("absent name must be allowed: %v", err)
	}
	if _, _, err := runUpdateBody(t, `{"name":""}`); err == nil {
		t.Error("a present empty name must still be rejected")
	}
}

// --- client-side pins: what the two templ surfaces actually send ---

// markCompleteBodyKeys extracts the key set of the Mark Complete button's
// request body straight out of sessions.templ, so a client that starts
// sending more than it means reddens here rather than in production.
func TestMarkCompleteClient_SendsOnlyStatus(t *testing.T) {
	src, err := os.ReadFile("sessions.templ")
	if err != nil {
		t.Fatalf("read sessions.templ: %v", err)
	}
	keys := bodyKeysNear(t, string(src), "status:'completed'")
	if !reflect.DeepEqual(keys, []string{"status"}) {
		t.Errorf("Mark Complete body keys = %v, want [status]. The button means one thing; sending more re-arms the whole-replace hazard for anything it gets wrong.", keys)
	}
}

// The Edit modal must NOT send the in-world date or the day-of-week: it has
// no inputs for them, so anything it sent would be invented.
func TestEditModalClient_NeverSendsFieldsItHasNoInputsFor(t *testing.T) {
	src, err := os.ReadFile("sessions.templ")
	if err != nil {
		t.Fatalf("read sessions.templ: %v", err)
	}
	for _, forbidden := range []string{"calendar_year", "calendar_month", "calendar_day", "recurrence_day_of_week"} {
		if strings.Contains(string(src), forbidden+":") {
			t.Errorf("sessions.templ sends %q, but the Edit modal has no input for it — it can only send a wrong value", forbidden)
		}
	}
}

// bodyKeysNear finds the `body:{…}` object literal containing needle and
// returns its sorted top-level keys.
func bodyKeysNear(t *testing.T, src, needle string) []string {
	t.Helper()
	idx := strings.Index(src, needle)
	if idx < 0 {
		t.Fatalf("could not find %q in the templ source", needle)
	}
	start := strings.LastIndex(src[:idx], "body:{")
	if start < 0 {
		t.Fatalf("no body:{ before %q", needle)
	}
	rest := src[start+len("body:{"):]
	end := strings.Index(rest, "}")
	if end < 0 {
		t.Fatal("unterminated body literal")
	}
	var keys []string
	for _, m := range regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)\s*:`).FindAllStringSubmatch(rest[:end], -1) {
		keys = append(keys, m[1])
	}
	sort.Strings(keys)
	return keys
}

func strp(s string) *string { return &s }

func assertStrPtr(t *testing.T, field string, got, want *string) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %q, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %q", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %q, want %q", field, *got, *want)
	}
}

func assertIntPtr(t *testing.T, field string, got, want *int) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s = %d, want nil", field, *got)
	case want != nil && got == nil:
		t.Errorf("%s = nil, want %d", field, *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("%s = %d, want %d", field, *got, *want)
	}
}
