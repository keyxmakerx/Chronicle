package quests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeQuestCalendar is a two-month calendar: Frost has 30 days (31 in a leap
// year, every 4 years from 0) and Bloom has 20. Today is Frost 10, year 100.
type fakeQuestCalendar struct{}

func (fakeQuestCalendar) ID() string   { return "cal-1" }
func (fakeQuestCalendar) Name() string { return "Harvest Reckoning" }
func (fakeQuestCalendar) Today() DueDay {
	return DueDay{Year: 100, Month: 1, Day: 10}
}
func (fakeQuestCalendar) Months() []CalendarMonth {
	return []CalendarMonth{{Name: "Frost", Days: 30, LeapDays: 1}, {Name: "Bloom", Days: 20}}
}
func (fakeQuestCalendar) Leap() (int, int) { return 4, 0 }

func (c fakeQuestCalendar) monthDays(m, y int) int {
	days := c.Months()[m-1]
	if y%4 == 0 {
		return days.Days + days.LeapDays
	}
	return days.Days
}

func (c fakeQuestCalendar) Valid(d DueDay) bool {
	return d.Month >= 1 && d.Month <= 2 && d.Day >= 1 && d.Day <= c.monthDays(d.Month, d.Year)
}

func (fakeQuestCalendar) Label(d DueDay) string {
	return fmt.Sprintf("%s %d, %d", []string{"", "Frost", "Bloom"}[d.Month], d.Day, d.Year)
}

// DaysFromToday is exact within year 100 (a leap year, so Frost has 31 days).
func (c fakeQuestCalendar) DaysFromToday(d DueDay) int {
	abs := func(x DueDay) int {
		n := x.Year * 51
		if x.Month == 2 {
			n += c.monthDays(1, x.Year)
		}
		return n + x.Day
	}
	return abs(d) - abs(c.Today())
}

// fakeCalendarDir records every event call. It stores events by id so tests
// can see what the calendar would hold.
type fakeCalendarDir struct {
	none      bool // no calendar
	saveErr   error
	events    map[string]DueEvent
	nextID    int
	saves     int
	deletes   []string
	visSets   int
	gone      map[string]bool // ids the calendar no longer has
	calendars int
}

func newFakeCalendarDir() *fakeCalendarDir {
	return &fakeCalendarDir{events: map[string]DueEvent{}, gone: map[string]bool{}}
}

func (f *fakeCalendarDir) Calendar(context.Context, string) (QuestCalendar, error) {
	f.calendars++
	if f.none {
		return nil, nil
	}
	return fakeQuestCalendar{}, nil
}

func (f *fakeCalendarDir) SaveDueEvent(_ context.Context, _ string, _ QuestCalendar, id string, ev DueEvent) (string, error) {
	if f.saveErr != nil {
		return "", f.saveErr
	}
	f.saves++
	if id == "" || f.gone[id] {
		f.nextID++
		id = fmt.Sprintf("ev-%d", f.nextID)
	}
	f.events[id] = ev
	return id, nil
}

func (f *fakeCalendarDir) SetDueEventVisibility(_ context.Context, _ string, _ QuestCalendar, id string, dmOnly bool) error {
	f.visSets++
	if ev, ok := f.events[id]; ok {
		ev.DMOnly = dmOnly
		f.events[id] = ev
	}
	return nil
}

func (f *fakeCalendarDir) DeleteDueEvent(_ context.Context, _ string, _ QuestCalendar, id string) error {
	f.deletes = append(f.deletes, id)
	delete(f.events, id)
	return nil
}

func newDueEnv() (questEnv, *fakeCalendarDir) {
	e := newQuestEnv()
	cal := newFakeCalendarDir()
	e.svc = NewQuestService(e.repo, e.ents, e.maps, cal)
	return e, cal
}

func putDM(t *testing.T, e questEnv, id string, body string) (*DMQuestView, error) {
	t.Helper()
	return e.svc.Put(context.Background(), camp, id, dm, parsePatch(t, body))
}

func TestDueDateValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		none bool
		want int
	}{
		{"valid", `{"year":100,"month":1,"day":10}`, false, 0},
		{"leap day in a leap year", `{"year":104,"month":1,"day":31}`, false, 0},
		{"leap day in a common year", `{"year":103,"month":1,"day":31}`, false, 422},
		{"month zero", `{"year":100,"month":0,"day":1}`, false, 422},
		{"month past the last", `{"year":100,"month":3,"day":1}`, false, 422},
		{"day zero", `{"year":100,"month":1,"day":0}`, false, 422},
		{"day past the month", `{"year":100,"month":2,"day":21}`, false, 422},
		{"negative day", `{"year":100,"month":1,"day":-4}`, false, 422},
		{"year far too large", `{"year":2000000,"month":1,"day":1}`, false, 422},
		{"year far too small", `{"year":-2000000,"month":1,"day":1}`, false, 422},
		{"no calendar", `{"year":100,"month":1,"day":1}`, true, 422},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, cal := newDueEnv()
			cal.none = tc.none
			_, err := putDM(t, e, qid, `{"version":0,"dueDate":`+tc.body+`}`)
			if got := code(err); got != tc.want {
				t.Fatalf("code = %d (%v), want %d", got, err, tc.want)
			}
			if tc.want != 0 {
				if cal.saves != 0 || len(e.repo.docs) != 0 {
					t.Fatalf("a refused date must change nothing: saves=%d docs=%d", cal.saves, len(e.repo.docs))
				}
			}
		})
	}
}

