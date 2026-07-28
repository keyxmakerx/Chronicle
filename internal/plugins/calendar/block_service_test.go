package calendar

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// --- a counting fake of the narrow BlockRepository --------------------------

// blockFakeRepo implements BlockRepository and COUNTS every call, so the batch
// guarantee ("nine queries, not nine per calendar") can be asserted rather than
// asserted-about-in-a-comment.
type blockFakeRepo struct {
	cals   map[string]*Calendar
	order  []string // ListByCampaignID order
	events map[string][]Event

	calls map[string]int
	// tieReadIDs records the event ids handed to the LAST TiedEventIDsForEntity
	// call, so the ordering invariant — the tie read only ever sees
	// viewer-filtered events — can be asserted rather than trusted to a comment.
	tieReadIDs []string
}

func newBlockFakeRepo(cals ...*Calendar) *blockFakeRepo {
	r := &blockFakeRepo{
		cals:   map[string]*Calendar{},
		events: map[string][]Event{},
		calls:  map[string]int{},
	}
	for _, c := range cals {
		r.cals[c.ID] = c
		r.order = append(r.order, c.ID)
	}
	return r
}

func (r *blockFakeRepo) hit(name string) { r.calls[name]++ }

// shallow returns the calendar row WITHOUT sub-resources, which is what
// ListByCampaignID / GetByID actually return.
func (r *blockFakeRepo) shallow(c *Calendar) Calendar {
	out := *c
	out.Months, out.Weekdays, out.Moons = nil, nil, nil
	out.Seasons, out.Eras, out.EventCategories = nil, nil, nil
	out.Festivals, out.Cycles, out.Weather = nil, nil, nil
	return out
}

func (r *blockFakeRepo) GetByID(_ context.Context, id string) (*Calendar, error) {
	r.hit("GetByID")
	c, ok := r.cals[id]
	if !ok {
		return nil, nil
	}
	s := r.shallow(c)
	return &s, nil
}

func (r *blockFakeRepo) ListByCampaignID(_ context.Context, campaignID string) ([]Calendar, error) {
	r.hit("ListByCampaignID")
	var out []Calendar
	for _, id := range r.order {
		if r.cals[id].CampaignID == campaignID {
			out = append(out, r.shallow(r.cals[id]))
		}
	}
	return out, nil
}

// ListEventsForMonth mirrors the real SQL: base-visibility narrowing only, with
// recurring rows pulled in regardless of their stored month.
func (r *blockFakeRepo) ListEventsForMonth(_ context.Context, calendarID string, year, month, role int) ([]Event, error) {
	r.hit("ListEventsForMonth")
	var out []Event
	for _, e := range r.events[calendarID] {
		if e.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
			continue
		}
		if (e.Year == year && e.Month == month) || e.IsRecurring {
			out = append(out, e)
		}
	}
	return out, nil
}

func (r *blockFakeRepo) MonthsForCalendars(_ context.Context, ids []string) (map[string][]Month, error) {
	r.hit("MonthsForCalendars")
	out := map[string][]Month{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Months
		}
	}
	return out, nil
}

func (r *blockFakeRepo) WeekdaysForCalendars(_ context.Context, ids []string) (map[string][]Weekday, error) {
	r.hit("WeekdaysForCalendars")
	out := map[string][]Weekday{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Weekdays
		}
	}
	return out, nil
}

func (r *blockFakeRepo) MoonsForCalendars(_ context.Context, ids []string) (map[string][]Moon, error) {
	r.hit("MoonsForCalendars")
	out := map[string][]Moon{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Moons
		}
	}
	return out, nil
}

func (r *blockFakeRepo) SeasonsForCalendars(_ context.Context, ids []string) (map[string][]Season, error) {
	r.hit("SeasonsForCalendars")
	out := map[string][]Season{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Seasons
		}
	}
	return out, nil
}

func (r *blockFakeRepo) ErasForCalendars(_ context.Context, ids []string) (map[string][]Era, error) {
	r.hit("ErasForCalendars")
	out := map[string][]Era{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Eras
		}
	}
	return out, nil
}

func (r *blockFakeRepo) EventCategoriesForCalendars(_ context.Context, ids []string) (map[string][]EventCategory, error) {
	r.hit("EventCategoriesForCalendars")
	out := map[string][]EventCategory{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.EventCategories
		}
	}
	return out, nil
}

func (r *blockFakeRepo) FestivalsForCalendars(_ context.Context, ids []string) (map[string][]Festival, error) {
	r.hit("FestivalsForCalendars")
	out := map[string][]Festival{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Festivals
		}
	}
	return out, nil
}

