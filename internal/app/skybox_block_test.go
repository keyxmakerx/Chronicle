package app

// skybox_block_test.go pins renderSkyboxBlock's outcome per
// GetDefaultCalendarForViewer result (issue #763's "skybox" dashboard/
// template block): a resolved default calendar renders sky.Mount, a
// NotFound (no default calendar yet, or one the viewer may not see) renders
// sky.Empty, any other error fails safe to an empty slot, and the viewer
// passed to the service is built from the block's own CampaignContext/UserID
// exactly like calendar.viewerFrom builds it from a request.

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeSkyboxCalendarService is a test double for the narrow
// skyboxCalendarService seam — it never needs to implement
// calendar.CalendarService's full surface.
type fakeSkyboxCalendarService struct {
	cal *calendar.Calendar
	err error

	// Captured for assertions on the viewer renderSkyboxBlock builds.
	gotCampaignID string
	gotViewer     permissions.Viewer
	calls         int
}

func (f *fakeSkyboxCalendarService) GetDefaultCalendarForViewer(_ context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error) {
	f.calls++
	f.gotCampaignID = campaignID
	f.gotViewer = v
	if f.err != nil {
		return nil, f.err
	}
	return f.cal, nil
}

// renderToString renders a templ.Component the way the layout would, for
// assertions on its output.
func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := c.Render(context.Background(), &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func TestRenderSkyboxBlock_NoCampaignContext(t *testing.T) {
	svc := &fakeSkyboxCalendarService{}
	got := renderToString(t, renderSkyboxBlock(context.Background(), svc, entities.BlockRenderContext{}))
	if got != "" {
		t.Errorf("expected an empty slot with no CampaignContext, got %q", got)
	}
	if svc.calls != 0 {
		t.Errorf("service must not be called with no campaign in context, got %d calls", svc.calls)
	}
}

func TestRenderSkyboxBlock_ResolvedDefaultCalendar_RendersMount(t *testing.T) {
	svc := &fakeSkyboxCalendarService{cal: &calendar.Calendar{ID: "cal-default", CampaignID: "camp-1"}}
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
		IsMember:   true,
	}
	rc := entities.BlockRenderContext{CC: cc, UserID: "user-1"}

	got := renderToString(t, renderSkyboxBlock(context.Background(), svc, rc))

	if !strings.Contains(got, `data-widget="sky-pane"`) {
		t.Errorf("expected sky.Mount's data-widget marker, got %q", got)
	}
	if !strings.Contains(got, `data-calendar-id="cal-default"`) {
		t.Errorf("expected the resolved default calendar's id, got %q", got)
	}
	if svc.gotCampaignID != "camp-1" {
		t.Errorf("gotCampaignID = %q, want camp-1", svc.gotCampaignID)
	}
	if svc.gotViewer.Role() != int(campaigns.RolePlayer) || svc.gotViewer.UserID() != "user-1" {
		t.Errorf("viewer = role %d user %q, want role %d user user-1", svc.gotViewer.Role(), svc.gotViewer.UserID(), int(campaigns.RolePlayer))
	}
}

func TestRenderSkyboxBlock_DmGrantedViewer_PromotedToOwnerRole(t *testing.T) {
	// A co-DM grant promotes VisibilityRole to Owner-equivalent even though
	// MemberRole stays Player — renderSkyboxBlock must build its viewer the
	// same way calendar.viewerFrom does (VisibilityRole, not MemberRole).
	svc := &fakeSkyboxCalendarService{cal: &calendar.Calendar{ID: "cal-default", CampaignID: "camp-1"}}
	cc := &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
		IsMember:    true,
	}
	rc := entities.BlockRenderContext{CC: cc, UserID: "co-dm-1"}

	renderToString(t, renderSkyboxBlock(context.Background(), svc, rc))

	if svc.gotViewer.Role() != int(campaigns.RoleOwner) {
		t.Errorf("a DM-granted viewer's role = %d, want %d (Owner-equivalent)", svc.gotViewer.Role(), int(campaigns.RoleOwner))
	}
}

func TestRenderSkyboxBlock_NotFound_RendersEmptyPlaceholder(t *testing.T) {
	svc := &fakeSkyboxCalendarService{err: apperror.NewNotFound("calendar not found")}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	rc := entities.BlockRenderContext{CC: cc, UserID: "user-1"}

	got := renderToString(t, renderSkyboxBlock(context.Background(), svc, rc))

	if strings.Contains(got, `data-widget="sky-pane"`) {
		t.Errorf("NotFound must not render the live mount, got %q", got)
	}
	if !strings.Contains(got, "skypane-empty") {
		t.Errorf("expected sky.Empty's placeholder, got %q", got)
	}
}

func TestRenderSkyboxBlock_OtherError_FailsSafeToEmptySlot(t *testing.T) {
	svc := &fakeSkyboxCalendarService{err: errors.New("db is on fire")}
	cc := &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "camp-1"}}
	rc := entities.BlockRenderContext{CC: cc, UserID: "user-1"}

	got := renderToString(t, renderSkyboxBlock(context.Background(), svc, rc))

	if got != "" {
		t.Errorf("a non-NotFound service error must fail safe to an empty slot (never a 500), got %q", got)
	}
}
