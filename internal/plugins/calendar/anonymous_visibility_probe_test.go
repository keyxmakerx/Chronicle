package calendar

// anonymous_visibility_probe_test.go — C-CALV4-GAMEREADY §10 [GR-19], WORLD 1.
//
// §10 IS A VERIFICATION, NOT A FIX. The audit measured a logged-out visitor to
// a PUBLIC campaign out-seeing a logged-in Player, and named four functions:
// calendarVisibleTo, filterCalendarsByUser, filterEventsByUser and
// EventsForEntityFiltered. All four were already converted to
// permissions.Viewer / SkipsPerUserRules on this branch, ADR-049 is recorded,
// and anonymous_visibility_test.go already pins the calendar and event halves
// with paired Player controls. Nothing here re-fixes any of that, and
// anonymous_visibility_test.go is untouched.
//
// WHAT THIS FILE ADDS is the one arm the shipped suite does not run as a
// four-viewer sweep: the audit's own probe over ENTITY TIES, the surface where
// the leak was most visible (`SECRET: the traitor is Bryn`,
// `HIDDEN CALENDAR EVENT`, badge-labelled "DM only" in the rendered entity
// block). It re-runs the exact four viewers the audit used and asserts the
// exact four numbers it published as the FIXED expectation:
//
//	anonymous public visitor   role=RoleNone user=""            -> 1 tie
//	authenticated NON-member   role=RoleNone user="u-stranger"  -> 1 tie
//	player                     role=RolePlayer user="u-bryn"    -> 1 tie
//	owner                      role=RoleOwner  user="u-kael"    -> 3 ties
//
// THE PAIRING IS THE POINT, exactly as the shipped suite says: a fix that hid
// everything from everyone would pass the anonymous row and fail the owner one.

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// probeTieRepo serves the audit's fixture: one public tie, one dm_only tie, and
// one tie into a calendar the viewer may not see at all — the three rows the
// probe measured leaking to anonymous.
func probeTieRepo() *mockCalendarRepo {
	return &mockCalendarRepo{
		eventsForEntityFn: func(_ context.Context, _ string) ([]EntityEventTie, error) {
			return []EntityEventTie{
				{Event: Event{ID: "e-public", CalendarID: "cal-open", Name: "Public Siege", Visibility: "everyone"}},
				{Event: Event{ID: "e-secret", CalendarID: "cal-open", Name: "SECRET: the traitor is Bryn", Visibility: "dm_only"}},
				{Event: Event{ID: "e-hidden", CalendarID: "cal-gm", Name: "HIDDEN CALENDAR EVENT", Visibility: "everyone"}},
			}, nil
		},
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			switch id {
			case "cal-open":
				return &Calendar{ID: "cal-open", CampaignID: "camp", Visibility: "everyone"}, nil
			case "cal-gm":
				return &Calendar{ID: "cal-gm", CampaignID: "camp", Visibility: "dm_only"}, nil
			}
			return nil, nil
		},
	}
}

// TestAnonymousProbe_FourViewersOverEntityTies re-runs the audit's probe and
// logs its four numbers, so the verification produces evidence rather than an
// assurance.
func TestAnonymousProbe_FourViewersOverEntityTies(t *testing.T) {
	svc := newTestCalendarService(probeTieRepo())
	ctx := context.Background()

	for _, v := range []struct {
		label  string
		role   int
		userID string
		want   int
	}{
		{"anonymous public visitor", int(permissions.RoleNone), "", 1},
		{"authenticated non-member", int(permissions.RoleNone), "u-stranger", 1},
		{"player", permissions.RolePlayer, "u-bryn", 1},
		{"owner", permissions.RoleOwner, "u-kael", 3},
	} {
		ties, err := svc.EventsForEntityFiltered(ctx, "ent-1", v.role, v.userID)
		if err != nil {
			t.Fatalf("EventsForEntityFiltered(%s): %v", v.label, err)
		}
		names := make([]string, 0, len(ties))
		for _, tie := range ties {
			names = append(names, tie.Event.Name)
		}
		t.Logf("%-26s role=%d user=%-12q -> %d tie(s): %v", v.label, v.role, v.userID, len(ties), names)
		if len(ties) != v.want {
			t.Errorf("%s saw %d ties (%v); want %d", v.label, len(ties), names, v.want)
		}
		// THE CONFIDENTIALITY ASSERTION, stated by NAME rather than by count,
		// so a fixture change cannot quietly turn this into arithmetic about
		// nothing.
		if v.role < permissions.RoleScribe {
			for _, n := range names {
				if n != "Public Siege" {
					t.Errorf("%s was served %q — a below-DM viewer may see only the public tie", v.label, n)
				}
			}
		}
	}
}