func (r *blockFakeRepo) CyclesForCalendars(_ context.Context, ids []string) (map[string][]Cycle, error) {
	r.hit("CyclesForCalendars")
	out := map[string][]Cycle{}
	for _, id := range ids {
		if c := r.cals[id]; c != nil {
			out[id] = c.Cycles
		}
	}
	return out, nil
}

func (r *blockFakeRepo) EntitiesForEventsBatch(_ context.Context, eventIDs []string, role int, userID string) (map[string][]EntityTieRef, error) {
	r.hit("EntitiesForEventsBatch")
	return map[string][]EntityTieRef{}, nil
}

func (r *blockFakeRepo) TiedEventIDsForEntity(_ context.Context, entityID string, eventIDs []string) (map[string]bool, error) {
	r.hit("TiedEventIDsForEntity")
	r.tieReadIDs = append([]string(nil), eventIDs...)
	return map[string]bool{}, nil
}

// UpcomingEventsForCalendars mirrors the real SQL: base-visibility narrowing in
// "SQL", nothing else — the Go resolver is the service's job.
func (r *blockFakeRepo) UpcomingEventsForCalendars(_ context.Context, ids []string, role int) (map[string][]Event, error) {
	r.hit("UpcomingEventsForCalendars")
	out := map[string][]Event{}
	for _, id := range ids {
		for _, e := range r.events[id] {
			if e.Visibility == "dm_only" && !permissions.CanSeeDmOnly(role) {
				continue
			}
			out[id] = append(out[id], e)
		}
	}
	return out, nil
}

// --- fixtures ---------------------------------------------------------------

// blockDeepCountCal is a second in-world calendar with an epoch UNRELATED to
// the first: year 218, ten months of 36 days, an eight-day week. Nothing about
// its dates is comparable to Harptos's — which is the whole point of the
// ordering pin below.
func blockDeepCountCal() *Calendar {
	cal := &Calendar{
		ID: "cal-dwarven", CampaignID: "camp-1", Mode: ModeFantasy,
		Name: "Dwarven Deep-count", CurrentYear: 218, CurrentMonth: 5, CurrentDay: 20,
		SortOrder: 1,
	}
	for i := 0; i < 10; i++ {
		cal.Months = append(cal.Months, Month{ID: 100 + i, CalendarID: cal.ID,
			Name: fmt.Sprintf("Deep%d", i+1), Days: 36, SortOrder: i})
	}
	for i := 0; i < 8; i++ {
		cal.Weekdays = append(cal.Weekdays, Weekday{ID: 100 + i, CalendarID: cal.ID,
			Name: fmt.Sprintf("D%d", i+1), SortOrder: i})
	}
	return cal
}

// --- the batch guarantee ----------------------------------------------------

// TestBlockEagerLoadIsBatchedNotNPlusOne is the guard for the Bench's landing
// cost. eagerLoad (service.go :668) is nine sequential queries PER CALENDAR;
// four calendars there is 1 + 36. Here it must be a constant.
func TestBlockEagerLoadIsBatchedNotNPlusOne(t *testing.T) {
	a, b := blockTenDayCal(), blockDeepCountCal()
	c, d := blockTenDayCal(), blockTenDayCal()
	c.ID, c.IsDefault, c.SortOrder = "cal-c", false, 2
	d.ID, d.IsDefault, d.SortOrder = "cal-d", false, 3
	repo := newBlockFakeRepo(a, b, c, d)
	svc := NewBlockService(repo)

	cals, err := svc.EagerLoadCampaignCalendars(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cals) != 4 {
		t.Fatalf("loaded %d calendars, want 4", len(cals))
	}
	for _, cal := range cals {
		if len(cal.Months) == 0 {
			t.Fatalf("%s came back with no Months — the Bench cannot print an in-world "+
				"date label without them, which is the whole reason this loader exists", cal.ID)
		}
	}
	total := 0
	for name, n := range repo.calls {
		total += n
		if n != 1 {
			t.Fatalf("%s was called %d times for 4 calendars; every loader must be batched", name, n)
		}
	}
	// 1 listing + 8 batched sub-resource reads. eagerLoad's equivalent is 1 + 36.
	if total != 9 {
		t.Fatalf("total queries = %d, want 9 (calls: %v)", total, repo.calls)
	}
	if cals[0].Weather != nil {
		t.Fatal("Weather is deliberately not batch-loaded; it must stay nil, not be guessed")
	}
}

