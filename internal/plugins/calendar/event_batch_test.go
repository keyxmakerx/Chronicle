package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// fakeBatchRepo records the one write ApplyEventBatch hands the repository.
type fakeBatchRepo struct {
	calls int
	got   EventBatchWrites
	err   error
}

func (f *fakeBatchRepo) ApplyEventBatch(_ context.Context, _ string, w EventBatchWrites) error {
	f.calls++
	f.got = w
	return f.err
}

// batchFixture is a two-month calendar (30 and 20 days) holding three
// stored events: a repeating market, a plain festival and one that belongs
// to another calendar.
func batchFixture(t *testing.T) (*calendarService, *fakeBatchRepo, *recordingPublisher) {
	t.Helper()
	weekly := RecurrenceWeekly
	stored := map[string]Event{
		"market":   {ID: "market", CalendarID: "cal", Name: "Market", Year: 5, Month: 1, Day: 2, IsRecurring: true, RecurrenceType: &weekly, Visibility: "dm_only"},
		"festival": {ID: "festival", CalendarID: "cal", Name: "Festival", Year: 5, Month: 2, Day: 4, Visibility: "everyone"},
	}
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", Name: "Reckoning", Mode: ModeFantasy, Visibility: "everyone",
				HoursPerDay: 24, MinutesPerHour: 60, SecondsPerMinute: 60, CurrentYear: 5, CurrentMonth: 2, CurrentDay: 10}, nil
		},
		getMonthsFn: func(context.Context, string) ([]Month, error) {
			return []Month{{Name: "One", Days: 30}, {Name: "Two", Days: 20}}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		// Like the real query, only events of the asked calendar come back.
		getEventsByIDsFn: func(_ context.Context, calendarID string, ids []string) ([]Event, error) {
			var out []Event
			for _, id := range ids {
				if e, ok := stored[id]; ok && e.CalendarID == calendarID {
					out = append(out, e)
				}
			}
			return out, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil).(*calendarService)
	repo := &fakeBatchRepo{}
	svc.SetEventBatchRepository(repo)
	pub := &recordingPublisher{}
	svc.SetEventPublisher(pub)
	return svc, repo, pub
}

func newGen(name string, y, m, d int) CreateEventInput {
	return CreateEventInput{Name: name, Year: y, Month: m, Day: d, Visibility: "dm_only"}
}

