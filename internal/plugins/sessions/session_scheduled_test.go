package sessions

// A scheduled night reaches players in Chronicle itself, asks them for an
// answer through the banner, and stops asking once they have answered.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

func TestNotifySessionScheduled(t *testing.T) {
	var made []*Notification
	repo := &mockSessionRepo{
		createNotificationFn: func(_ context.Context, n *Notification) error { made = append(made, n); return nil },
	}
	date, clock := "2026-10-31", "19:00"
	s := &Session{ID: "s4", CampaignID: "c1", Name: "Session 4: The Lowest Tide", ScheduledDate: &date, ScheduledTime: &clock}
	if err := NewSessionService(repo, nil, nil).NotifySessionScheduled(context.Background(), s, "dm", "Mira", []string{"dm", "jules", "rowan"}); err != nil {
		t.Fatal(err)
	}
	if len(made) != 2 {
		t.Fatalf("notified %d people, want the two players and not the DM", len(made))
	}
	for _, n := range made {
		if n.UserID == "dm" {
			t.Error("the DM who scheduled it must not be told")
		}
		if n.Type != NotifSessionScheduled || n.Link == nil || *n.Link != "/campaigns/c1/sessions/s4" {
			t.Errorf("notification = %+v, want session_scheduled linked to the night", n)
		}
		if msg := notificationMessage(*n); msg != "Mira scheduled Session 4: The Lowest Tide. Are you coming?" {
			t.Errorf("message = %q", msg)
		}
		if d := notificationDetail(*n); !strings.Contains(d, "Oct 31") || !strings.Contains(d, "7:00 PM") {
			t.Errorf("detail = %q, want the date and time", d)
		}
	}
}

func TestScheduledNight_RaisesBannerUntilAnswered(t *testing.T) {
	link := "/campaigns/c1/sessions/s4"
	unread := []Notification{{Type: NotifSessionScheduled, Link: &link}}
	if got := BuildCallout(unread, true, time.Now()); got.Kind != CalloutRSVP || got.Link != link {
		t.Fatalf("callout = %+v, want an RSVP ask pointing at the night", got)
	}

	var cleared []string
	repo := &mockSessionRepo{
		markReadForLinkFn: func(_ context.Context, userID, ntype, l string) error {
			cleared = append(cleared, userID+"|"+ntype+"|"+l)
			return nil
		},
	}
	svc := NewSessionService(repo, nil, nil)
	if err := svc.RespondRSVP(context.Background(), &Session{ID: "s4", CampaignID: "c1", Status: StatusPlanned}, "jules", RSVPAccepted, nil, patch.Field[string]{}); err != nil {
		t.Fatal(err)
	}
	if len(cleared) != 1 || cleared[0] != "jules|session_scheduled|"+link {
		t.Errorf("answering must clear that night's ask, got %v", cleared)
	}
}