func TestBlockEagerLoadCalendarsDedupesAndTolerates(t *testing.T) {
	a := blockTenDayCal()
	repo := newBlockFakeRepo(a)
	svc := NewBlockService(repo)

	got, err := svc.EagerLoadCalendars(context.Background(),
		[]string{a.ID, a.ID, "", "missing"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[a.ID] == nil {
		t.Fatalf("loaded %v, want just the one real calendar", got)
	}
	if repo.calls["GetByID"] != 2 {
		t.Fatalf("GetByID called %d times; duplicates and blanks must be dropped first",
			repo.calls["GetByID"])
	}
	if repo.calls["MonthsForCalendars"] != 1 {
		t.Fatalf("sub-resources were not batched: %v", repo.calls)
	}
	if empty, err := svc.EagerLoadCalendars(context.Background(), nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty id set = %v, %v", empty, err)
	}
}

// TestBlockEagerLoadAppliesTheRealTimeSeam pins that a batch-loaded real-time
// calendar shows the wall clock, not its stale stored date (C-REAL-CALENDAR-P2
// F1). Without the seam the Bench would print a date that is months old.
func TestBlockEagerLoadAppliesTheRealTimeSeam(t *testing.T) {
	rt := blockRealTimeCal()
	repo := newBlockFakeRepo(rt)
	svc := NewBlockService(repo)
	svc.SetRealTimeSeam(blockSeamStub{year: 2099, month: 12, day: 31})

	got, err := svc.EagerLoadCalendars(context.Background(), []string{rt.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got[rt.ID].CurrentYear != 2099 {
		t.Fatalf("CurrentYear = %d; the real-time seam did not run", got[rt.ID].CurrentYear)
	}
	// A nil seam degrades to the stored date rather than panicking.
	plain := NewBlockService(newBlockFakeRepo(blockRealTimeCal()))
	if _, err := plain.EagerLoadCalendars(context.Background(), []string{rt.ID}); err != nil {
		t.Fatalf("a nil real-time seam must degrade, not error: %v", err)
	}
}

type blockSeamStub struct{ year, month, day int }

func (s blockSeamStub) ApplyRealTime(cal *Calendar) {
	if cal == nil || !cal.UsesRealTime() {
		return
	}
	cal.CurrentYear, cal.CurrentMonth, cal.CurrentDay = s.year, s.month, s.day
}

// --- THE CROSS-CALENDAR ORDERING PIN ---------------------------------------

// TestBlockUpcomingOrderingAcrossIncomparableCalendars pins the rule the
// dispatch requires be CHOSEN, STATED and PINNED.
//
// Harptos year 1523 and a Dwarven deep-count year 218 share no epoch, no month
// length and no week length. Sorting by (year, month, day) across them is
// meaningless — 218 would always sort first regardless of how far away the
// event is. The one quantity defined for both is HOW MANY OF ITS OWN DAYS
// separate the occurrence from that calendar's current date, so:
//
//	primary   DaysUntil ascending
//	tie-break calendar sort_order, then calendar name, then the date tuple,
//	          then event name, then event id  (a TOTAL, collation-independent order)
func TestBlockUpcomingOrderingAcrossIncomparableCalendars(t *testing.T) {
	harptos := blockTenDayCal() // 1523-01-14, sort_order 0
	dwarven := blockDeepCountCal()
	repo := newBlockFakeRepo(harptos, dwarven)
	repo.events[harptos.ID] = []Event{
		{ID: "h-far", CalendarID: harptos.ID, Name: "Ward levy", Year: 1523, Month: 1, Day: 17, Visibility: "everyone"},
		{ID: "h-soon", CalendarID: harptos.ID, Name: "Council", Year: 1523, Month: 1, Day: 15, Visibility: "everyone"},
	}
	repo.events[dwarven.ID] = []Event{
		// Deep-count year 218, month 5, day 21 — one of ITS days from its own
		// current date, even though its year number is far "behind" Harptos's.
		{ID: "d-soon", CalendarID: dwarven.ID, Name: "Deep vigil", Year: 218, Month: 5, Day: 21, Visibility: "everyone"},
		{ID: "d-far", CalendarID: dwarven.ID, Name: "Forge rite", Year: 218, Month: 5, Day: 30, Visibility: "everyone"},
	}

	svc := NewBlockService(repo)
	got, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u", Role: permissions.RoleOwner}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("got %d rows, want 4", len(got))
	}

	wantIDs := []string{"h-soon", "d-soon", "h-far", "d-far"}
	wantDays := []int{1, 1, 3, 10}
	for i, row := range got {
		if row.Event.ID != wantIDs[i] {
			t.Fatalf("row %d = %s (%d days), want %s — ordering is DaysUntil ascending, "+
				"then calendar sort_order", i, row.Event.ID, row.DaysUntil, wantIDs[i])
		}
		if row.DaysUntil != wantDays[i] {
			t.Fatalf("row %d (%s) DaysUntil = %d, want %d", i, row.Event.ID, row.DaysUntil, wantDays[i])
		}
	}
	// The tie at 1 day is broken by sort_order: Harptos (0) before Dwarven (1).
	if got[0].Calendar.SortOrder >= got[1].Calendar.SortOrder {
		t.Fatal("an equal-proximity tie must break on the operator's own calendar order")
	}
	// n bounds the index.
	short, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u", Role: permissions.RoleOwner}, 2)
	if err != nil || len(short) != 2 {
		t.Fatalf("n=2 returned %d rows (%v)", len(short), err)
	}
}

// TestBlockUpcomingIsViewerFiltered pins that the NEXT UP index does not repeat
// the existing UpcomingByCalendar leak, where base visibility is narrowed in
// SQL only and filterEventsByUser is never called — so an event carrying
// visibility_rules {allowed_users} hands its NAME to a player.
func TestBlockUpcomingIsViewerFiltered(t *testing.T) {
	harptos := blockTenDayCal()
	repo := newBlockFakeRepo(harptos)
	restricted := Event{ID: "restricted", CalendarID: harptos.ID, Name: "Secret council",
		Year: 1523, Month: 1, Day: 16, Visibility: "everyone",
		VisibilityRules: blockStrPtr(`{"allowed_users":["u-gm"]}`)}
	repo.events[harptos.ID] = []Event{
		{ID: "open", CalendarID: harptos.ID, Name: "Open court", Year: 1523, Month: 1, Day: 15, Visibility: "everyone"},
		{ID: "gm", CalendarID: harptos.ID, Name: "GM plot", Year: 1523, Month: 1, Day: 15, Visibility: "dm_only"},
		restricted,
	}
	svc := NewBlockService(repo)

	player, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range player {
		if row.Event.ID != "open" {
			t.Fatalf("a player received %q (%s) in NEXT UP", row.Event.Name, row.Event.ID)
		}
	}
	if len(player) != 1 {
		t.Fatalf("player rows = %d, want 1", len(player))
	}

	gm, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(gm) != 3 {
		t.Fatalf("GM rows = %d, want 3", len(gm))
	}
}

// TestBlockUpcomingSkipsCalendarsTheViewerCannotSee pins that calendar-level
// visibility gates the whole calendar before its events are read.
func TestBlockUpcomingSkipsCalendarsTheViewerCannotSee(t *testing.T) {
	open := blockTenDayCal()
	secret := blockDeepCountCal()
	secret.Visibility = "dm_only"
	repo := newBlockFakeRepo(open, secret)
	repo.events[open.ID] = []Event{{ID: "open", CalendarID: open.ID, Name: "Court",
		Year: 1523, Month: 1, Day: 15, Visibility: "everyone"}}
	repo.events[secret.ID] = []Event{{ID: "hidden-cal", CalendarID: secret.ID, Name: "Deep secret",
		Year: 218, Month: 5, Day: 21, Visibility: "everyone"}}

	svc := NewBlockService(repo)
	player, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(player) != 1 || player[0].Event.ID != "open" {
		t.Fatalf("player rows = %+v; an event on a dm_only CALENDAR must not appear", player)
	}
}

func TestBlockUpcomingExpandsRecurrenceForwardOnly(t *testing.T) {
	cal := blockTenDayCal() // current 1523-01-14, ten-day weeks
	repo := newBlockFakeRepo(cal)
	rt := RecurrenceWeekly
	repo.events[cal.ID] = []Event{{ID: "weekly", CalendarID: cal.ID, Name: "Tenday market",
		Year: 1500, Month: 1, Day: 2, Visibility: "everyone", IsRecurring: true, RecurrenceType: &rt}}

	svc := NewBlockService(repo)
	got, err := svc.UpcomingAcrossCalendars(context.Background(), "camp-1",
		BlockViewer{UserID: "u", Role: permissions.RoleOwner}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1 — a recurring event contributes its NEXT occurrence", len(got))
	}
	if got[0].DaysUntil < 0 {
		t.Fatalf("a recurring event resolved to a PAST occurrence (%d days)", got[0].DaysUntil)
	}
	if got[0].Date.Year != 1523 {
		t.Fatalf("next occurrence resolved to year %d, want 1523", got[0].Date.Year)
	}
}

// --- EventsForDay -----------------------------------------------------------

func TestBlockEventsForDayFiltersOnceAndExpandsRecurrence(t *testing.T) {
	cal := blockTenDayCal()
	repo := newBlockFakeRepo(cal)
	rt := RecurrenceWeekly
	repo.events[cal.ID] = []Event{
		{ID: "once", CalendarID: cal.ID, Name: "Once", Year: 1523, Month: 1, Day: 12, Visibility: "everyone"},
		{ID: "weekly", CalendarID: cal.ID, Name: "Weekly", Year: 1523, Month: 1, Day: 2,
			Visibility: "everyone", IsRecurring: true, RecurrenceType: &rt},
		{ID: "gm", CalendarID: cal.ID, Name: "GM only", Year: 1523, Month: 1, Day: 12, Visibility: "dm_only"},
	}
	svc := NewBlockService(repo)

	player, err := svc.EventsForDay(context.Background(), cal.ID,
		BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}, BlockDate{Year: 1523, Month: 1, Day: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(player) != 2 {
		t.Fatalf("player got %d events on day 12, want 2 (Once + the weekly instance)", len(player))
	}
	for _, e := range player {
		if e.Visibility == "dm_only" {
			t.Fatal("a dm_only event reached a player")
		}
	}
	gm, err := svc.EventsForDay(context.Background(), cal.ID,
		BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}, BlockDate{Year: 1523, Month: 1, Day: 12})
	if err != nil {
		t.Fatal(err)
	}
	if len(gm) != 3 {
		t.Fatalf("GM got %d events on day 12, want 3", len(gm))
	}
	// Deterministic order regardless of the repo's return order.
	if gm[0].Name > gm[1].Name && gm[0].Day == gm[1].Day {
		t.Fatalf("EventsForDay is not stably ordered: %v", []string{gm[0].Name, gm[1].Name})
	}
}

// --- calendar-level visibility on the spine (C-CALV4-SEAM-P5 §4) ------------

// TestBlockSpineCalendarVisibilityGate pins that Block and EventsForDay refuse
// a calendar the viewer may not see with EXACTLY the not-found shape a
// nonexistent calendar produces — the apperror.NewNotFound("calendar not
// found") handler_v2.go answers through requireVisibleCalendar — so a hidden
// calendar's existence cannot be inferred from a distinguishable error.
//
// The gate lives HERE, in the spine, because calendarVisibleTo is unexported:
// a phase-B host outside the package cannot reproduce it, and its only
// alternative would be the 60-method service the spine exists to avoid.
func TestBlockSpineCalendarVisibilityGate(t *testing.T) {
	newSvc := func() *BlockService {
		open := blockTenDayCal()
		secret := blockDeepCountCal()
		secret.Visibility = "dm_only"
		repo := newBlockFakeRepo(open, secret)
		repo.events[open.ID] = []Event{{ID: "e-open", CalendarID: open.ID, Name: "Open court",
			Year: 1523, Month: 1, Day: 14, Visibility: "everyone"}}
		repo.events[secret.ID] = []Event{{ID: "e-secret", CalendarID: secret.ID, Name: "Deep secret",
			Year: 218, Month: 5, Day: 20, Visibility: "everyone"}}
		return NewBlockService(repo)
	}

	player := BlockViewer{UserID: "u-player", Role: permissions.RolePlayer}
	gm := BlockViewer{UserID: "u-gm", Role: permissions.RoleOwner}

	entryPoints := []struct {
		name string
		call func(svc *BlockService, calID string, v BlockViewer) error
	}{
		{"Block", func(svc *BlockService, calID string, v BlockViewer) error {
			_, err := svc.Block(context.Background(), BlockRequest{CalendarID: calID, Viewer: v})
			return err
		}},
		{"EventsForDay", func(svc *BlockService, calID string, v BlockViewer) error {
			_, err := svc.EventsForDay(context.Background(), calID, v,
				BlockDate{Year: 1523, Month: 1, Day: 14})
			return err
		}},
	}

	cases := []struct {
		name         string
		calID        string
		viewer       BlockViewer
		wantNotFound bool
	}{
		{"player + dm_only calendar → the missing-calendar answer", "cal-dwarven", player, true},
		{"player + nonexistent calendar → not-found", "cal-does-not-exist", player, true},
		{"GM + dm_only calendar → success", "cal-dwarven", gm, false},
		{"player + visible calendar → success", "cal-harptos", player, false},
	}

	for _, ep := range entryPoints {
		t.Run(ep.name, func(t *testing.T) {
			svc := newSvc()
			// The oracle: what THIS entry point answers for an id that does not
			// exist at all. Every hidden-calendar refusal below must be
			// byte-for-byte this.
			missingErr := ep.call(svc, "cal-does-not-exist", player)
			if missingErr == nil {
				t.Fatal("a nonexistent calendar must error")
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					err := ep.call(svc, tc.calID, tc.viewer)
					if !tc.wantNotFound {
						if err != nil {
							t.Fatalf("want success, got %v", err)
						}
						return
					}
					if err == nil {
						t.Fatalf("calendar %q must be refused for this viewer", tc.calID)
					}
					var appErr *apperror.AppError
					if !errors.As(err, &appErr) || appErr.Code != http.StatusNotFound {
						t.Fatalf("refusal is %v, want the apperror not-found shape handler_v2.go uses", err)
					}
					if err.Error() != missingErr.Error() {
						t.Fatalf("a hidden calendar is distinguishable from a missing one:\n  hidden:  %q\n  missing: %q",
							err.Error(), missingErr.Error())
					}
					if want := apperror.NewNotFound("calendar not found").Error(); err.Error() != want {
						t.Fatalf("refusal %q does not match requireVisibleCalendar's %q", err.Error(), want)
					}
				})
			}
		})
	}
}

// TestBlockTieReadReceivesOnlyViewerVisibleEvents pins the §4 ordering fix:
// TiedEventIDsForEntity applies no visibility filter of its own — its contract
// is that the event set handed to it is ALREADY viewer-filtered. That is only
// true if Block runs filterEventsByUser BEFORE the tie read, not later inside
// projectBlock. Base dm_only narrowing already happens in "SQL"
// (ListEventsForMonth), so the event that leaks through a late filter is one
// carrying visibility_rules.
func TestBlockTieReadReceivesOnlyViewerVisibleEvents(t *testing.T) {
	cal := blockTenDayCal()
	repo := newBlockFakeRepo(cal)
	repo.events[cal.ID] = []Event{
		{ID: "e-open", CalendarID: cal.ID, Name: "Open court",
			Year: 1523, Month: 1, Day: 3, Visibility: "everyone"},
		{ID: "e-rules", CalendarID: cal.ID, Name: "Secret council",
			Year: 1523, Month: 1, Day: 5, Visibility: "everyone",
			VisibilityRules: blockStrPtr(`{"allowed_users":["u-gm"]}`)},
	}
	svc := NewBlockService(repo)

	if _, err := svc.Block(context.Background(), BlockRequest{
		CalendarID: cal.ID,
		Viewer:     BlockViewer{UserID: "u-player", Role: permissions.RolePlayer, HostEntity: "ent-1"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := repo.calls["TiedEventIDsForEntity"]; got != 1 {
		t.Fatalf("tie read issued %d times, want 1", got)
	}
	for _, id := range repo.tieReadIDs {
		if id == "e-rules" {
			t.Fatalf("the tie read received %q, an event this viewer may not see — "+
				"filterEventsByUser must run before tiedEventIDs (tie read saw %v)",
				id, repo.tieReadIDs)
		}
	}
	if len(repo.tieReadIDs) != 1 || repo.tieReadIDs[0] != "e-open" {
		t.Fatalf("tie read saw %v, want exactly the viewer-visible [e-open]", repo.tieReadIDs)
	}
}

// --- the sync pill ----------------------------------------------------------

type blockProbeStub struct {
	snap BlockSyncSnapshot
	err  error
}

func (p blockProbeStub) CampaignSyncSnapshot(context.Context, string) (BlockSyncSnapshot, error) {
	return p.snap, p.err
}

// TestBlockCalendarLinkStatus pins ruling COMMON §6.3 — the numerator is
// DEFINED, not queried — and the signed strings.
func TestBlockCalendarLinkStatus(t *testing.T) {
	a, b := blockTenDayCal(), blockDeepCountCal()
	c, d := blockTenDayCal(), blockTenDayCal()
	c.ID, c.IsDefault = "cal-c", false
	d.ID, d.IsDefault = "cal-d", false
	repo := newBlockFakeRepo(a, b, c, d)

	// No probe installed → not linked, but the DENOMINATOR still tells the truth.
	svc := NewBlockService(repo)
	pill, err := svc.CalendarLinkStatus(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if pill.State != blockSyncStateNone || pill.Linked != 0 || pill.Total != 4 {
		t.Fatalf("unprobed pill = %+v, want none / 0 of 4", pill)
	}
	if pill.Full != "Not linked · 0 of 4 linked" || pill.Compact != "Not linked · 0 of 4" {
		t.Fatalf("none strings = %q / %q", pill.Full, pill.Compact)
	}

	// Connected, applied date matches the campaign default → in sync.
	svc.SetSyncLinkProbe(blockProbeStub{snap: BlockSyncSnapshot{
		Connected: true, Transport: "Foundry",
		LastSeen:     time.Now().Add(-2 * time.Minute),
		AppliedYear:  blockIntPtr(a.CurrentYear),
		AppliedMonth: blockIntPtr(a.CurrentMonth),
		AppliedDay:   blockIntPtr(a.CurrentDay),
	}})
	pill, err = svc.CalendarLinkStatus(context.Background(), "camp-1")
	if err != nil {
		t.Fatal(err)
	}
	if pill.State != blockSyncStateOK || pill.Linked != 1 || pill.Total != 4 {
		t.Fatalf("connected pill = %+v, want ok / 1 of 4", pill)
	}
	if pill.Full != "In sync · Foundry · pushed 2m ago · 1 of 4 linked" {
		t.Fatalf("ok Full = %q", pill.Full)
	}
	if pill.Compact != "In sync · 1 of 4" {
		t.Fatalf("ok Compact = %q", pill.Compact)
	}

	// Connected but the applied date is three of the default calendar's days
	// behind → drifted, with the magnitude named.
	svc.SetSyncLinkProbe(blockProbeStub{snap: BlockSyncSnapshot{
		Connected: true, Transport: "Foundry",
		LastSeen:     time.Now().Add(-2 * time.Minute),
		AppliedYear:  blockIntPtr(a.CurrentYear),
		AppliedMonth: blockIntPtr(a.CurrentMonth),
		AppliedDay:   blockIntPtr(a.CurrentDay - 3),
	}})
	pill, _ = svc.CalendarLinkStatus(context.Background(), "camp-1")
	if pill.State != blockSyncStateDrift {
		t.Fatalf("drift state = %q", pill.State)
	}
	if pill.Full != "Drifted · 3 days · 1 of 4 linked" || pill.Compact != "Drifted · 1 of 4" {
		t.Fatalf("drift strings = %q / %q", pill.Full, pill.Compact)
	}

	// A never-confirmed module is CONNECTED, not drifted — calling it drift
	// would warn every operator who has simply not pressed push yet.
	svc.SetSyncLinkProbe(blockProbeStub{snap: BlockSyncSnapshot{
		Connected: true, Transport: "Foundry", LastSeen: time.Now().Add(-90 * time.Minute)}})
	pill, _ = svc.CalendarLinkStatus(context.Background(), "camp-1")
	if pill.State != blockSyncStateOK {
		t.Fatalf("never-confirmed state = %q, want ok", pill.State)
	}
	if !strings.Contains(pill.Full, "pushed 1h ago") {
		t.Fatalf("ok Full = %q, want an hours-scale ago", pill.Full)
	}

	// A failing probe degrades to "not linked" WITHOUT dropping the denominator.
	svc.SetSyncLinkProbe(blockProbeStub{err: fmt.Errorf("boom")})
	pill, err = svc.CalendarLinkStatus(context.Background(), "camp-1")
	if err == nil {
		t.Fatal("a probe failure must be reported to the caller")
	}
	if pill.Total != 4 || pill.State != blockSyncStateNone {
		t.Fatalf("degraded pill = %+v; the denominator NEVER drops", pill)
	}
}

func TestBlockSyncStringsDropAbsentSegments(t *testing.T) {
	full, compact := blockSyncStrings(blockSyncFacts{State: blockSyncStateOK, Linked: 1, Total: 2})
	if full != "In sync · 1 of 2 linked" {
		t.Fatalf("Full with no transport/timestamp = %q; empty segments must be dropped, "+
			"not printed blank", full)
	}
	if compact != "In sync · 1 of 2" {
		t.Fatalf("Compact = %q", compact)
	}
	if f, c := blockSyncStrings(blockSyncFacts{State: blockSyncStateBad}); f != "Incompatible structure" || c != "Incompatible" {
		t.Fatalf("bad strings = %q / %q", f, c)
	}
}

func TestBlockHumanAgoAndDays(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "moments"},
		{2 * time.Minute, "2m"},
		{3 * time.Hour, "3h"},
		{50 * time.Hour, "2d"},
	}
	for _, c := range cases {
		if got := blockHumanAgo(c.d); got != c.want {
			t.Fatalf("blockHumanAgo(%v) = %q, want %q", c.d, got, c.want)
		}
	}
	if blockHumanDays(1) != "1 day" || blockHumanDays(3) != "3 days" {
		t.Fatalf("day pluralisation is wrong: %q / %q", blockHumanDays(1), blockHumanDays(3))
	}
}

// --- end to end -------------------------------------------------------------

func TestBlockServiceProducesBlockData(t *testing.T) {
	cal := blockTenDayCal()
	cal.Moons = []Moon{{ID: 1, CalendarID: cal.ID, Name: "Selune", CycleDays: 30.4}}
	cal.Eras = []Era{{ID: 1, CalendarID: cal.ID, Name: "Reckoning of Wards", StartYear: 1, Color: "#7c5cff"}}
	repo := newBlockFakeRepo(cal)
	repo.events[cal.ID] = []Event{
		{ID: "e1", CalendarID: cal.ID, Name: "Council of Wards", Year: 1523, Month: 1, Day: 3, Visibility: "everyone"},
		{ID: "e2", CalendarID: cal.ID, Name: "Barrow scouting", Year: 1523, Month: 1, Day: 5, Visibility: "dm_only"},
	}
	svc := NewBlockService(repo)

	got, err := svc.Block(context.Background(), BlockRequest{
		CalendarID: cal.ID, CampaignID: "camp-1",
		Viewer: BlockViewer{UserID: "u-player", Role: permissions.RolePlayer},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != cal.Name || got.CalendarSlug != cal.ID {
		t.Fatalf("identity = %q / %q", got.Name, got.CalendarSlug)
	}
	if got.Month.Year != 1523 || got.Month.Index != 0 {
		t.Fatalf("unnavigated Block landed on %d/%d, want the calendar's own current month",
			got.Month.Year, got.Month.Index)
	}
	if got.Month.WeekLen != 10 {
		t.Fatalf("WeekLen = %d", got.Month.WeekLen)
	}
	if got.Viewer.WholeCount != 1 {
		t.Fatalf("player WholeCount = %d, want 1 (the dm_only event is absent)", got.Viewer.WholeCount)
	}
	if got.Sync.Total != 1 {
		t.Fatalf("sync denominator = %d, want the campaign's 1 calendar", got.Sync.Total)
	}
	if len(got.Month.Rows) == 0 || len(got.Month.Rows[0].Bands) == 0 {
		t.Fatal("no era bands were produced for a calendar that declares an era")
	}
	if got.Fault != "" {
		t.Fatalf("unexpected fault %q", got.Fault)
	}
	// Moons are the DEF layer, so discs must be present on dated cells.
	found := false
	for _, row := range got.Month.Rows {
		for _, cell := range row.Cells {
			if cell.Day > 0 && len(cell.Moons) == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("no moon discs on any dated cell despite DEF = [moons]")
	}

	if _, err := svc.Block(context.Background(), BlockRequest{CalendarID: "nope"}); err == nil {
		t.Fatal("a missing calendar must error rather than render an empty Block")
	}
}

func TestBlockRequestViewIsClamped(t *testing.T) {
	cal := blockTenDayCal()
	svc := NewBlockService(newBlockFakeRepo(cal))
	for _, tc := range []struct {
		in                BlockDate
		wantIdx, wantYear int
	}{
		{BlockDate{}, 0, 1523},                       // unset → the calendar's own current month
		{BlockDate{Year: 1600, Month: 5}, 4, 1600},   // honoured
		{BlockDate{Year: 1600, Month: 99}, 11, 1600}, // clamped to the last month
		{BlockDate{Year: 1600, Month: -3}, 0, 1600},  // clamped to the first
	} {
		gotIdx, gotYear := svc.resolveView(cal, tc.in)
		if gotIdx != tc.wantIdx || gotYear != tc.wantYear {
			t.Fatalf("resolveView(%+v) = %d/%d, want %d/%d", tc.in, gotIdx, gotYear, tc.wantIdx, tc.wantYear)
		}
	}
}

// TestBlockSpineInstall pins the composition-root provider: the phase-B
// surfaces reach the spine through it because internal/app/routes.go belongs to
// exactly one calendar-v4 slice for the whole wave.
func TestBlockSpineInstall(t *testing.T) {
	prev := BlockSpine()
	t.Cleanup(func() { blockSpine.Store(prev) })

	blockSpine.Store(nil)
	if BlockSpine() != nil {
		t.Fatal("an uninstalled spine must read as nil so callers can degrade")
	}
	svc := NewBlockService(newBlockFakeRepo())
	InstallBlockSpine(svc)
	if BlockSpine() != svc {
		t.Fatal("InstallBlockSpine did not publish the spine")
	}
}

// TestBlockEntitiesForEventsIsBatched pins that tie rendering is one query, not
// one per event.
func TestBlockEntitiesForEventsIsBatched(t *testing.T) {
	repo := newBlockFakeRepo()
	svc := NewBlockService(repo)
	if _, err := svc.EntitiesForEvents(context.Background(),
		[]string{"e1", "e2", "e3"}, BlockViewer{Role: permissions.RolePlayer}); err != nil {
		t.Fatal(err)
	}
	if repo.calls["EntitiesForEventsBatch"] != 1 {
		t.Fatalf("EntitiesForEventsBatch called %d times for 3 events",
			repo.calls["EntitiesForEventsBatch"])
	}
	if _, err := svc.EntitiesForEvents(context.Background(), nil, BlockViewer{}); err != nil {
		t.Fatal(err)
	}
	if repo.calls["EntitiesForEventsBatch"] != 1 {
		t.Fatal("an empty event set must not issue a query")
	}
}
