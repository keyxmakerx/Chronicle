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
		role   int
		userID string
		want   bool
	}{
		{"everyone → player sees", &Calendar{Visibility: "everyone"}, w5aRolePlayer, "u1", true},
		{"everyone (default) → player sees (no regression)", &Calendar{Visibility: "everyone"}, w5aRolePlayer, "u1", true},
		{"dm_only → player hidden", &Calendar{Visibility: "dm_only"}, w5aRolePlayer, "u1", false},
		{"dm_only → owner sees (bypass)", &Calendar{Visibility: "dm_only"}, w5aRoleOwner, "u1", true},
		{"dm_only + allow-list → player STILL hidden (dm_only is a hard gate)",
			&Calendar{Visibility: "dm_only", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aRolePlayer, "u1", false},
		{"everyone + allow-list → listed player sees",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aRolePlayer, "u1", true},
		{"everyone + allow-list → unlisted player hidden",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"allowed_users":["u1"]}`)}, w5aRolePlayer, "u2", false},
		{"everyone + deny-list → denied player hidden",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u2"]}`)}, w5aRolePlayer, "u2", false},
		{"everyone + deny-list → other player sees",
			&Calendar{Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u2"]}`)}, w5aRolePlayer, "u1", true},
		{"system context (empty userID) bypasses", &Calendar{Visibility: "dm_only"}, w5aRolePlayer, "", true},
		{"nil calendar → not visible", nil, w5aRoleOwner, "u1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := calendarVisibleTo(tc.cal, tc.role, tc.userID); got != tc.want {
				t.Errorf("calendarVisibleTo = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestFilterCalendarsByUser(t *testing.T) {
	cals := []Calendar{
		{ID: "open", Visibility: "everyone"},
		{ID: "secret", Visibility: "dm_only"},
		{ID: "denied", Visibility: "everyone", VisibilityRules: strptr(`{"denied_users":["u1"]}`)},
	}

	// Player u1: sees only the open calendar (secret is dm_only, denied excludes u1).
	got := filterCalendarsByUser(append([]Calendar(nil), cals...), w5aRolePlayer, "u1")
	if len(got) != 1 || got[0].ID != "open" {
		t.Errorf("player got %v; want only [open]", ids(got))
	}

	// Owner: sees all (bypass).
	gotOwner := filterCalendarsByUser(append([]Calendar(nil), cals...), w5aRoleOwner, "u1")
	if len(gotOwner) != 3 {
		t.Errorf("owner got %d calendars; want all 3", len(gotOwner))
	}

	// No-regression: an all-default ('everyone') set is fully visible to a player.
	allOpen := []Calendar{{ID: "a", Visibility: "everyone"}, {ID: "b", Visibility: "everyone"}}
	if got := filterCalendarsByUser(allOpen, w5aRolePlayer, "u1"); len(got) != 2 {
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