func TestApplyEventBatchRefusesWholeBatch(t *testing.T) {
	tooMany := make([]CreateEventInput, maxBatchEventWrites+1)
	for i := range tooMany {
		tooMany[i] = newGen("x", 5, 2, 12)
	}
	scribe := permissions.RequestViewer(int(permissions.RoleScribe), "u-scribe")
	tests := []struct {
		name   string
		in     EventBatchInput
		actor  BatchActor
		status int
	}{
		{"empty", EventBatchInput{}, BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"over the cap", EventBatchInput{Create: tooMany}, BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"weather over the cap", EventBatchInput{Weather: &EventBatchWeather{Set: make([]DayWeatherInput, maxBatchWeatherDays+1)}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a scribe cannot delete", EventBatchInput{Create: []CreateEventInput{{Name: "ok", Year: 5, Month: 2, Day: 12}}, Delete: []string{"festival"}},
			BatchActor{Viewer: scribe}, http.StatusForbidden},
		{"a scribe cannot write weather", EventBatchInput{Weather: &EventBatchWeather{Clear: []DayDate{{Year: 5, Month: 2, Day: 12}}}},
			BatchActor{Viewer: scribe}, http.StatusForbidden},
		{"a scribe cannot create dm_only", EventBatchInput{Create: []CreateEventInput{{Name: "ok", Year: 5, Month: 2, Day: 12}, newGen("secret", 5, 2, 13)}},
			BatchActor{Viewer: scribe}, http.StatusForbidden},
		{"one bad date refuses the good ones", EventBatchInput{Create: []CreateEventInput{newGen("ok", 5, 2, 12), newGen("bad", 5, 2, 31)}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusUnprocessableEntity},
		{"one bad name refuses the good ones", EventBatchInput{Create: []CreateEventInput{newGen("ok", 5, 2, 12), newGen("", 5, 2, 13)}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a day of weather named twice", EventBatchInput{Weather: &EventBatchWeather{Set: []DayWeatherInput{{Year: 5, Month: 2, Day: 12}, {Year: 5, Month: 2, Day: 12}}}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a weather day the calendar lacks", EventBatchInput{Weather: &EventBatchWeather{Clear: []DayDate{{Year: 5, Month: 3, Day: 1}}}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a change with no id", EventBatchInput{Update: []EventBatchUpdate{{}}}, BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a delete with no id", EventBatchInput{Delete: []string{""}}, BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"an event changed twice", EventBatchInput{Update: []EventBatchUpdate{{ID: "market"}, {ID: "market"}}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"changed and deleted", EventBatchInput{Update: []EventBatchUpdate{{ID: "market"}}, Delete: []string{"market"}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusBadRequest},
		{"a change that fails validation", EventBatchInput{Update: []EventBatchUpdate{{ID: "market", Input: UpdateEventInput{
			RecurrenceEndYear: patch.Of(5), RecurrenceEndMonth: patch.Of(9), RecurrenceEndDay: patch.Of(1)}}}},
			BatchActor{Viewer: owner, CanDelete: true}, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, pub := batchFixture(t)
			_, err := svc.ApplyEventBatch(context.Background(), "cal", "camp", tt.in, tt.actor)
			if err == nil {
				t.Fatal("the batch was accepted")
			}
			if got := apperror.SafeCode(err); got != tt.status {
				t.Errorf("status = %d, want %d (%v)", got, tt.status, err)
			}
			if repo.calls != 0 || len(pub.got) != 0 {
				t.Errorf("a refused batch wrote %d times and published %v", repo.calls, pub.types())
			}
		})
	}
}

func TestApplyEventBatchWritesEverythingOnce(t *testing.T) {
	svc, repo, pub := batchFixture(t)
	end := patch.Of(1)
	in := EventBatchInput{
		Create: []CreateEventInput{newGen("Harvest", 5, 2, 12), newGen("Eclipse", 5, 2, 15)},
		Update: []EventBatchUpdate{
			{ID: "market", Input: UpdateEventInput{RecurrenceEndYear: patch.Of(5), RecurrenceEndMonth: patch.Of(2), RecurrenceEndDay: patch.Of(9)}},
			{ID: "gone", Input: UpdateEventInput{RecurrenceEndYear: end}},
		},
		Delete: []string{"festival", "gone-too"},
		Weather: &EventBatchWeather{
			Clear: []DayDate{{Year: 5, Month: 2, Day: 12}},
			Set:   []DayWeatherInput{{Year: 5, Month: 2, Day: 12, Source: WeatherSourceGenerated}, {Year: 5, Month: 2, Day: 13}},
		},
	}
	out, err := svc.ApplyEventBatch(context.Background(), "cal", "camp", in, BatchActor{Viewer: owner, UserID: "u-owner", CanDelete: true})
	if err != nil {
		t.Fatal(err)
	}
	if repo.calls != 1 {
		t.Fatalf("repository was called %d times, want 1", repo.calls)
	}
	if len(repo.got.Create) != 2 || repo.got.Create[0].Name != "Harvest" || repo.got.Create[0].CreatedBy == nil || *repo.got.Create[0].CreatedBy != "u-owner" {
		t.Errorf("creates = %+v", repo.got.Create)
	}
	if len(repo.got.Update) != 1 {
		t.Fatalf("updates = %d, want 1 (the missing event is skipped)", len(repo.got.Update))
	}
	m := repo.got.Update[0]
	if m.ID != "market" || m.RecurrenceEndDay == nil || *m.RecurrenceEndDay != 9 {
		t.Errorf("the market's repeat end was not set: %+v", m)
	}
	if m.Name != "Market" || m.Visibility != "dm_only" || m.Month != 1 || m.Day != 2 {
		t.Errorf("an update that named only the repeat end changed other fields: %+v", m)
	}
	if !reflect.DeepEqual(repo.got.DeleteIDs, []string{"festival"}) {
		t.Errorf("deletes = %v, want only the stored festival", repo.got.DeleteIDs)
	}
	if repo.got.SetWeather[1].Source != WeatherSourceManual {
		t.Errorf("an unnamed weather source should default to manual, got %q", repo.got.SetWeather[1].Source)
	}
	want := &EventBatchResult{Created: []string{repo.got.Create[0].ID, repo.got.Create[1].ID}, Updated: 1, Deleted: 1, WeatherSet: 2, WeatherCleared: 1}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("result = %+v, want %+v", out, want)
	}
	if got, wantTypes := pub.types(), []string{PubEventCreated, PubEventCreated, PubEventUpdated, PubEventDeleted, PubWeatherChanged}; !reflect.DeepEqual(got, wantTypes) {
		t.Errorf("published %v, want %v", got, wantTypes)
	}
}

// Undo puts a repeat end back, which may mean no end at all: an explicit
// null clears it, and the same update with the key absent leaves it alone.
func TestApplyEventBatchUpdateNullClearsAbsentPreserves(t *testing.T) {
	tests := []struct {
		name  string
		input UpdateEventInput
		want  *int
	}{
		{"absent preserves", UpdateEventInput{}, intp(20)},
		{"null clears", UpdateEventInput{RecurrenceEndYear: patch.Null[int](), RecurrenceEndMonth: patch.Null[int](), RecurrenceEndDay: patch.Null[int]()}, nil},
		{"a value replaces", UpdateEventInput{RecurrenceEndDay: patch.Of(9)}, intp(9)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, _ := batchFixture(t)
			base := svc.eventRepo.(*fakeEventRepo).getEventsByIDsFn
			svc.eventRepo.(*fakeEventRepo).getEventsByIDsFn = func(ctx context.Context, cal string, ids []string) ([]Event, error) {
				evs, err := base(ctx, cal, ids)
				for i := range evs {
					evs[i].RecurrenceEndYear, evs[i].RecurrenceEndMonth, evs[i].RecurrenceEndDay = intp(5), intp(2), intp(20)
				}
				return evs, err
			}
			_, err := svc.ApplyEventBatch(context.Background(), "cal", "camp",
				EventBatchInput{Update: []EventBatchUpdate{{ID: "market", Input: tt.input}}}, BatchActor{Viewer: owner, CanDelete: true})
			if err != nil {
				t.Fatal(err)
			}
			got := repo.got.Update[0].RecurrenceEndDay
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("repeat end day = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyEventBatchRepositoryFailureIsNotPublished(t *testing.T) {
	svc, repo, pub := batchFixture(t)
	repo.err = errors.New("deadlock")
	_, err := svc.ApplyEventBatch(context.Background(), "cal", "camp",
		EventBatchInput{Create: []CreateEventInput{newGen("Harvest", 5, 2, 12)}}, BatchActor{Viewer: owner, CanDelete: true})
	if err == nil {
		t.Fatal("a failed write was reported as success")
	}
	if len(pub.got) != 0 {
		t.Errorf("published %v after a failed write", pub.types())
	}
}

func TestApplyEventBatchNeedsItsRepository(t *testing.T) {
	svc, _, _ := batchFixture(t)
	svc.batchRepo = nil
	if _, err := svc.ApplyEventBatch(context.Background(), "cal", "camp",
		EventBatchInput{Delete: []string{"festival"}}, BatchActor{Viewer: owner, CanDelete: true}); err == nil {
		t.Fatal("a batch with no repository was accepted")
	}
}

func TestApplyEventBatchOtherCampaignsCalendarIsNotFound(t *testing.T) {
	svc, repo, _ := batchFixture(t)
	_, err := svc.ApplyEventBatch(context.Background(), "cal", "other-camp",
		EventBatchInput{Delete: []string{"festival"}}, BatchActor{Viewer: owner, CanDelete: true})
	if apperror.SafeCode(err) != http.StatusNotFound || repo.calls != 0 {
		t.Errorf("err = %v, calls = %d, want a 404 and no write", err, repo.calls)
	}
}

// The wire keeps the partial-update contract inside a batch: a key left out
// preserves, an explicit null clears, a value replaces.
func TestEventBatchRequestKeepsPartialUpdateSemantics(t *testing.T) {
	var req eventBatchRequest
	body := `{"update":[{"id":"market","changes":{"recurrence_end_year":null,"recurrence_end_day":9}}],"delete":["a"],"weather":{"clear":[{"year":5,"month":2,"day":9}]}}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	c := UpdateEventInput(req.Update[0].Changes)
	if !c.RecurrenceEndYear.IsNull() {
		t.Error("an explicit null did not clear")
	}
	if v, ok := c.RecurrenceEndDay.Get(); !ok || v != 9 {
		t.Error("a value did not replace")
	}
	if c.RecurrenceEndMonth.Present() || c.Name.Present() {
		t.Error("an absent key was read as present")
	}
	if req.Weather == nil || len(req.Weather.Clear) != 1 || len(req.Weather.Set) != 0 {
		t.Errorf("weather = %+v", req.Weather)
	}
}

// A fill ends an earlier repeat by setting its repeat-until date to the day
// before today (the repeat's own end, not the first occurrence's span end), so
// the days already marked stay and nothing from today on is left to double up
// with the new fill's own repeat.
func TestRepeatEndedTheDayBeforeTodayKeepsThePastAndDropsTheFuture(t *testing.T) {
	cal := recurrenceCal()
	weekly := RecurrenceWeekly
	market := Event{Year: 1, Month: 1, Day: 2, IsRecurring: true, RecurrenceType: &weekly,
		RecurrenceEndYear: intp(1), RecurrenceEndMonth: intp(2), RecurrenceEndDay: intp(13)}
	tests := []struct {
		name    string
		y, m, d int
		want    bool
	}{
		{"its first day", 1, 1, 2, true},
		{"a marked day before today", 1, 1, 30, true},
		{"the last marked day", 1, 2, 7, true},
		{"today, a market day", 1, 2, 14, false},
		{"a later market day", 1, 3, 7, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := market.OccursOn(cal, tt.y, tt.m, tt.d); got != tt.want {
				t.Errorf("OccursOn(%d-%d-%d) = %v, want %v", tt.y, tt.m, tt.d, got, tt.want)
			}
		})
	}
	market.RecurrenceEndYear, market.RecurrenceEndMonth, market.RecurrenceEndDay = nil, nil, nil
	if !market.OccursOn(cal, 1, 3, 7) {
		t.Error("clearing the end (an Undo) did not bring the later days back")
	}
}
