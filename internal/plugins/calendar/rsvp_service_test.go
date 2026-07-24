package calendar

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Unit tests for the calendar-event RSVP business logic (C-CAL-RSVP-P1). Pure
// helpers + the service orchestration are exercised with in-memory fakes — no DB.

// --- fakes ---

type fakeRSVPRepo struct {
	upserts   []upsertRec
	counts    RSVPCounts
	mine      *EventRSVP
	people    []EventRSVP
	collect   map[string]bool
	token     *EventRSVPToken
	markedIDs []int
}

type upsertRec struct {
	eventID, userID, status string
	note                    *string
}

func (f *fakeRSVPRepo) UpsertRSVP(_ context.Context, eventID, userID, status string, note *string) error {
	f.upserts = append(f.upserts, upsertRec{eventID, userID, status, note})
	return nil
}
func (f *fakeRSVPRepo) GetMyRSVP(_ context.Context, _, _ string) (*EventRSVP, error) {
	return f.mine, nil
}
func (f *fakeRSVPRepo) CountRSVPs(_ context.Context, _ string) (RSVPCounts, error) {
	return f.counts, nil
}
func (f *fakeRSVPRepo) ListRSVPs(_ context.Context, _ string) ([]EventRSVP, error) {
	return f.people, nil
}
func (f *fakeRSVPRepo) GetCollectRSVPs(_ context.Context, eventID string) (bool, error) {
	return f.collect[eventID], nil
}
func (f *fakeRSVPRepo) SetCollectRSVPs(_ context.Context, eventID string, enabled bool) error {
	if f.collect == nil {
		f.collect = map[string]bool{}
	}
	f.collect[eventID] = enabled
	return nil
}
func (f *fakeRSVPRepo) CreateToken(_ context.Context, _, _, _, _ string, _ time.Time) error {
	return nil
}
func (f *fakeRSVPRepo) GetToken(_ context.Context, _ string) (*EventRSVPToken, error) {
	return f.token, nil
}
func (f *fakeRSVPRepo) MarkTokenUsed(_ context.Context, id int) error {
	f.markedIDs = append(f.markedIDs, id)
	if f.token != nil && f.token.ID == id {
		now := time.Now()
		f.token.UsedAt = &now
	}
	return nil
}

type fakeEventReader struct {
	evt *Event
	cal *Calendar
}

func (f *fakeEventReader) GetEvent(_ context.Context, _ string) (*Event, error) {
	return f.evt, nil
}
func (f *fakeEventReader) GetCalendarByID(_ context.Context, _ string) (*Calendar, error) {
	return f.cal, nil
}

type fakeMemberLister struct{ members []campaigns.CampaignMember }

func (f *fakeMemberLister) ListMembers(_ context.Context, _ string) ([]campaigns.CampaignMember, error) {
	return f.members, nil
}

type notifyRec struct {
	userIDs []string
	ntype   string
	message string
}
type fakeNotifier struct{ calls []notifyRec }

func (f *fakeNotifier) NotifyUsers(_ context.Context, userIDs []string, _, ntype, message, _ string) error {
	f.calls = append(f.calls, notifyRec{userIDs, ntype, message})
	return nil
}

type fakeAvailability struct {
	existing map[string]bool
	added    []string
}

func (f *fakeAvailability) ListMyExceptionDates(_ context.Context, _, _ string) (map[string]bool, error) {
	return f.existing, nil
}
func (f *fakeAvailability) AddFullDayUnavailable(_ context.Context, _, _, onDate string) error {
	f.added = append(f.added, onDate)
	return nil
}

// --- pure helpers ---