func TestDueDateSetClearPreserve(t *testing.T) {
	e, cal := newDueEnv()

	v, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":2,"day":3}}`)
	if err != nil {
		t.Fatal(err)
	}
	if v.Due == nil || v.Due.Label != "Bloom 3, 100" {
		t.Fatalf("due after set: %+v", v.Due)
	}
	if len(cal.events) != 1 || cal.saves != 1 {
		t.Fatalf("one event expected: %+v", cal.events)
	}
	id := "ev-1"
	if ev := cal.events[id]; ev.Day != (DueDay{100, 2, 3}) || ev.Title != "Due: Missing Goat" || ev.EntityID != qid || ev.CreatedBy != "dm" {
		t.Fatalf("event wrong: %+v", ev)
	}

	// Another field in the patch leaves the date and the calendar alone.
	v, err = putDM(t, e, qid, `{"version":1,"status":"active"}`)
	if err != nil {
		t.Fatal(err)
	}
	if v.Due == nil || v.Due.Day != 3 || cal.saves != 1 || len(cal.deletes) != 0 {
		t.Fatalf("an unrelated patch touched the due date: %+v saves=%d deletes=%v", v.Due, cal.saves, cal.deletes)
	}

	// Re-sending the date updates the same event rather than adding one.
	if _, err = putDM(t, e, qid, `{"version":2,"dueDate":{"year":100,"month":2,"day":5}}`); err != nil {
		t.Fatal(err)
	}
	if len(cal.events) != 1 || cal.events[id].Day.Day != 5 {
		t.Fatalf("event not updated in place: %+v", cal.events)
	}

	// An explicit null clears the date and deletes the event.
	v, err = putDM(t, e, qid, `{"version":3,"dueDate":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if v.Due != nil || len(cal.events) != 0 || len(cal.deletes) != 1 || cal.deletes[0] != id {
		t.Fatalf("clear failed: due=%+v events=%v deletes=%v", v.Due, cal.events, cal.deletes)
	}
	var stored Quest
	if err := json.Unmarshal(e.repo.docs[qid], &stored); err != nil {
		t.Fatal(err)
	}
	if stored.DueDate != nil || stored.DueEventID != "" {
		t.Fatalf("stored doc still has a due date: %+v", stored)
	}

	// Clearing when nothing is set touches no event.
	if _, err = putDM(t, e, qid, `{"version":4,"dueDate":null}`); err != nil || len(cal.deletes) != 1 {
		t.Fatalf("clearing an unset date: err=%v deletes=%v", err, cal.deletes)
	}
}

func TestDueDaysLeft(t *testing.T) {
	tests := []struct {
		name string
		date string
		want int
	}{
		{"ahead", `{"year":100,"month":1,"day":20}`, 10},
		{"across the month", `{"year":100,"month":2,"day":1}`, 22},
		{"today", `{"year":100,"month":1,"day":10}`, 0},
		{"late", `{"year":100,"month":1,"day":7}`, -3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := newDueEnv()
			v, err := putDM(t, e, qid, `{"version":0,"dueDate":`+tc.date+`}`)
			if err != nil {
				t.Fatal(err)
			}
			if v.Due.DaysLeft != tc.want {
				t.Fatalf("put daysLeft = %d, want %d", v.Due.DaysLeft, tc.want)
			}
			got, _ := e.svc.Get(context.Background(), camp, qid, dm)
			if d := got.(*DMQuestView).Due.DaysLeft; d != tc.want {
				t.Fatalf("get daysLeft = %d, want %d", d, tc.want)
			}
		})
	}
}

func TestDueWireShape(t *testing.T) {
	e, cal := newDueEnv()
	ctx := context.Background()

	// No due date yet: "due" is an explicit null, "dueCalendar" is filled in.
	if _, err := putDM(t, e, qid, `{"version":0,"status":"active"}`); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, camp, qid, dm)
	raw, _ := json.Marshal(got)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if v, ok := m["due"]; !ok || v != nil {
		t.Fatalf("due should be null: %s", raw)
	}
	c, _ := m["dueCalendar"].(map[string]any)
	if c == nil || c["name"] != "Harvest Reckoning" || c["leapEvery"] != float64(4) || c["leapOffset"] != float64(0) {
		t.Fatalf("calendar wrong: %s", raw)
	}
	today := c["today"].(map[string]any)
	if today["label"] != "Frost 10, 100" || today["year"] != float64(100) {
		t.Fatalf("today wrong: %v", today)
	}
	months := c["months"].([]any)
	if first := months[0].(map[string]any); len(months) != 2 || first["name"] != "Frost" || first["days"] != float64(30) || first["leapDays"] != float64(1) {
		t.Fatalf("months wrong: %v", months)
	}

	// Without a calendar the DM view says calendar:null.
	cal.none = true
	got, _ = e.svc.Get(ctx, camp, qid, dm)
	raw, _ = json.Marshal(got)
	m = map[string]any{}
	_ = json.Unmarshal(raw, &m)
	if v, ok := m["dueCalendar"]; !ok || v != nil {
		t.Fatalf("calendar should be null: %s", raw)
	}

	// Set a date, then read the due object.
	cal.none = false
	if _, err := putDM(t, e, qid, `{"version":1,"dueDate":{"year":100,"month":1,"day":7}}`); err != nil {
		t.Fatal(err)
	}
	got, _ = e.svc.Get(ctx, camp, qid, dm)
	raw, _ = json.Marshal(got)
	m = map[string]any{}
	_ = json.Unmarshal(raw, &m)
	due := m["due"].(map[string]any)
	if due["label"] != "Frost 7, 100" || due["daysLeft"] != float64(-3) || due["year"] != float64(100) || due["month"] != float64(1) || due["day"] != float64(7) {
		t.Fatalf("due wrong: %v", due)
	}
	// The date is not listed in the DM view as anything but "due".
	if strings.Contains(string(raw), "dueEventId") {
		t.Fatalf("the event id must not reach the wire: %s", raw)
	}
}

func TestPlayerViewGetsDueButNotCalendar(t *testing.T) {
	e, _ := newDueEnv()
	ctx := context.Background()
	if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":12}}`); err != nil {
		t.Fatal(err)
	}
	for _, who := range []Viewer{scribe, player, guest} {
		got, err := e.svc.Get(ctx, camp, qid, who)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got)
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		due, _ := m["due"].(map[string]any)
		if due == nil || due["daysLeft"] != float64(2) || due["label"] != "Frost 12, 100" {
			t.Fatalf("player due wrong: %s", raw)
		}
		if _, has := m["dueCalendar"]; has || strings.Contains(string(raw), "Harvest") || strings.Contains(string(raw), "dueEventId") {
			t.Fatalf("player view leaks the calendar: %s", raw)
		}
	}

	// A player cannot set one.
	if _, err := e.svc.Put(ctx, camp, qid, player, parsePatch(t, `{"version":1,"dueDate":null}`)); code(err) != 403 {
		t.Fatalf("player put: %v", err)
	}

	// The due date hides with the notice.
	if _, err := putDM(t, e, qid, `{"version":1,"layout":{"notice":{"x":1,"y":2,"w":30,"r":0,"hidden":true}}}`); err != nil {
		t.Fatal(err)
	}
	got, _ := e.svc.Get(ctx, camp, qid, player)
	if got.(*PlayerQuestView).Due != nil {
		t.Fatal("a hidden notice must not show its due date")
	}
}

