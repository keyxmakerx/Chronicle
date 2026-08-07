// calendar_visibility_w5a_test.go — C-CAL-DASHBOARD-W5a: per-calendar
// visibility resolution. These pin the security logic of the gate wave: the
// resolver (calendarVisibleTo / filterCalendarsByUser) and the visibility-aware
// service methods (ListVisibleCalendars / GetActiveVisibleCalendar).
//
// SEMANTIC NOTE (mirrors events exactly, canUserView unchanged): `dm_only` is a
// HARD DM-gate — the allow-list does NOT admit a player into a dm_only calendar.
// To grant a specific player access to an otherwise-restricted calendar, use
// base `everyone` + an `allowed_users` whitelist; `denied_users` hides an
// `everyone` calendar from specific players. This is the event model verbatim.
package calendar

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

var (
	w5aRolePlayer = int(permissions.RolePlayer) // 1
	w5aRoleOwner  = int(permissions.RoleOwner)  // 3
)

func strptr(s string) *string { return &s }

func TestCalendarVisibleTo(t *testing.T) {
	tests := []struct {
		name   string
		cal    *Calendar
		viewer permissions.Viewer
		want   bool
	}{
		{"everyone → player sees", &Calendar{Visibility: "everyone"}, w5aPlayer("u1"), true},
		{"everyone (default) → player sees (no regression)", &Calendar{Visibility: "everyone"}, w5aPlayer("u1"), true},
		{"dm_only → player hidden", &Calendar{Visibility: "dm_only"}, w5aPlayer("u1"), false},
		{"dm_only → owner sees (bypass)", &Calendar{Visibility: "dm_only"}, w5aOwner("u1"), true},
		{"dm_only + allow-list → player STILL hidden (dm_only is a hard gate)",
			&Calendar{Visibility: "dm_only", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aPlayer("u1"), false},
		{"everyone + allow-list → listed player sees",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aPlayer("u1"), true},
		{"everyone + allow-list → unlisted player hidden",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aPlayer("u2"), false},
		{"everyone + deny-list → denied player hidden",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u2"]}`)}, w5aPlayer("u2"), false},
		{"everyone + deny-list → other player sees",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u2"]}`)}, w5aPlayer("u1"), true},

		// ── C-AUTHZ-EMPTY-USERID / ADR-049 — THE AMENDMENT ───────────────────
		// This row used to read `{"system context (empty userID) bypasses",
		// dm_only, w5aRolePlayer, "", true}` and it pinned the bug as intended:
		// an ANONYMOUS request carries exactly that empty user id, so a
		// logged-out visitor to a PUBLIC campaign was served every dm_only
		// calendar. It is INVERTED here rather than deleted, and the system
		// path it was meant to describe keeps its own row directly below — so
		// the pair proves the DISTINCTION instead of erasing it.
		{"ANONYMOUS (no user, RoleNone) → dm_only hidden", &Calendar{Visibility: "dm_only"}, w5aAnon(), false},
		{"ANONYMOUS (no user, but member-shaped role) → dm_only hidden",
			&Calendar{Visibility: "dm_only"}, permissions.RequestViewer(w5aRolePlayer, ""), false},
		{"ANONYMOUS → everyone + allow-list hidden (an empty id is not on any list)",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aAnon(), false},
		{"ANONYMOUS → plain everyone still visible (a public calendar is public)",
			&Calendar{Visibility: "everyone"}, w5aAnon(), true},
		{"SYSTEM caller (declared, non-owner role) still bypasses",
			&Calendar{Visibility: "dm_only"}, permissions.SystemViewer(w5aRolePlayer), true},

		{"nil calendar → not visible", nil, w5aOwner("u1"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := calendarVisibleTo(tc.cal, tc.viewer); got != tc.want {
				t.Errorf("calendarVisibleTo = %v; want %v", got, tc.want)
			}
		})
	}
}

// w5aPlayer / w5aOwner / w5aAnon name the three viewer shapes these tables use.
// w5aAnon is the one that did not exist before C-AUTHZ-EMPTY-USERID: a
// logged-out visitor on a public campaign — no user id AND no membership role.
func w5aPlayer(userID string) permissions.Viewer {
	return permissions.RequestViewer(w5aRolePlayer, userID)
}
func w5aOwner(userID string) permissions.Viewer {
	return permissions.RequestViewer(w5aRoleOwner, userID)
}
func w5aAnon() permissions.Viewer {
	return permissions.RequestViewer(int(permissions.RoleNone), "")
}

func TestFilterCalendarsByUser(t *testing.T) {
	cals := []Calendar{
		{ID: "open", Visibility: "everyone"},
		{ID: "secret", Visibility: "dm_only"},
		{ID: "denied", Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u1"]}`)},
	}

	// Player u1: sees only the open calendar (secret is dm_only, denied excludes u1).
	got := filterCalendarsByUser(append([]Calendar(nil), cals...), w5aPlayer("u1"))
	if len(got) != 1 || got[0].ID != "open" {
		t.Errorf("player got %v; want only [open]", ids(got))
	}

	// Owner: sees all (bypass).
	gotOwner := filterCalendarsByUser(append([]Calendar(nil), cals...), w5aOwner("u1"))
	if len(gotOwner) != 3 {
		t.Errorf("owner got %d calendars; want all 3", len(gotOwner))
	}

	// C-AUTHZ-EMPTY-USERID: the logged-out visitor gets the SAME narrowing as a
	// player — never the owner's list. ('denied' has no rule naming an empty id,
	// so it stays; 'secret' is dm_only and must go.)
	gotAnon := filterCalendarsByUser(append([]Calendar(nil), cals...), w5aAnon())
	for _, c := range gotAnon {
		if c.ID == "secret" {
			t.Errorf("anonymous got %v; a dm_only calendar must never be listed", ids(gotAnon))
		}
	}

	// The declared system caller keeps the whole list — that is the state the
	// empty-userID sentinel used to stand for, now said out loud.
	gotSystem := filterCalendarsByUser(append([]Calendar(nil), cals...), permissions.SystemViewer(w5aRolePlayer))
	if len(gotSystem) != 3 {
		t.Errorf("system caller got %d calendars; want all 3", len(gotSystem))
	}

	// No-regression: an all-default ('everyone') set is fully visible to a player.
	allOpen := []Calendar{{ID: "a", Visibility: "everyone"}, {ID: "b", Visibility: "everyone"}}
	if got := filterCalendarsByUser(allOpen, w5aPlayer("u1")); len(got) != 2 {
		t.Errorf("default-everyone set: player got %d; want 2 (no regression)", len(got))
	}
}

func TestListVisibleCalendars_FiltersForPlayerAllForOwner(t *testing.T) {
	repo := &mockCalendarRepo{
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			// Campaign-scoped: the repo returns only this campaign's calendars;
			// the filter only narrows, never reaches another campaign.
			return []Calendar{
				{ID: "open", CampaignID: campaignID, Visibility: "everyone"},
				{ID: "secret", CampaignID: campaignID, Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestCalendarService(repo)

	player, err := svc.ListVisibleCalendars(context.Background(), "camp", w5aRolePlayer, "u1")
	if err != nil {
		t.Fatalf("ListVisibleCalendars(player): %v", err)
	}
	if len(player) != 1 || player[0].ID != "open" {
		t.Errorf("player visible = %v; want [open]", ids(player))
	}

	owner, _ := svc.ListVisibleCalendars(context.Background(), "camp", w5aRoleOwner, "u1")
	if len(owner) != 2 {
		t.Errorf("owner visible = %d; want all 2", len(owner))
	}
}

func TestGetActiveVisibleCalendar_SkipsHiddenActive(t *testing.T) {
	repo := &mockCalendarRepo{
		// The viewer's active pointer points at a calendar hidden from players.
		getActiveCalendarIDFn: func(_ context.Context, _, _ string) (string, error) { return "secret", nil },
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp", Visibility: "dm_only"}, nil
		},
		listByCampaignIDFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "secret", CampaignID: campaignID, Visibility: "dm_only"},
				{ID: "open", CampaignID: campaignID, Visibility: "everyone"},
			}, nil
		},
	}
	svc := newTestCalendarService(repo)

	// Player: active is hidden → falls back to the first visible ('open').
	got, err := svc.GetActiveVisibleCalendar(context.Background(), "camp", w5aRolePlayer, "u1")
	if err != nil {
		t.Fatalf("GetActiveVisibleCalendar(player): %v", err)
	}
	if got == nil || got.ID != "open" {
		t.Errorf("player active = %v; want 'open' (hidden active skipped)", got)
	}

	// Owner: sees the active (hidden) calendar directly.
	gotOwner, _ := svc.GetActiveVisibleCalendar(context.Background(), "camp", w5aRoleOwner, "u1")
	if gotOwner == nil || gotOwner.ID != "secret" {
		t.Errorf("owner active = %v; want 'secret' (owner sees all)", gotOwner)
	}
}

// ids extracts calendar IDs for readable failure messages.
func ids(cals []Calendar) []string {
	out := make([]string, len(cals))
	for i, c := range cals {
		out[i] = c.ID
	}
	return out
}
