// Pins that an anonymous request (userID == "") on a public campaign gets the
// same per-user visibility filtering as a logged-in Player, never a bypass
// (ADR-049). Each anonymous assertion is paired with a Player control so a
// "fix" that just hides everything would still fail.
package timeline

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// anonViewer is the logged-out visitor: no membership role, no user id.
func anonViewer() permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleNone), "")
}

func strPtr(s string) *string { return &s }

// TestAnonymous_PublicCampaign_SeesNoRestrictedTimeline covers both shapes:
// the dm_only timeline (the repo's SQL narrowing is the first term, this
// filter the second — and the second was a no-op for anonymous) and the
// allow-list-restricted one, which SQL cannot express at all.
func TestAnonymous_PublicCampaign_SeesNoRestrictedTimeline(t *testing.T) {
	rows := []Timeline{
		{ID: "pub", Name: "Public", Visibility: "everyone"},
		{ID: "secret", Name: "Secret", Visibility: "dm_only"},
		{ID: "for-u1", Name: "For u1", Visibility: "everyone", VisibilityRules: strPtr(`{"allowed_users":["u1"]}`)},
	}
	repo := &mockTimelineRepo{
		listFn: func(_ context.Context, _ string, _ int) ([]Timeline, error) {
			return append([]Timeline(nil), rows...), nil
		},
	}
	svc := newTestTimelineService(repo)
	ctx := context.Background()

	anon, err := svc.ListTimelines(ctx, "camp-1", anonViewer())
	if err != nil {
		t.Fatalf("ListTimelines(anonymous): %v", err)
	}
	if len(anon) != 1 || anon[0].ID != "pub" {
		t.Errorf("anonymous saw %+v; want only the plain 'everyone' timeline", anon)
	}

	// Control: the whitelisted player sees their restricted timeline and still
	// not the dm_only one.
	u1, err := svc.ListTimelines(ctx, "camp-1", permissions.RequestViewer(permissions.RolePlayer, "u1"))
	if err != nil {
		t.Fatalf("ListTimelines(u1): %v", err)
	}
	if len(u1) != 2 {
		t.Errorf("player u1 saw %d timelines; want 2 (public + the one whitelisted to them)", len(u1))
	}
	for _, tl := range u1 {
		if tl.ID == "secret" {
			t.Error("player u1 must not see the dm_only timeline")
		}
	}

	// Control: the owner is unaffected.
	owner, err := svc.ListTimelines(ctx, "camp-1", permissions.RequestViewer(permissions.RoleOwner, "u1"))
	if err != nil {
		t.Fatalf("ListTimelines(owner): %v", err)
	}
	if len(owner) != 3 {
		t.Errorf("owner saw %d timelines; want all 3", len(owner))
	}

	// The declared SYSTEM caller — the widget picker's path — keeps the whole
	// list at a non-owner role. That is the state the empty-userID sentinel
	// used to stand for, now said out loud instead of shared with anonymous.
	sys, err := svc.ListTimelines(ctx, "camp-1", permissions.SystemViewer(permissions.RolePlayer))
	if err != nil {
		t.Fatalf("ListTimelines(system): %v", err)
	}
	if len(sys) != 3 {
		t.Errorf("system caller saw %d timelines; want all 3 (the picker's shipped behaviour)", len(sys))
	}
}

// TestAnonymous_PublicCampaign_SeesNoRestrictedEventLink is the same claim one
// layer down, on the merged event stream a public timeline page renders.
func TestAnonymous_PublicCampaign_SeesNoRestrictedEventLink(t *testing.T) {
	links := []EventLink{
		{EventID: "pub", EventVisibility: "everyone", EventYear: 1},
		{EventID: "secret", EventVisibility: "dm_only", EventYear: 2},
		{EventID: "for-u1", EventVisibility: "everyone", EventYear: 3,
			VisibilityRules: strPtr(`{"allowed_users":["u1"]}`)},
	}
	repo := &mockTimelineRepo{
		listEventLinksFn: func(_ context.Context, _ string, _ int) ([]EventLink, error) {
			return append([]EventLink(nil), links...), nil
		},
		listStandaloneEventsFn: func(_ context.Context, _ string, _ int) ([]TimelineEvent, error) {
			return nil, nil
		},
	}
	svc := newTestTimelineService(repo)
	ctx := context.Background()

	anon, err := svc.ListTimelineEvents(ctx, "tl-1", "camp-1", anonViewer())
	if err != nil {
		t.Fatalf("ListTimelineEvents(anonymous): %v", err)
	}
	if len(anon) != 1 || anon[0].EventID != "pub" {
		t.Errorf("anonymous saw %+v; want only the plain 'everyone' event link", anon)
	}

	// Control: the whitelisted player keeps their event.
	u1, err := svc.ListTimelineEvents(ctx, "tl-1", "camp-1", permissions.RequestViewer(permissions.RolePlayer, "u1"))
	if err != nil {
		t.Fatalf("ListTimelineEvents(u1): %v", err)
	}
	if len(u1) != 2 {
		t.Errorf("player u1 saw %d event links; want 2", len(u1))
	}
	for _, el := range u1 {
		if el.EventID == "secret" {
			t.Error("player u1 must not see the dm_only event link")
		}
	}

	// Control: the owner sees all three.
	owner, err := svc.ListTimelineEvents(ctx, "tl-1", "camp-1", permissions.RequestViewer(permissions.RoleOwner, "u1"))
	if err != nil {
		t.Fatalf("ListTimelineEvents(owner): %v", err)
	}
	if len(owner) != 3 {
		t.Errorf("owner saw %d event links; want all 3", len(owner))
	}
}

// TestCanUserView_DeniedUsersExcludesAnonymous is the deny-list half of
// ADR-049: an allow list already excludes an unnamed anonymous viewer by
// its default-deny shape, but a deny list defaults to allowing anyone NOT
// named — and an anonymous viewer's empty userID never matches a real
// denied id, so without this rule they'd see what the denied player can't.
func TestCanUserView_DeniedUsersExcludesAnonymous(t *testing.T) {
	denyRules := `{"denied_users":["u-bryn"]}`

	if canUserView("everyone", &denyRules, int(permissions.RoleNone), "") {
		t.Error("anonymous viewer must be denied when the item carries a non-empty deny list")
	}
	// Control: an authenticated non-member (also not on the deny list) still sees it —
	// the falsifiable invariant is "anonymous == authenticated non-member", not
	// "anonymous sees strictly less".
	if !canUserView("everyone", &denyRules, int(permissions.RoleNone), "u-someone-else") {
		t.Error("an authenticated user not on the deny list must still see the item")
	}
	// Control: the actually-denied player is still denied.
	if canUserView("everyone", &denyRules, int(permissions.RolePlayer), "u-bryn") {
		t.Error("the specifically denied player must stay denied")
	}
	// Control: no deny list at all leaves anonymous unaffected.
	noRules := `{}`
	if !canUserView("everyone", &noRules, int(permissions.RoleNone), "") {
		t.Error("an item with no visibility_rules must stay visible to anonymous")
	}
}