func TestRSVPStatusForAction(t *testing.T) {
	cases := map[string]string{
		RSVPActionYes:     RSVPStatusYes,
		RSVPActionMaybe:   RSVPStatusMaybe,
		RSVPActionNo:      RSVPStatusNo,
		RSVPActionOutWeek: RSVPStatusNo,
		RSVPActionSuggest: RSVPStatusNo,
	}
	for action, want := range cases {
		if got := rsvpStatusForAction(action); got != want {
			t.Errorf("rsvpStatusForAction(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestValidRSVPAction(t *testing.T) {
	for _, a := range []string{RSVPActionYes, RSVPActionMaybe, RSVPActionNo, RSVPActionOutWeek, RSVPActionSuggest} {
		if !validRSVPAction(a) {
			t.Errorf("validRSVPAction(%q) = false, want true", a)
		}
	}
	for _, a := range []string{"", "going", "declined", "out_month"} {
		if validRSVPAction(a) {
			t.Errorf("validRSVPAction(%q) = true, want false", a)
		}
	}
}

func TestRealWeekDates(t *testing.T) {
	// A Wednesday (2026-07-22). The week (Mon–Sun) is 2026-07-20 .. 2026-07-26.
	now := time.Date(2026, 7, 22, 15, 4, 0, 0, time.UTC)
	got := realWeekDates(now)
	want := []string{"2026-07-20", "2026-07-21", "2026-07-22", "2026-07-23", "2026-07-24", "2026-07-25", "2026-07-26"}
	if len(got) != 7 {
		t.Fatalf("realWeekDates returned %d dates, want 7", len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("realWeekDates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRealWeekDates_Sunday(t *testing.T) {
	// A Sunday must anchor to the PREVIOUS Monday (ISO week), not the next.
	now := time.Date(2026, 7, 26, 1, 0, 0, 0, time.UTC) // Sunday
	got := realWeekDates(now)
	if got[0] != "2026-07-20" || got[6] != "2026-07-26" {
		t.Errorf("Sunday week = %v, want Mon 2026-07-20 .. Sun 2026-07-26", got)
	}
}

func TestFantasyDateLabel(t *testing.T) {
	epoch := "AE"
	cal := &Calendar{EpochName: &epoch, Months: []Month{{Name: "Firstmonth"}, {Name: "Secondmonth"}}}
	sh, sm := 19, 30
	evt := &Event{Year: 812, Month: 2, Day: 5, StartHour: &sh, StartMinute: &sm}
	got := fantasyDateLabel(evt, cal)
	want := "Secondmonth 5, Year 812 AE · 19:30"
	if got != want {
		t.Errorf("fantasyDateLabel = %q, want %q", got, want)
	}
	// Fallback when month index is out of range.
	evt2 := &Event{Year: 1, Month: 99, Day: 3}
	if got := fantasyDateLabel(evt2, cal); got != "Year 1, Month 99, Day 3 AE" {
		t.Errorf("fantasyDateLabel fallback = %q", got)
	}
}

// --- token validation ---

func newTestService(repo RSVPRepository, events EventReader) *rsvpService {
	return NewRSVPService(repo, events, &fakeMemberLister{}, "https://example.test")
}

func TestValidateToken(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-1 * time.Hour)
	used := time.Now()

	t.Run("valid", func(t *testing.T) {
		repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 1, Token: "t", Action: RSVPActionYes, ExpiresAt: future}}
		svc := newTestService(repo, &fakeEventReader{})
		if _, err := svc.ValidateToken(context.Background(), "t"); err != nil {
			t.Fatalf("valid token rejected: %v", err)
		}
	})
	t.Run("missing", func(t *testing.T) {
		svc := newTestService(&fakeRSVPRepo{token: nil}, &fakeEventReader{})
		if _, err := svc.ValidateToken(context.Background(), "x"); err == nil {
			t.Fatal("missing token accepted")
		}
	})
	t.Run("used", func(t *testing.T) {
		repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 1, Action: RSVPActionYes, UsedAt: &used, ExpiresAt: future}}
		svc := newTestService(repo, &fakeEventReader{})
		if _, err := svc.ValidateToken(context.Background(), "t"); err == nil {
			t.Fatal("used token accepted")
		}
	})
	t.Run("expired", func(t *testing.T) {
		repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 1, Action: RSVPActionYes, ExpiresAt: past}}
		svc := newTestService(repo, &fakeEventReader{})
		if _, err := svc.ValidateToken(context.Background(), "t"); err == nil {
			t.Fatal("expired token accepted")
		}
	})
}

// --- apply token side effects ---

func testEventAndCal(owner string) (*Event, *Calendar) {
	evt := &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Council", Visibility: "everyone", CreatedBy: &owner}
	cal := &Calendar{ID: "cal-1", CampaignID: "camp-1", Name: "Harptos"}
	return evt, cal
}

func TestApplyToken_OutWeek_WritesAvailabilitySkippingHandAuthored(t *testing.T) {
	evt, cal := testEventAndCal("owner-1")
	future := time.Now().Add(24 * time.Hour)
	repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 7, Token: "t", EventID: "evt-1", UserID: "u-2", Action: RSVPActionOutWeek, ExpiresAt: future}}
	svc := newTestService(repo, &fakeEventReader{evt: evt, cal: cal})
	// Freeze the clock to a known Wednesday so the week is deterministic.
	svc.now = func() time.Time { return time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) }
	avail := &fakeAvailability{existing: map[string]bool{"2026-07-24": true}} // one hand-authored day
	svc.SetAvailabilityExceptionWriter(avail)
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)

	if _, err := svc.ApplyToken(context.Background(), "t", nil); err != nil {
		t.Fatalf("ApplyToken: %v", err)
	}
	// Token consumed.
	if len(repo.markedIDs) != 1 || repo.markedIDs[0] != 7 {
		t.Errorf("token not marked used: %v", repo.markedIDs)
	}
	// RSVP recorded as "no".
	if len(repo.upserts) != 1 || repo.upserts[0].status != RSVPStatusNo {
		t.Errorf("out_week upsert = %+v, want status no", repo.upserts)
	}
	// 6 days written, the hand-authored 2026-07-24 skipped.
	if len(avail.added) != 6 {
		t.Fatalf("wrote %d days, want 6 (one skipped): %v", len(avail.added), avail.added)
	}
	for _, d := range avail.added {
		if d == "2026-07-24" {
			t.Errorf("hand-authored day 2026-07-24 was overwritten")
		}
	}
	// Owner notified.
	if len(notifier.calls) != 1 || notifier.calls[0].ntype != NotifCalendarRSVP {
		t.Errorf("owner not notified of response: %+v", notifier.calls)
	}
}

