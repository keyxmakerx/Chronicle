package sessions

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// Who hears by email that a night changed: Going or Maybe from either answer
// store, once each, still readable after the night is deleted.
func TestDB_ListGoingOrMaybeUserIDs(t *testing.T) {
	db := newScratchDB(t)
	ctx := context.Background()
	repo := NewSessionRepository(db)
	campID, owner := seedCampaign(t, db)
	sid := seedSession(t, db, campID, owner, "Night")
	going, maybe, cant, invited, nightOnly, both :=
		seedUser(t, db, "Going"), seedUser(t, db, "Maybe"), seedUser(t, db, "Cant"),
		seedUser(t, db, "Invited"), seedUser(t, db, "NightOnly"), seedUser(t, db, "Both")
	for id, status := range map[string]string{going: RSVPAccepted, maybe: RSVPTentative, cant: RSVPDeclined, invited: RSVPInvited, both: RSVPAccepted} {
		if err := repo.AddAttendee(ctx, sid, id, status); err != nil {
			t.Fatalf("add attendee: %v", err)
		}
	}
	for id, status := range map[string]string{nightOnly: RSVPTentative, both: RSVPAccepted, cant: RSVPDeclined} {
		if err := repo.UpsertOccurrenceRSVP(ctx, sid, id, "2026-11-07", status, nil); err != nil {
			t.Fatalf("upsert occurrence rsvp: %v", err)
		}
	}
	if err := repo.SoftDeleteSession(ctx, sid); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	got, err := repo.ListGoingOrMaybeUserIDs(ctx, sid)
	if err != nil {
		t.Fatalf("ListGoingOrMaybeUserIDs: %v", err)
	}
	want := []string{going, maybe, nightOnly, both}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}
