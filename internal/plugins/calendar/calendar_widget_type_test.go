package calendar

// calendar_widget_type_test.go pins the two restored entity-page blocks'
// player protection: both RenderBlock methods read through
// GetCalendarForViewer/GetDefaultCalendarForViewer/ListUpcomingEvents —
// never a raw repo fetch — so a Player bound to a dm_only calendar sees the
// same "nothing here" a missing calendar would show, exactly like the
// calendar's own pages, and a dm_only EVENT on an otherwise-visible calendar
// stays out of the "calendar" block's upcoming-events list the same way it
// stays out of the dashboard card.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/widgetbindings"
)

func testBlockCC(campaignID string) *campaigns.CampaignContext {
	return &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: campaignID}}
}

// renderComponentToString renders a templ.Component for assertions on its
// output, mirroring internal/app's own renderToString test helper.
func renderComponentToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

// --- calendarWidgetType.RenderBlock ---

func TestCalendarWidgetType_RenderBlock_DmOnlyCalendarHiddenFromPlayer(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "dm_only"}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listUpcomingFn: func(_ context.Context, _ string, _, _, _, _, _ int) ([]Event, error) {
			t.Fatal("must not read events for a calendar this viewer cannot see")
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)
	wt := NewCalendarWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{
		CC: testBlockCC(testCampaignA), HostID: "ent-1", UserID: "u-1",
		Role:       int(permissions.RolePlayer),
		Resolution: widgetbindings.Resolution{InstanceID: "cal-dm", Source: widgetbindings.SourceOwn, WidgetType: WidgetTypeCalendar},
	}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if strings.Contains(got, "No calendar has been set up yet.") {
		// fine — this is the honest empty state
	} else if strings.Contains(got, "Nothing upcoming") || strings.Contains(got, "cal-dm") {
		t.Errorf("a dm_only calendar must not be readable to a Player, got %q", got)
	}
}

func TestCalendarWidgetType_RenderBlock_OwnerSeesDmOnlyCalendar(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "dm_only"}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listUpcomingFn: func(_ context.Context, _ string, _, _, _, role, _ int) ([]Event, error) {
			return []Event{{ID: "evt-1", CalendarID: "cal-dm", Name: "Secret Council", Visibility: "everyone"}}, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)
	wt := NewCalendarWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{
		CC: testBlockCC(testCampaignA), HostID: "ent-1", UserID: "u-owner",
		Role:       int(permissions.RoleOwner),
		Resolution: widgetbindings.Resolution{InstanceID: "cal-dm", Source: widgetbindings.SourceOwn, WidgetType: WidgetTypeCalendar},
	}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if !strings.Contains(got, "Secret Council") {
		t.Errorf("an Owner bound to a dm_only calendar must see its events, got %q", got)
	}
}

func TestCalendarWidgetType_RenderBlock_DmOnlyEventHiddenFromPlayer(t *testing.T) {
	// The bound calendar itself is visible to everyone; ListUpcomingEvents
	// (unchanged, existing code) is what must keep the dm_only event out —
	// this test pins that the block's wiring does not bypass it.
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "everyone"}, nil
		},
	}
	eventRepo := &fakeEventRepo{
		listUpcomingFn: func(_ context.Context, _ string, _, _, _, role, _ int) ([]Event, error) {
			// Mirrors the repository's own SQL role filter (dm_only excluded
			// below Owner) — service.go's ListUpcomingEvents passes v.Role() straight through.
			if permissions.CanSeeDmOnly(role) {
				return []Event{{ID: "evt-secret", Name: "Secret Council", Visibility: "dm_only"}}, nil
			}
			return nil, nil
		},
	}
	svc := newTestCalendarService(calRepo, eventRepo, nil, nil)
	wt := NewCalendarWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{
		CC: testBlockCC(testCampaignA), HostID: "ent-1", UserID: "u-1",
		Role:       int(permissions.RolePlayer),
		Resolution: widgetbindings.Resolution{InstanceID: "cal-open", Source: widgetbindings.SourceOwn, WidgetType: WidgetTypeCalendar},
	}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if strings.Contains(got, "Secret Council") {
		t.Errorf("a dm_only event must not reach a Player through the calendar block, got %q", got)
	}
}

func TestCalendarWidgetType_RenderBlock_Unresolved_RendersEmptyState(t *testing.T) {
	svc := newTestCalendarService(nil, nil, nil, nil)
	wt := NewCalendarWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{CC: testBlockCC(testCampaignA), Role: int(permissions.RolePlayer)}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if !strings.Contains(got, "No calendar has been set up yet.") {
		t.Errorf("an unresolved binding must render the same empty state as no calendar, got %q", got)
	}
}

// --- worldstateWidgetType.RenderBlock ---

func TestWorldstateWidgetType_RenderBlock_DmOnlyCalendarHiddenFromPlayer(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "dm_only"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewWorldstateWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{
		CC: testBlockCC(testCampaignA), HostID: "ent-1", UserID: "u-1",
		Role:       int(permissions.RolePlayer),
		Resolution: widgetbindings.Resolution{InstanceID: "cal-dm", Source: widgetbindings.SourceOwn, WidgetType: WidgetTypeWorldstate},
	}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if strings.Contains(got, `data-widget="sky-pane"`) {
		t.Errorf("a dm_only calendar's sky must not mount for a Player, got %q", got)
	}
	if !strings.Contains(got, "skypane-empty") {
		t.Errorf("expected the quiet empty placeholder, got %q", got)
	}
}