func TestApplyToken_Suggest_StoresNoteAndNotifiesOwner(t *testing.T) {
	evt, cal := testEventAndCal("owner-1")
	future := time.Now().Add(24 * time.Hour)
	repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 3, Token: "t", EventID: "evt-1", UserID: "u-2", Action: RSVPActionSuggest, ExpiresAt: future}}
	svc := newTestService(repo, &fakeEventReader{evt: evt, cal: cal})
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)

	note := "Any evening after 8pm"
	if _, err := svc.ApplyToken(context.Background(), "t", &note); err != nil {
		t.Fatalf("ApplyToken: %v", err)
	}
	if len(repo.upserts) != 1 || repo.upserts[0].note == nil || *repo.upserts[0].note != note {
		t.Errorf("suggest note not stored: %+v", repo.upserts)
	}
	if repo.upserts[0].status != RSVPStatusNo {
		t.Errorf("suggest status = %q, want no", repo.upserts[0].status)
	}
	if len(notifier.calls) != 1 {
		t.Fatalf("owner not notified of suggestion: %+v", notifier.calls)
	}
}

func TestApplyToken_OwnerSelfResponse_NoNotify(t *testing.T) {
	// When the owner RSVPs to their own event, skip the self-notify.
	evt, cal := testEventAndCal("owner-1")
	future := time.Now().Add(24 * time.Hour)
	repo := &fakeRSVPRepo{token: &EventRSVPToken{ID: 1, Token: "t", EventID: "evt-1", UserID: "owner-1", Action: RSVPActionYes, ExpiresAt: future}}
	svc := newTestService(repo, &fakeEventReader{evt: evt, cal: cal})
	notifier := &fakeNotifier{}
	svc.SetNotifier(notifier)
	if _, err := svc.ApplyToken(context.Background(), "t", nil); err != nil {
		t.Fatalf("ApplyToken: %v", err)
	}
	if len(notifier.calls) != 0 {
		t.Errorf("owner self-response should not notify: %+v", notifier.calls)
	}
}

func TestApplyAction_InvalidAction(t *testing.T) {
	svc := newTestService(&fakeRSVPRepo{}, &fakeEventReader{})
	if err := svc.ApplyAction(context.Background(), "evt-1", "u-1", "bogus", nil); err == nil {
		t.Fatal("invalid action accepted")
	}
}

// --- visibility-gated fan-out audience ---

func TestViewableMembers_VisibilityGated(t *testing.T) {
	// dm_only event: players are excluded, owner/co-DM included.
	evt := &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Secret", Visibility: "dm_only"}
	members := []campaigns.CampaignMember{
		{UserID: "player", Role: campaigns.RolePlayer},
		{UserID: "scribe", Role: campaigns.RoleScribe},
		{UserID: "owner", Role: campaigns.RoleOwner},
	}
	svc := NewRSVPService(&fakeRSVPRepo{}, &fakeEventReader{}, &fakeMemberLister{members: members}, "")
	got, err := svc.viewableMembers(context.Background(), evt, "camp-1")
	if err != nil {
		t.Fatalf("viewableMembers: %v", err)
	}
	if len(got) != 1 || got[0].UserID != "owner" {
		t.Errorf("dm_only audience = %+v, want only owner", got)
	}
}

func TestViewableMembers_EveryoneEvent(t *testing.T) {
	evt := &Event{ID: "evt-1", CalendarID: "cal-1", Name: "Feast", Visibility: "everyone"}
	members := []campaigns.CampaignMember{
		{UserID: "player", Role: campaigns.RolePlayer},
		{UserID: "owner", Role: campaigns.RoleOwner},
	}
	svc := NewRSVPService(&fakeRSVPRepo{}, &fakeEventReader{}, &fakeMemberLister{members: members}, "")
	got, _ := svc.viewableMembers(context.Background(), evt, "camp-1")
	if len(got) != 2 {
		t.Errorf("everyone audience = %d members, want 2", len(got))
	}
}
