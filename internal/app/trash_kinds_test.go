package app

import (
	"context"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/sessions"
)

type restoreStub struct{ sessions.SessionService }

func (restoreStub) RestoreAndAnnounce(_ context.Context, _, id string) (*sessions.Session, error) {
	return &sessions.Session{ID: id}, nil
}

// A game night put back from the campaign Trash sends the same back-on email
// as the night's own Restore button.
func TestGameNightTrash_RestoreEmailsBackOn(t *testing.T) {
	got := make(chan string, 1)
	g := gameNightTrash{svc: restoreStub{}, backOn: func(_ context.Context, campaignID string, s *sessions.Session) {
		got <- campaignID + "/" + s.ID
	}}
	link, err := g.RestoreTrashed(context.Background(), "c1", "s1")
	if err != nil || link != "/campaigns/c1/sessions/s1" {
		t.Fatalf("RestoreTrashed = %q, %v", link, err)
	}
	select {
	case v := <-got:
		if v != "c1/s1" {
			t.Errorf("back-on email for %q, want c1/s1", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no back-on email was sent")
	}
	if _, err := (gameNightTrash{svc: restoreStub{}}).RestoreTrashed(context.Background(), "c1", "s1"); err != nil {
		t.Errorf("without an emailer the restore still works: %v", err)
	}
}
