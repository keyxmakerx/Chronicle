package calendar

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeMarkRepo is an in-memory MoonMarkRepository over one calendar.
type fakeMarkRepo struct {
	mode  map[int]string
	names map[int]map[string]string
	marks map[int]map[DayDate]MoonMark
}

func newFakeMarkRepo() *fakeMarkRepo {
	return &fakeMarkRepo{mode: map[int]string{}, names: map[int]map[string]string{}, marks: map[int]map[DayDate]MoonMark{}}
}

func (f *fakeMarkRepo) MarksFor(context.Context, string) (map[int][]MoonMark, error) {
	out := map[int][]MoonMark{}
	for id, ms := range f.marks {
		for _, k := range ms {
			out[id] = append(out[id], k)
		}
	}
	return out, nil
}
func (f *fakeMarkRepo) CountMarks(_ context.Context, id int) (int, error) {
	return len(f.marks[id]), nil
}
func (f *fakeMarkRepo) SetMode(_ context.Context, _ string, id int, mode string, names map[string]string) (bool, error) {
	if id != 7 {
		return false, nil
	}
	f.mode[id], f.names[id] = mode, names
	return true, nil
}
func (f *fakeMarkRepo) PutMark(_ context.Context, id int, k MoonMark) error {
	if f.marks[id] == nil {
		f.marks[id] = map[DayDate]MoonMark{}
	}
	f.marks[id][k.date()] = k
	return nil
}
func (f *fakeMarkRepo) DeleteMark(_ context.Context, id int, d DayDate) (bool, error) {
	_, ok := f.marks[id][d]
	delete(f.marks[id], d)
	return ok, nil
}

// markFixture is a two-month calendar (30 and 20 days, today 5/2/10) with
// one moon, Umbra (id 7), whose mode the fake repo holds.
func markFixture(t *testing.T, realTime bool) (*calendarService, *fakeMarkRepo) {
	t.Helper()
	marks := newFakeMarkRepo()
	mode := ModeFantasy
	if realTime {
		mode = ModeRealLife
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", Name: "Reckoning", Mode: mode,
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60,
				CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
		getMoonsFn: func(context.Context, string) ([]Moon, error) {
			m := Moon{ID: 7, CalendarID: "cal", Name: "Umbra", CycleDays: 30, PhaseMode: marks.mode[7], StateNames: marks.names[7]}
			if m.PhaseMode == "" {
				m.PhaseMode = MoonCycle
			}
			return []Moon{m}, nil
		},
	}
	svc := newTestCalendarService(calRepo, &fakeEventRepo{}, nil, nil).(*calendarService)
	svc.SetMoonMarkRepository(marks)
	return svc, marks
}

func manualUmbra(marks ...MoonMark) *Calendar {
	cal := &Calendar{Months: []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}},
		Moons: []Moon{{ID: 7, Name: "Umbra", PhaseMode: MoonManual, Marks: marks}}}
	cal.indexMoonMarks()
	return cal
}

func TestManualMoonHoldsItsLatestMark(t *testing.T) {
	cal := manualUmbra(
		MoonMark{Year: 5, Month: 1, Day: 9, State: MoonVanished},
		MoonMark{Year: 5, Month: 1, Day: 3, State: MoonFull},
		MoonMark{Year: 5, Month: 2, Day: 40, State: MoonHalf}, // no such day: dropped
	)
	m := &cal.Moons[0]
	m.StateNames = map[string]string{MoonVanished: "Gone below"}
	abs := func(mo, d int) int { return cal.absDayIndex(5, mo, d) }
	tests := []struct {
		name      string
		day       int
		wantState string
		wantWord  string
		wantPhase float64
	}{
		{"before the first mark", abs(1, 2), "", "Not set", 0},
		{"on the full mark", abs(1, 3), MoonFull, "Full", 0.5},
		{"held after it", abs(1, 8), MoonFull, "Full", 0.5},
		{"vanished, in the Director's word", abs(1, 9), MoonVanished, "Gone below", 0},
		{"still vanished next month", abs(2, 15), MoonVanished, "Gone below", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.StateOn(tt.day); got != tt.wantState {
				t.Errorf("state %q, want %q", got, tt.wantState)
			}
			if got := m.MoonPhaseName(tt.day); got != tt.wantWord {
				t.Errorf("word %q, want %q", got, tt.wantWord)
			}
			if got := m.MoonPhase(tt.day); got != tt.wantPhase {
				t.Errorf("phase %v, want %v", got, tt.wantPhase)
			}
		})
	}
	if len(m.Marks) != 2 {
		t.Errorf("a mark on a missing day should be dropped, kept %d", len(m.Marks))
	}
}

// A rule on Umbra's full moon falls on the day it was set full, not on every
// day it stays full.
func TestMoonRuleFallsOnTheMarkDay(t *testing.T) {
	cal := manualUmbra(MoonMark{Year: 5, Month: 1, Day: 3, State: MoonFull}, MoonMark{Year: 5, Month: 1, Day: 20, State: MoonDark})
	m := &cal.Moons[0]
	for day, want := range map[int]bool{2: false, 3: true, 4: false} {
		if got := moonPhaseFalls(m, cal.absDayIndex(5, 1, day), 0.5); got != want {
			t.Errorf("full on day %d = %v, want %v", day, got, want)
		}
	}
	if !moonPhaseFalls(m, cal.absDayIndex(5, 1, 20), 0) {
		t.Error("a new-moon rule should fall on the day Umbra went dark")
	}
}

