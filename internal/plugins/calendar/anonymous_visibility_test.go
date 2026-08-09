// anonymous_visibility_test.go — C-AUTHZ-EMPTY-USERID / ADR-049.
//
// THE BUG THESE PIN. The calendar visibility filters treated `userID == ""` as
// "trusted system caller, skip the per-user layer". A logged-out visitor to a
// PUBLIC campaign carries exactly that value (role RoleNone, no user id), so
// the most privileged path in the filter was the one anonymous traffic took.
//
// Every assertion below is written from the PUBLIC-CAMPAIGN viewpoint: the
// campaign is reachable without a session, and the question is what the wire
// hands back to somebody who never logged in. Each anonymous assertion is
// paired with a Player control — a fix that hides everything from everyone
// would pass the first half and fail the second, which is the point.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// anonViewer is the logged-out visitor: no membership role, no user id.
func anonViewer() permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleNone), "")
}

// TestAnonymous_PublicCampaign_SeesNoDmOnlyCalendar — the calendar-level half.
// ListVisibleCalendars is the list the Calendars dashboard and the v2 shell
// both build from; before the fix the anonymous branch returned it unfiltered,
// so a dm_only calendar's NAME and grid were readable by id.
func TestAnonymous_PublicCampaign_SeesNoDmOnlyCalendar(t *testing.T) {
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "open", CampaignID: campaignID, Visibility: "everyone"},
				{ID: "secret", CampaignID: campaignID, Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestCalendarService(repo)

	anon, err := svc.ListVisibleCalendars(context.Background(), "camp", int(permissions.RoleNone), "")
	if err != nil {
		t.Fatalf("ListVisibleCalendars(anonymous): %v", err)
	}
	if len(anon) != 1 || anon[0].ID != "open" {
		t.Errorf("anonymous visible = %v; want only [open] — a dm_only calendar must never reach a logged-out visitor", ids(anon))
	}

	// Control 1: the Player still sees exactly what they should.
	player, err := svc.ListVisibleCalendars(context.Background(), "camp", permissions.RolePlayer, "u1")
	if err != nil {
		t.Fatalf("ListVisibleCalendars(player): %v", err)
	}
	if len(player) != 1 || player[0].ID != "open" {
		t.Errorf("player visible = %v; want [open]", ids(player))
	}

	// Control 2: the Owner is unaffected — the fix narrows the anonymous path,
	// it does not break the product.
	owner, err := svc.ListVisibleCalendars(context.Background(), "camp", permissions.RoleOwner, "u1")
	if err != nil {
		t.Fatalf("ListVisibleCalendars(owner): %v", err)
	}
	if len(owner) != 2 {
		t.Errorf("owner visible = %d; want both calendars", len(owner))
	}
}

// TestAnonymous_PublicCampaign_SeesNoRestrictedEvent — the event-level half,
// both shapes.
//
//   - The dm_only EVENT has two terms in production: the repository's SQL
//     `visibility = 'everyone'` narrowing (first term) and filterEventsByUser
//     (second). The mock deliberately hands back the dm_only row so this pins
//     the SECOND term, which is the one that was a no-op for anonymous.
//   - The per-user-restricted event is the case SQL CANNOT express — an
//     `everyone` row with an allowed_users whitelist. Before the fix it was
//     served to anonymous in full; after, an empty user id is on nobody's
//     allow-list and it is dropped.
func TestAnonymous_PublicCampaign_SeesNoRestrictedEvent(t *testing.T) {
	events := []Event{
		{ID: "public", Visibility: "everyone"},
		{ID: "secret", Visibility: "dm_only"},
		{ID: "for-u1", Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)},
	}
	fresh := func() []Event { return append([]Event(nil), events...) }

	anon := filterEventsByUser(fresh(), anonViewer())
	for _, e := range anon {
		if e.ID != "public" {
			t.Errorf("anonymous got event %q; a logged-out visitor may only see the plain 'everyone' event", e.ID)
		}
	}
	if len(anon) != 1 {
		t.Errorf("anonymous saw %d events; want exactly 1 (the public one)", len(anon))
	}

	// Control: the whitelisted Player still gets their event, and still does
	// not get the dm_only one.
	u1 := filterEventsByUser(fresh(), permissions.RequestViewer(permissions.RolePlayer, "u1"))
	if len(u1) != 2 {
		t.Errorf("player u1 saw %d events; want 2 (public + the one whitelisted to them)", len(u1))
	}
	for _, e := range u1 {
		if e.ID == "secret" {
			t.Error("player u1 must not see the dm_only event")
		}
	}

	// Control: a different player is NOT on the whitelist and sees one event.
	u2 := filterEventsByUser(fresh(), permissions.RequestViewer(permissions.RolePlayer, "u2"))
	if len(u2) != 1 || u2[0].ID != "public" {
		t.Errorf("player u2 saw %v; want only the public event", u2)
	}

	// Control: the Owner sees all three — the bypass that is SUPPOSED to exist.
	owner := filterEventsByUser(fresh(), permissions.RequestViewer(permissions.RoleOwner, "u1"))
	if len(owner) != 3 {
		t.Errorf("owner saw %d events; want all 3", len(owner))
	}
}

// TestAnonymous_CannotBecomeASystemViewer is the structural half of the fix:
// the two states no longer share a representation, and the trusted one cannot
// be reached from request data. If a future edit re-adds a request-derived
// constructor that can set the system bit, this fails.
func TestAnonymous_CannotBecomeASystemViewer(t *testing.T) {
	anon := anonViewer()
	if anon.IsSystem() {
		t.Error("a request-derived viewer with an empty user id must never be a system viewer")
	}
	if !anon.IsAnonymous() {
		t.Error("a request-derived viewer with an empty user id IS the anonymous one")
	}
	if anon.SkipsPerUserRules() {
		t.Error("the anonymous viewer must take the per-user filter, not bypass it")
	}

	// And the declared system caller still bypasses — the state the sentinel
	// was meant to stand for, kept rather than deleted.
	sys := permissions.SystemViewer(permissions.RolePlayer)
	if !sys.IsSystem() || !sys.SkipsPerUserRules() {
		t.Error("a declared system viewer must still bypass the per-user layer")
	}
	if sys.IsAnonymous() {
		t.Error("a system viewer is not anonymous, even though it has no user id")
	}
}
