package sessions

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/notifyprefs"
)

// muteFilter drops the listed users for every category.
type muteFilter struct{ muted map[string]bool }

func (f muteFilter) AllowedRecipients(_ context.Context, ids []string, _ string, _ notifyprefs.Channel) []string {
	var out []string
	for _, id := range ids {
		if !f.muted[id] {
			out = append(out, id)
		}
	}
	return out
}

func TestBellWritesFollowChoices(t *testing.T) {
	tests := []struct {
		name  string
		ntype string
		want  []string
	}{
		{"category on the page is filtered", NotifSessionMoved, []string{"u1", "u3"}},
		{"availability ask is filtered", NotifAvailabilityNudge, []string{"u1", "u3"}},
		{"type with no category always goes out", "some_future_kind", []string{"u1", "u2", "u3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var to []string
			repo := &mockSessionRepo{createNotificationFn: func(_ context.Context, n *Notification) error {
				to = append(to, n.UserID)
				return nil
			}}
			svc := NewSessionService(repo, nil, nil)
			ConfigureRecipientFilter(svc, muteFilter{muted: map[string]bool{"u2": true}})
			if err := svc.NotifyUsers(context.Background(), []string{"u1", "u2", "u3"}, "c1", tt.ntype, "m", "/l"); err != nil {
				t.Fatal(err)
			}
			if len(to) != len(tt.want) {
				t.Fatalf("notified %v, want %v", to, tt.want)
			}
			for i := range to {
				if to[i] != tt.want[i] {
					t.Fatalf("notified %v, want %v", to, tt.want)
				}
			}
		})
	}
}

func TestEmailWanted(t *testing.T) {
	h := &Handler{}
	if !h.emailWanted(context.Background(), "u2", notifyprefs.GameNightInvites) {
		t.Error("with no filter wired every email should go out")
	}
	h.SetRecipientFilter(muteFilter{muted: map[string]bool{"u2": true}})
	if h.emailWanted(context.Background(), "u2", notifyprefs.GameNightInvites) {
		t.Error("muted member still emailed")
	}
	if !h.emailWanted(context.Background(), "u1", notifyprefs.GameNightInvites) {
		t.Error("member who wants it was dropped")
	}
}