func TestDueEventVisibilityAndTitle(t *testing.T) {
	tests := []struct {
		name   string
		id     string
		dmOnly bool
	}{
		{"page players can open", qid, false},
		{"page hidden from players", "secret-quest", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, cal := newDueEnv()
			if _, err := putDM(t, e, tc.id, `{"version":0,"notice":{"title":"Find the Goat"},"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
				t.Fatal(err)
			}
			ev := cal.events["ev-1"]
			if ev.DMOnly != tc.dmOnly || ev.Title != "Due: Find the Goat" || ev.EntityID != tc.id {
				t.Fatalf("event wrong: %+v", ev)
			}
		})
	}

	t.Run("title falls back to the page name", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		if got := cal.events["ev-1"].Title; got != "Due: Missing Goat" {
			t.Fatalf("title = %q", got)
		}
	})

	t.Run("a title change renames the event", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"notice":{"title":"Old"},"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		if _, err := putDM(t, e, qid, `{"version":1,"notice":{"title":"New name"}}`); err != nil {
			t.Fatal(err)
		}
		if len(cal.events) != 1 || cal.events["ev-1"].Title != "Due: New name" || cal.events["ev-1"].Day.Day != 20 {
			t.Fatalf("event not renamed in place: %+v", cal.events)
		}
		// A save that leaves the title alone does not write the event.
		saves := cal.saves
		if _, err := putDM(t, e, qid, `{"version":2,"notice":{"title":"New name"},"status":"done"}`); err != nil {
			t.Fatal(err)
		}
		if cal.saves != saves {
			t.Fatal("an unchanged title rewrote the event")
		}
	})

	t.Run("a title change with no due date touches no calendar", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"notice":{"title":"A"}}`); err != nil {
			t.Fatal(err)
		}
		if _, err := putDM(t, e, qid, `{"version":1,"notice":{"title":"B"}}`); err != nil {
			t.Fatal(err)
		}
		if cal.saves != 0 {
			t.Fatal("event written without a due date")
		}
	})

	t.Run("an event the calendar lost is created again", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		cal.gone["ev-1"] = true
		if _, err := putDM(t, e, qid, `{"version":1,"dueDate":{"year":100,"month":1,"day":21}}`); err != nil {
			t.Fatal(err)
		}
		var stored Quest
		_ = json.Unmarshal(e.repo.docs[qid], &stored)
		if stored.DueEventID != "ev-2" {
			t.Fatalf("stored event id = %q", stored.DueEventID)
		}
	})
}

