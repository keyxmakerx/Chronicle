package sessions

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The bell leaves finished asks out of its count by reading the member's
// unread asks: only theirs, only that kind, only unread, newest first, and
// no more than it was told to read.
func TestDB_ListUnreadNotificationsOfType(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	repo := NewSessionRepository(db)
	campID, me := seedCampaign(t, db)
	other := seedUser(t, db, "Other")
	now := time.Now().UTC().Truncate(time.Second)

	add := func(id, userID, ntype string, age time.Duration, read bool) {
		t.Helper()
		link := sessionLink(campID, id)
		n := &Notification{ID: id, UserID: userID, CampaignID: &campID, Type: ntype, Link: &link, CreatedAt: now.Add(-age)}
		if read {
			n.ReadAt = &now
		}
		if err := repo.CreateNotification(ctx, n); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	add("older", me, NotifSessionScheduled, 3*time.Hour, false)
	add("newer", me, NotifSessionScheduled, time.Hour, false)
	add("answered", me, NotifSessionScheduled, 2*time.Hour, true)
	add("news", me, NotifProposalConfirmed, time.Minute, false)
	add("someone-elses", other, NotifSessionScheduled, time.Minute, false)

	ids := func(limit int) string {
		t.Helper()
		got, err := repo.ListUnreadNotificationsOfType(ctx, me, NotifSessionScheduled, limit)
		if err != nil {
			t.Fatalf("ListUnreadNotificationsOfType: %v", err)
		}
		out := make([]string, 0, len(got))
		for _, n := range got {
			if n.UserID != me || n.Type != NotifSessionScheduled || n.ReadAt != nil || n.Link == nil {
				t.Errorf("row %s is not one of this member's unread asks: %+v", n.ID, n)
			}
			out = append(out, n.ID)
		}
		return strings.Join(out, ",")
	}
	if got := ids(100); got != "newer,older" {
		t.Errorf("unread asks = %q, want newer,older", got)
	}
	if got := ids(1); got != "newer" {
		t.Errorf("with a limit of 1 = %q, want the newest only", got)
	}
}