func TestPlayersKeepOnlyShownMarks(t *testing.T) {
	moons := []Moon{
		{ID: 1, Name: "Umbra", PhaseMode: MoonManual, Marks: []MoonMark{{Day: 1, State: MoonFull, Shown: true}, {Day: 5, State: MoonVanished}}},
		{ID: 2, Name: "Secret", HiddenFromPlayers: true},
	}
	got := filterMoonsForViewer(moons, player)
	if len(got) != 1 || len(got[0].Marks) != 1 || got[0].Marks[0].State != MoonFull {
		t.Fatalf("player moons = %+v", got)
	}
}

func TestNormalizeMoonStateNames(t *testing.T) {
	got, err := normalizeMoonStateNames(map[string]string{MoonVanished: " Gone below ", MoonFull: "Full", MoonHalf: ""})
	if err != nil || len(got) != 1 || got[MoonVanished] != "Gone below" {
		t.Fatalf("names = %v %v", got, err)
	}
	if _, err := normalizeMoonStateNames(map[string]string{"gibbous": "x"}); err == nil {
		t.Error("an unknown state was accepted")
	}
	if got, _ := normalizeMoonStateNames(map[string]string{MoonFull: "Full"}); got != nil {
		t.Errorf("only defaults should store nothing, got %v", got)
	}
}

func TestSetMoonByHand(t *testing.T) {
	ctx := context.Background()
	svc, marks := markFixture(t, false)

	if _, err := svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 3, State: MoonFull}, owner); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("a mark on a cycling moon = %v, want 422", err)
	}
	if _, err := svc.SetMoonMode(ctx, "cal", "camp", 7, MoonManual, map[string]string{MoonVanished: "Gone below"}, owner); err != nil {
		t.Fatal(err)
	}
	m, err := svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 9, State: MoonVanished, Shown: true}, owner)
	if err != nil || len(m.Marks) != 1 || m.StateWord(MoonVanished) != "Gone below" {
		t.Fatalf("set mark = %+v %v", m, err)
	}
	if _, err := svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 2, Day: 25, State: MoonFull}, owner); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Errorf("a mark on a day the calendar lacks = %v, want 422", err)
	}
	if _, err := svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 4, State: "gibbous"}, owner); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Errorf("an unknown state = %v, want 422", err)
	}
	if _, err := svc.SetMoonMark(ctx, "cal", "camp", 99, MoonMark{Year: 5, Month: 1, Day: 4, State: MoonFull}, owner); apperror.SafeCode(err) != http.StatusNotFound {
		t.Errorf("another calendar's moon = %v, want 404", err)
	}

	// Writes are the Director's.
	checks := map[string]error{}
	_, checks["mode"] = svc.SetMoonMode(ctx, "cal", "camp", 7, MoonCycle, nil, player)
	_, checks["mark"] = svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 4, State: MoonFull}, player)
	_, checks["delete"] = svc.DeleteMoonMark(ctx, "cal", "camp", 7, DayDate{Year: 5, Month: 1, Day: 9}, player)
	for name, err := range checks {
		if apperror.SafeCode(err) != http.StatusForbidden {
			t.Errorf("%s by a player = %v, want 403", name, err)
		}
	}

	if m, err = svc.DeleteMoonMark(ctx, "cal", "camp", 7, DayDate{Year: 5, Month: 1, Day: 9}, owner); err != nil || len(m.Marks) != 0 {
		t.Fatalf("delete = %+v %v", m, err)
	}
	if _, err := svc.DeleteMoonMark(ctx, "cal", "camp", 7, DayDate{Year: 5, Month: 1, Day: 9}, owner); apperror.SafeCode(err) != http.StatusNotFound {
		t.Errorf("deleting a missing mark = %v, want 404", err)
	}
	if len(marks.marks[7]) != 0 {
		t.Errorf("marks left: %v", marks.marks[7])
	}

	// A player reading the calendar gets only the marks they were shown.
	_, _ = svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 3, State: MoonFull, Shown: true}, owner)
	_, _ = svc.SetMoonMark(ctx, "cal", "camp", 7, MoonMark{Year: 5, Month: 1, Day: 9, State: MoonVanished}, owner)
	cal, err := svc.GetCalendarForViewer(ctx, "cal", "camp", player)
	if err != nil || len(cal.Moons) != 1 || len(cal.Moons[0].Marks) != 1 || cal.Moons[0].StateOn(cal.absDayIndex(5, 1, 12)) != MoonFull {
		t.Fatalf("player calendar moons = %+v %v", cal.Moons, err)
	}
}

func TestRealWorldMoonCannotBeSetByHand(t *testing.T) {
	svc, _ := markFixture(t, true)
	if _, err := svc.SetMoonMode(context.Background(), "cal", "camp", 7, MoonManual, nil, owner); apperror.SafeCode(err) != http.StatusUnprocessableEntity {
		t.Fatalf("manual on a real-world calendar = %v, want 422", err)
	}
}