func TestDueEventFailureOrdering(t *testing.T) {
	t.Run("a failing event write refuses the save and changes nothing", func(t *testing.T) {
		e, cal := newDueEnv()
		cal.saveErr = errors.New("calendar down")
		_, err := putDM(t, e, qid, `{"version":0,"status":"active","dueDate":{"year":100,"month":1,"day":20}}`)
		if code(err) != 500 {
			t.Fatalf("code = %d (%v)", code(err), err)
		}
		if len(e.repo.docs) != 0 {
			t.Fatal("the sheet was written although the event was not")
		}
	})

	t.Run("a lost version race removes the event the save created", func(t *testing.T) {
		e, cal := newDueEnv()
		// Two writers load version 0; the second one's save loses.
		racing := &racingRepo{fakeQuestRepo: e.repo}
		e.svc = NewQuestService(racing, e.ents, e.maps, cal)
		_, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`)
		if code(err) != 409 {
			t.Fatalf("code = %d (%v)", code(err), err)
		}
		if len(cal.events) != 0 || len(cal.deletes) != 1 {
			t.Fatalf("stray event left: %v deletes=%v", cal.events, cal.deletes)
		}
	})

	t.Run("no calendar at all still saves a rename", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"notice":{"title":"A"},"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		cal.none = true
		if _, err := putDM(t, e, qid, `{"version":1,"notice":{"title":"B"}}`); err != nil {
			t.Fatalf("a rename must not need the calendar: %v", err)
		}
	})
}

// racingRepo loses every save of a first version, as if another writer got in
// between the read and the write.
type racingRepo struct{ *fakeQuestRepo }

func (r *racingRepo) Save(context.Context, string, string, []byte, int, string) (bool, error) {
	return false, nil
}

func TestSyncDueEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("flips visibility both ways", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		if cal.events["ev-1"].DMOnly {
			t.Fatal("starts visible")
		}
		e.ents.hidden[qid] = true
		if err := e.svc.SyncDueEvent(ctx, camp, qid); err != nil || !cal.events["ev-1"].DMOnly {
			t.Fatalf("hiding the page must hide the event: err=%v ev=%+v", err, cal.events["ev-1"])
		}
		e.ents.hidden[qid] = false
		if err := e.svc.SyncDueEvent(ctx, camp, qid); err != nil || cal.events["ev-1"].DMOnly {
			t.Fatalf("showing the page must show the event: err=%v ev=%+v", err, cal.events["ev-1"])
		}
	})

	for _, withSheet := range []bool{false, true} {
		name := "no-op for a page without a quest sheet"
		if withSheet {
			name = "no-op for a quest without a due date"
		}
		t.Run(name, func(t *testing.T) {
			e, cal := newDueEnv()
			if withSheet {
				if _, err := putDM(t, e, qid, `{"version":0,"status":"active"}`); err != nil {
					t.Fatal(err)
				}
			}
			cal.calendars = 0
			if err := e.svc.SyncDueEvent(ctx, camp, qid); err != nil {
				t.Fatal(err)
			}
			if cal.calendars != 0 || cal.visSets != 0 {
				t.Fatalf("the calendar was touched: reads=%d sets=%d", cal.calendars, cal.visSets)
			}
		})
	}

	t.Run("never reads another campaign's sheet", func(t *testing.T) {
		e, cal := newDueEnv()
		if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
			t.Fatal(err)
		}
		if err := e.svc.SyncDueEvent(ctx, other, qid); err != nil || cal.visSets != 0 {
			t.Fatalf("cross-campaign sync acted: err=%v sets=%d", err, cal.visSets)
		}
	})
}

func TestRemoveDueEvent(t *testing.T) {
	ctx := context.Background()
	e, cal := newDueEnv()
	if err := e.svc.RemoveDueEvent(ctx, camp, qid); err != nil || len(cal.deletes) != 0 {
		t.Fatalf("no sheet, nothing to delete: %v %v", err, cal.deletes)
	}
	if _, err := putDM(t, e, qid, `{"version":0,"dueDate":{"year":100,"month":1,"day":20}}`); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveDueEvent(ctx, camp, qid); err != nil || len(cal.events) != 0 || len(cal.deletes) != 1 {
		t.Fatalf("event not removed: %v %v", err, cal.events)
	}
}

func TestBoardNoticeDaysLeft(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		body string // quest 'q1' patch; "" means no sheet
		want *int
	}{
		{"no sheet", ``, nil},
		{"no due date", `{"version":0,"status":"active"}`, nil},
		{"ahead", `{"version":0,"dueDate":{"year":100,"month":1,"day":14}}`, ptr(4)},
		{"today", `{"version":0,"dueDate":{"year":100,"month":1,"day":10}}`, ptr(0)},
		{"late", `{"version":0,"dueDate":{"year":100,"month":1,"day":3}}`, ptr(-7)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cal := newFakeCalendarDir()
			e := newBoardEnv()
			qs := NewQuestService(e.quest, e.ents, e.maps, cal)
			if tc.body != "" {
				if _, err := qs.Put(ctx, camp, "q1", dm, parsePatch(t, tc.body)); err != nil {
					t.Fatal(err)
				}
			}
			e.svc = NewBoardService(e.repo, e.quest, e.ents, fakeTypes{camp: {catType: true}}, e.maps, fakeNames{"dm": "Dana"}, cal)
			b := e.board(t, WhoDM)
			if _, err := e.svc.CreateItem(ctx, camp, pageHome, b, dm, ItemInput{Kind: KindNotice, RefID: "q1"}); err != nil {
				t.Fatal(err)
			}
			for _, who := range []Viewer{dm, player} {
				view, err := e.svc.View(ctx, camp, pageHome, who)
				if err != nil {
					t.Fatal(err)
				}
				var it *ItemView
				for i := range view.Boards[0].Items {
					it = &view.Boards[0].Items[i]
				}
				if it == nil {
					t.Fatal("no notice on the board")
				}
				if (it.DaysLeft == nil) != (tc.want == nil) || (tc.want != nil && *it.DaysLeft != *tc.want) {
					t.Fatalf("daysLeft = %v, want %v", deref(it.DaysLeft), deref(tc.want))
				}
				raw, _ := json.Marshal(it)
				has := strings.Contains(string(raw), `"daysLeft"`)
				if has != (tc.want != nil) {
					t.Fatalf("daysLeft key presence = %v in %s", has, raw)
				}
			}
		})
	}

	t.Run("no calendar leaves the days off", func(t *testing.T) {
		cal := newFakeCalendarDir()
		e := newBoardEnv()
		if _, err := NewQuestService(e.quest, e.ents, e.maps, cal).Put(ctx, camp, "q1", dm, parsePatch(t, `{"version":0,"dueDate":{"year":100,"month":1,"day":14}}`)); err != nil {
			t.Fatal(err)
		}
		cal.none = true
		e.svc = NewBoardService(e.repo, e.quest, e.ents, fakeTypes{camp: {catType: true}}, e.maps, fakeNames{"dm": "Dana"}, cal)
		b := e.board(t, WhoDM)
		it, err := e.svc.CreateItem(ctx, camp, pageHome, b, dm, ItemInput{Kind: KindNotice, RefID: "q1"})
		if err != nil || it.DaysLeft != nil {
			t.Fatalf("err=%v item=%+v", err, it)
		}
	})
}

func ptr(n int) *int { return &n }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