func TestWorldstateWidgetType_RenderBlock_OwnerSeesDmOnlyCalendar(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "dm_only"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewWorldstateWidgetType(svc)

	rc := widgetbindings.BlockRenderContext{
		CC: testBlockCC(testCampaignA), HostID: "ent-1", UserID: "u-owner",
		Role:       int(permissions.RoleOwner),
		Resolution: widgetbindings.Resolution{InstanceID: "cal-dm", Source: widgetbindings.SourceOwn, WidgetType: WidgetTypeWorldstate},
	}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if !strings.Contains(got, `data-calendar-id="cal-dm"`) {
		t.Errorf("an Owner bound to a dm_only calendar must see the sky mount, got %q", got)
	}
}

func TestWorldstateWidgetType_RenderBlock_UnresolvedFallsBackToCampaignDefault(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getDefaultFn: func(_ context.Context, campaignID string) (*Calendar, error) {
			return &Calendar{ID: "cal-default", CampaignID: campaignID, Visibility: "everyone"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewWorldstateWidgetType(svc)

	// No Resolution set — the dashboard-host shape (renderBoundBlock builds
	// no HostRef with no entity), same as skybox's own unbound path.
	rc := widgetbindings.BlockRenderContext{CC: testBlockCC(testCampaignA), Role: int(permissions.RolePlayer)}
	got := renderComponentToString(t, wt.RenderBlock(context.Background(), rc))

	if !strings.Contains(got, `data-calendar-id="cal-default"`) {
		t.Errorf("an unresolved binding must fall back to the campaign default calendar, got %q", got)
	}
}

// --- InstanceExists / DefaultInstance / ListInstances ---

func TestCalendarInstanceBase_InstanceExists(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getByIDFn: func(_ context.Context, id string) (*Calendar, error) {
			if id == "cal-1" {
				return &Calendar{ID: id, CampaignID: testCampaignA, Visibility: "dm_only"}, nil
			}
			return nil, apperror.NewNotFound("calendar not found")
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewCalendarWidgetType(svc)

	ok, err := wt.InstanceExists(context.Background(), testCampaignA, "cal-1")
	if err != nil || !ok {
		t.Errorf("InstanceExists(cal-1) = %v, %v; want true, nil (dm_only must still validate — it's an orphan/scope check, not a content read)", ok, err)
	}

	ok, err = wt.InstanceExists(context.Background(), "camp-other", "cal-1")
	if err != nil || ok {
		t.Errorf("InstanceExists in the wrong campaign = %v, %v; want false, nil", ok, err)
	}

	ok, err = wt.InstanceExists(context.Background(), testCampaignA, "cal-missing")
	if err != nil || ok {
		t.Errorf("InstanceExists for a missing calendar = %v, %v; want false, nil", ok, err)
	}
}

func TestCalendarInstanceBase_DefaultInstance(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		getDefaultFn: func(_ context.Context, campaignID string) (*Calendar, error) {
			return &Calendar{ID: "cal-default", CampaignID: campaignID, Visibility: "dm_only"}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewWorldstateWidgetType(svc)

	id, ok, err := wt.DefaultInstance(context.Background(), widgetbindings.HostRef{CampaignID: testCampaignA})
	if err != nil || !ok || id != "cal-default" {
		t.Errorf("DefaultInstance = %q, %v, %v; want cal-default, true, nil (a dm_only default calendar is still the resolved default — RenderBlock re-checks visibility per viewer)", id, ok, err)
	}
}

func TestCalendarInstanceBase_ListInstances(t *testing.T) {
	calRepo := &fakeCalendarRepo{
		listByCampaignFn: func(_ context.Context, campaignID string) ([]Calendar, error) {
			return []Calendar{
				{ID: "cal-a", CampaignID: campaignID, Name: "Visible", Visibility: "everyone"},
				{ID: "cal-b", CampaignID: campaignID, Name: "Hidden", Visibility: "dm_only"},
			}, nil
		},
	}
	svc := newTestCalendarService(calRepo, nil, nil, nil)
	wt := NewCalendarWidgetType(svc)

	refs, err := wt.ListInstances(context.Background(), testCampaignA, int(permissions.RoleScribe))
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("ListInstances (Scribe-gated picker route, system-trusted read) = %d refs, want 2 (both calendars)", len(refs))
	}
}

func TestCalendarInstanceBase_CreateInstance_NotImplemented(t *testing.T) {
	wt := NewCalendarWidgetType(newTestCalendarService(nil, nil, nil, nil))
	if _, err := wt.CreateInstance(context.Background(), testCampaignA, nil); err != widgetbindings.ErrNotImplemented {
		t.Errorf("CreateInstance error = %v, want widgetbindings.ErrNotImplemented (calendar creation is a full wizard, not a quick-create)", err)
	}
}
