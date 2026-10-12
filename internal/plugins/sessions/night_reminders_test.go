package sessions

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestRemindNight(t *testing.T) {
	members := []NightMember{{"dm", "Echo"}, {"u1", "Ana"}, {"u2", "Bryn"}, {"u3", "Cal"}, {"u4", "Dee"}, {"u5", "Eli"}}
	// Ana is coming, Bryn is a maybe, Cal can't, Dee and Eli haven't
	// answered, and Eli left himself out of the count.
	atts := []Attendee{
		{UserID: "u1", Status: RSVPAccepted},
		{UserID: "u2", Status: RSVPTentative},
		{UserID: "u3", Status: RSVPDeclined},
		{UserID: "u5", Status: RSVPInvited, ExcludedFromCount: true},
	}
	tests := []struct {
		name     string
		date     string
		target   string
		today    string
		want     []string
		skipped  int
		errPart  string
		notified []string
	}{
		{name: "everyone who hasn't answered", date: "2026-11-07", today: "2026-10-12", want: []string{"Dee"}, skipped: 3, notified: []string{"u4"}},
		{name: "one player with no answer", date: "2026-11-07", target: "u4", today: "2026-10-12", want: []string{"Dee"}, notified: []string{"u4"}},
		{name: "a maybe can be asked to decide", date: "2026-11-07", target: "u2", today: "2026-10-12", want: []string{"Bryn"}, notified: []string{"u2"}},
		{name: "a firm answer isn't chased", date: "2026-11-07", target: "u3", today: "2026-10-12", errPart: "already answered"},
		{name: "not yourself", date: "2026-11-07", target: "dm", today: "2026-10-12", errPart: "yourself"},
		{name: "someone not on the night", date: "2026-11-07", target: "nobody", today: "2026-10-12", errPart: "isn't on"},
		{name: "a night already played", date: "2026-11-07", today: "2026-11-08", errPart: "already happened"},
		{name: "a date the night isn't on", date: "2026-11-08", today: "2026-10-12", errPart: "isn't on that date"},
		{name: "a malformed date", date: "Nov 7", today: "2026-10-12", errPart: "YYYY-MM-DD"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var notified []string
			var link, msg string
			repo := &mockSessionRepo{
				listByDateRangeFn: func(_ context.Context, _, _, _ string) ([]Session, error) {
					return []Session{{ID: "s1", CampaignID: "c1", Name: "The Ashen Road", ScheduledDate: strp("2026-11-07"), ScheduledTime: strp("19:00"), CreatedBy: "dm"}}, nil
				},
				listAttendeesFn: func(_ context.Context, _ string) ([]Attendee, error) { return atts, nil },
				createNotificationFn: func(_ context.Context, n *Notification) error {
					notified = append(notified, n.UserID)
					link = *n.Link
					msg = *n.Payload
					return nil
				},
			}
			rem, err := newTestSessionService(repo).RemindNight(context.Background(), "c1", "s1", tt.date, "dm", "Echo", tt.target, tt.today, members)
			if tt.errPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errPart) {
					t.Fatalf("err = %v, want one saying %q", err, tt.errPart)
				}
				if len(notified) != 0 {
					t.Fatalf("a refused reminder notified %v", notified)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(rem.Result.Notified, tt.want) || rem.Result.Skipped != tt.skipped {
				t.Fatalf("result = %+v, want %v skipped %d", rem.Result, tt.want, tt.skipped)
			}
			sort.Strings(notified)
			if !reflect.DeepEqual(notified, tt.notified) || !reflect.DeepEqual(rem.UserIDs, tt.notified) {
				t.Fatalf("notified %v (ids %v), want %v", notified, rem.UserIDs, tt.notified)
			}
			if link != "/campaigns/c1/game-nights?date=2026-11-07&session=s1" {
				t.Fatalf("link = %q", link)
			}
			if !strings.Contains(msg, "Echo asked you to say if you") || !strings.Contains(msg, "Sat Nov 7 at 7:00 PM") {
				t.Fatalf("payload = %s", msg)
			}
		})
	}
}

func TestRemindNightOneOccurrenceOfASeries(t *testing.T) {
	members := []NightMember{{"dm", "Echo"}, {"u1", "Ana"}, {"u2", "Bryn"}}
	repo := &mockSessionRepo{
		listByDateRangeFn: func(_ context.Context, _, _, _ string) ([]Session, error) {
			return []Session{{ID: "s1", CampaignID: "c1", Name: "Weekly", ScheduledDate: strp("2026-10-01"), IsRecurring: true, RecurrenceType: strp(RecurrenceWeekly), CreatedBy: "dm"}}, nil
		},
		// Ana answered this night; Bryn answered only another one.
		listOccurrenceRSVPsFn: func(_ context.Context, _, date string) ([]OccurrenceRSVP, error) {
			if date == "2026-10-15" {
				return []OccurrenceRSVP{{UserID: "u1", Status: RSVPAccepted}}, nil
			}
			return []OccurrenceRSVP{{UserID: "u2", Status: RSVPAccepted}}, nil
		},
	}
	rem, err := newTestSessionService(repo).RemindNight(context.Background(), "c1", "s1", "2026-10-15", "dm", "Echo", "", "2026-10-12", members)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rem.Result.Notified, []string{"Bryn"}) || !rem.Night.Recurring {
		t.Fatalf("got %+v recurring=%v; each night of a series is answered on its own", rem.Result, rem.Night.Recurring)
	}
}