// TestAnonymousProbe_AnonymousMatchesANonMember states the audit's finding as
// the invariant it actually is, on all three surfaces the probe covered:
//
//	AN EMPTY USER ID MUST BEHAVE EXACTLY AS A REAL NON-MEMBER'S USER ID DOES.
//
// That is ADR-049 in one sentence, and it is the falsifiable form. It is
// written as an EQUIVALENCE rather than as "anonymous ⊆ player" because the
// subset form is not actually true and a guard that asserted it would be
// wrong rather than strict:
//
// MEASURED WHILE WRITING THIS, AND DELIBERATELY NOT ESCALATED. On an
// `everyone` event carrying a `denied_users:["u-bryn"]` rule, a logged-out
// visitor receives the event and the PLAYER NAMED BRYN DOES NOT — anonymous
// legitimately outnumbers that one player, 2 events to 1. That is a targeted
// exclusion doing exactly what it says: the event is public minus one named
// person, and a visitor who is not that person is not excluded by it. It is
// NOT the ADR-049 class — the anonymous viewer is being evaluated by the
// per-user rules, not skipping them, which is the whole distinction the fix
// drew. An authenticated non-member sees the same 2. So this is not a fifth
// site, and it is recorded here rather than filed, because the next hand who
// runs a naive subset probe will see the same number and needs to know why.
func TestAnonymousProbe_AnonymousMatchesANonMember(t *testing.T) {
	ctx := context.Background()

	// Surface 1 — entity ties.
	tieSvc := newTestCalendarService(probeTieRepo())
	anonTies, err := tieSvc.EventsForEntityFiltered(ctx, "ent-1", int(permissions.RoleNone), "")
	if err != nil {
		t.Fatalf("ties(anonymous): %v", err)
	}
	strangerTies, err := tieSvc.EventsForEntityFiltered(ctx, "ent-1", int(permissions.RoleNone), "u-stranger")
	if err != nil {
		t.Fatalf("ties(non-member): %v", err)
	}
	if len(anonTies) != len(strangerTies) {
		t.Errorf("anonymous received %d ties against an authenticated non-member's %d — an "+
			"empty user id must be no more trusted than a real one (ADR-049)",
			len(anonTies), len(strangerTies))
	}
	// …and the Player control, so a fix that hid everything is still caught.
	playerTies, err := tieSvc.EventsForEntityFiltered(ctx, "ent-1", permissions.RolePlayer, "u-bryn")
	if err != nil {
		t.Fatalf("ties(player): %v", err)
	}
	if len(playerTies) != 1 {
		t.Errorf("the player control received %d ties; want 1", len(playerTies))
	}

	// Surface 2 — calendars.
	calRepo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "harptos", CampaignID: campaignID, Visibility: "everyone"},
				{ID: "gm-only", CampaignID: campaignID, Visibility: "dm_only"},
				{ID: "kael-only", CampaignID: campaignID, Visibility: "everyone",
					VisibilityRules: strptr(`{"allowed_users":["u-kael"]}`)},
			}, nil
		},
	}
	calSvc := newTestCalendarService(calRepo)
	anonCals, err := calSvc.ListVisibleCalendars(ctx, "camp", int(permissions.RoleNone), "")
	if err != nil {
		t.Fatalf("calendars(anonymous): %v", err)
	}
	strangerCals, err := calSvc.ListVisibleCalendars(ctx, "camp", int(permissions.RoleNone), "u-stranger")
	if err != nil {
		t.Fatalf("calendars(non-member): %v", err)
	}
	playerCals, err := calSvc.ListVisibleCalendars(ctx, "camp", permissions.RolePlayer, "u-bryn")
	if err != nil {
		t.Fatalf("calendars(player): %v", err)
	}
	t.Logf("calendars: anonymous %v · non-member %v · player %v",
		ids(anonCals), ids(strangerCals), ids(playerCals))
	if len(anonCals) != len(strangerCals) {
		t.Errorf("anonymous received %v against a non-member's %v", ids(anonCals), ids(strangerCals))
	}
	if len(playerCals) != 1 || playerCals[0].ID != "harptos" {
		t.Errorf("the player control received %v; want [harptos]", ids(playerCals))
	}

	// Surface 3 — per-user-restricted events, the shape SQL cannot express.
	events := []Event{
		{ID: "public", Visibility: "everyone"},
		{ID: "RESTRICTED-TO-KAEL", Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u-kael"]}`)},
		{ID: "DENIED-TO-BRYN", Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u-bryn"]}`)},
	}
	fresh := func() []Event { return append([]Event(nil), events...) }
	anonEvents := filterEventsByUser(fresh(), permissions.RequestViewer(int(permissions.RoleNone), ""))
	strangerEvents := filterEventsByUser(fresh(), permissions.RequestViewer(int(permissions.RoleNone), "u-stranger"))
	playerEvents := filterEventsByUser(fresh(), permissions.RequestViewer(permissions.RolePlayer, "u-bryn"))
	t.Logf("events: anonymous %d · non-member %d · player(bryn, denied) %d",
		len(anonEvents), len(strangerEvents), len(playerEvents))
	for _, e := range anonEvents {
		if e.ID == "RESTRICTED-TO-KAEL" {
			t.Error("anonymous was served an event whitelisted to one named user — the ADR-049 defect")
		}
	}
	if len(anonEvents) != len(strangerEvents) {
		t.Errorf("anonymous received %d events against an authenticated non-member's %d; an "+
			"empty user id must take the same path a real non-member's does",
			len(anonEvents), len(strangerEvents))
	}
	// The Player named in the deny rule sees FEWER than either. That is the
	// deny list working, not a bypass — see this test's header.
	if len(playerEvents) != 1 {
		t.Errorf("the denied player received %d events; want 1 (the plain public one)",
			len(playerEvents))
	}
}
