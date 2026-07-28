// app_dashboard_w5b_test.go — C-CAL-DASHBOARD-W5b: the per-calendar permissions
// write path. Validates persistence, level/rules validation, bulk-replace, and
// (at the gate level) that non-owners cannot reach the route.
package calendar

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestUpdateCalendarVisibility_PersistsLevelAndRules(t *testing.T) {
	var gotCal, gotVis string
	var gotRules *string
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Visibility: "everyone"}, nil
		},
		updateCalVisFn: func(_ context.Context, calID, vis string, rules *string) error {
			gotCal, gotVis, gotRules = calID, vis, rules
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	rules := `{"allowed_users":["u1","u2"]}`
	err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{
		Visibility: "everyone", VisibilityRules: &rules,
	})
	if err != nil {
		t.Fatalf("UpdateCalendarVisibility: %v", err)
	}
	if gotCal != "cal-1" || gotVis != "everyone" {
		t.Errorf("repo got (%q,%q); want (cal-1,everyone)", gotCal, gotVis)
	}
	if gotRules == nil || *gotRules != rules {
		t.Errorf("rules not bulk-replaced verbatim; got %v want %q", gotRules, rules)
	}
}

func TestUpdateCalendarVisibility_BulkReplaceClearsRules(t *testing.T) {
	var gotRules *string
	called := false
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1", Visibility: "everyone"}, nil
		},
		updateCalVisFn: func(_ context.Context, _, _ string, rules *string) error {
			called, gotRules = true, rules
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Switching to GM-only with no rules must write nil rules (bulk-replace),
	// not merge with any prior rule set.
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{
		Visibility: "dm_only", VisibilityRules: nil,
	}); err != nil {
		t.Fatalf("UpdateCalendarVisibility: %v", err)
	}
	if !called {
		t.Fatal("repo write not called")
	}
	if gotRules != nil {
		t.Errorf("rules should be cleared to nil on bulk-replace; got %v", *gotRules)
	}
}

func TestUpdateCalendarVisibility_Validation(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: "camp-1"}, nil
		},
		updateCalVisFn: func(_ context.Context, _, _ string, _ *string) error {
			t.Fatal("repo write must not be reached on a validation failure")
			return nil
		},
	}
	svc := newTestCalendarService(repo)

	// Bad base level.
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{Visibility: "bogus"}); err == nil {
		t.Error("invalid visibility level should error")
	}
	// Bad rules JSON.
	bad := `{not json`
	if err := svc.UpdateCalendarVisibility(context.Background(), "cal-1", UpdateCalendarVisibilityInput{Visibility: "everyone", VisibilityRules: &bad}); err == nil {
		t.Error("invalid visibility_rules JSON should error")
	}
}

func TestUpdateCalendarVisibility_NotFound(t *testing.T) {
	repo := &mockCalendarRepo{
		getByIDFn: func(_ context.Context, _ string) (*Calendar, error) { return nil, nil },
	}
	svc := newTestCalendarService(repo)
	err := svc.UpdateCalendarVisibility(context.Background(), "missing", UpdateCalendarVisibilityInput{Visibility: "everyone"})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("missing calendar should be NotFound; got %v", err)
	}
}

func TestCalVisModeForCard(t *testing.T) {
	rules := `{"allowed_users":["u1"]}`
	cases := []struct {
		cal  Calendar
		want string
	}{
		{Calendar{Visibility: "everyone"}, "public"},
		{Calendar{Visibility: "dm_only"}, "gmonly"},
		{Calendar{Visibility: "everyone", VisibilityRules: &rules}, "specific"},
		{Calendar{Visibility: "dm_only", VisibilityRules: &rules}, "gmonly"}, // dm_only wins
	}
	for _, c := range cases {
		if got := calVisModeForCard(c.cal); got != c.want {
			t.Errorf("calVisModeForCard(%+v) = %q; want %q", c.cal, got, c.want)
		}
	}
}

// benchW5bData is a one-calendar Bench whose single calendar is dm_only, so the
// permissions trigger's seeded state is exercised end to end.
//
// PIN REFRESH (C-CALV4-BENCH-P4): the route now renders the Bench, so these
// drive BenchPage. The W5b guarantee is unchanged and so is every marker —
// the Bench moved the per-calendar trigger from the card onto the row and the
// Block's management strip, and left the modal and its driver untouched.
func benchW5bData(isOwner bool) BenchData {
	cal := Calendar{ID: "c1", CampaignID: "camp-1", Name: "Secret", Visibility: "dm_only"}
	return BenchData{
		CampaignID: "camp-1", CampaignName: "Imix",
		IsGM: isOwner, IsOwner: isOwner, ShowNewSlot: isOwner,
		CalendarCount: 1,
		Rsvp:          benchRsvpPanel(),
		Rows:          benchRows([]*Calendar{&cal}, "", "camp-1", isOwner),
	}
}

// TestDashboard_OwnerGetsPermissionsEditor: the owner page ships the modal +
// the reused chip-row editor (with the GM-only mode) + the driver script, and
// the per-calendar trigger carries its current visibility state for the editor
// to seed.
func TestDashboard_OwnerGetsPermissionsEditor(t *testing.T) {
	html := renderBench(t, benchW5bData(true))
	for _, want := range []string{
		"cal-permissions-modal",      // the modal
		"data-visibility-editor",     // the reused Q-V2-7 widget
		`value="gmonly"`,             // the W5b GM-only mode
		"calendar_permissions.js",    // the driver
		`data-cal-vis-mode="gmonly"`, // the trigger seeds the editor with current state
		`data-cal-visibility="dm_only"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("owner bench missing %q", want)
		}
	}
}

// TestDashboard_PlayerNoPermissionsEditor: players never receive the editor DOM,
// the driver, or the per-calendar Permissions trigger.
func TestDashboard_PlayerNoPermissionsEditor(t *testing.T) {
	html := renderBench(t, benchW5bData(false))
	for _, forbidden := range []string{"cal-permissions-modal", "data-cal-permissions", "calendar_permissions.js", "data-visibility-editor"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("player must NOT receive %q", forbidden)
		}
	}
}

// TestCalendarVisibilityRouteGate documents the route gate (non-owner forbidden):
// the PUT uses CanControlWorldState — owner OR co-DM only, players excluded.
func TestCalendarVisibilityRouteGate(t *testing.T) {
	player := &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer}
	owner := &campaigns.CampaignContext{MemberRole: campaigns.RoleOwner}
	coDM := &campaigns.CampaignContext{MemberRole: campaigns.RolePlayer, IsDmGranted: true}
	if player.CanControlWorldState() {
		t.Error("a player must NOT pass the calendar-visibility gate")
	}
	if !owner.CanControlWorldState() || !coDM.CanControlWorldState() {
		t.Error("owner and co-DM must pass the calendar-visibility gate")
	}
}
