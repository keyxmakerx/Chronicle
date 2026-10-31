// Pins effectiveRole, the role-selection function every timeline
// content-filtering call site (Index, Show, and friends) goes through: a
// co-DM must be promoted to cc.VisibilityRole() so they see dm_only events
// like an Owner, EXCEPT when view-as-player is active, which must still
// force RolePlayer regardless of promotion.
//
// This exercises effectiveRole directly (a pure function of the preview
// cookie and CampaignContext); it proves role selection, not that every SQL/service
// path narrows content correctly for that role (see service.go / repository.go).
package timeline

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// newTimelineTestContext builds an echo.Context carrying the view-as-player
// toggle the way a browser sends it: the cookie, not the layout's context
// flag, which is only set while rendering, after the handler loaded its data.
func newTimelineTestContext(viewingAsPlayer bool) echo.Context {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/campaigns/camp-1/timelines", nil)
	if viewingAsPlayer {
		req.AddCookie(&http.Cookie{Name: "chronicle_view_as_player", Value: "1"})
	}
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec)
}

// TestEffectiveRole_CoDMIsPromotedToOwnerForVisibility is the RED/GREEN case:
// outside view-as-player mode, a co-DM (MemberRole=Player, IsDmGranted=true)
// must resolve to Owner, matching cc.VisibilityRole() and every entity path.
func TestEffectiveRole_CoDMIsPromotedToOwnerForVisibility(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	}
	c := newTimelineTestContext(false)

	got := effectiveRole(c, cc)
	if got != int(permissions.RoleOwner) {
		t.Errorf("effectiveRole for a co-DM (not previewing) = %d, want %d (permissions.RoleOwner via cc.VisibilityRole()) — "+
			"a co-DM must see dm_only timeline events like an Owner does",
			got, int(permissions.RoleOwner))
	}
}

// TestEffectiveRole_OwnerViewAsPlayerStillReturnsPlayer pins that an Owner
// (co-DM or not) previewing the player experience still gets RolePlayer,
// never promoted back up.
func TestEffectiveRole_OwnerViewAsPlayerStillReturnsPlayer(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	}
	c := newTimelineTestContext(true)

	got := effectiveRole(c, cc)
	if got != int(campaigns.RolePlayer) {
		t.Errorf("effectiveRole for an Owner in view-as-player mode = %d, want %d (campaigns.RolePlayer) — "+
			"view-as-player must still show the PLAYER view",
			got, int(campaigns.RolePlayer))
	}
}

// TestEffectiveRole_CoDMViewAsPlayerStillReturnsPlayer covers the specific
// composition the task called out: a co-DM (who WOULD be promoted outside
// preview mode) previewing as a player must also see the PLAYER view, not
// their promoted Owner-for-visibility role.
func TestEffectiveRole_CoDMViewAsPlayerStillReturnsPlayer(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:    &campaigns.Campaign{ID: "camp-1"},
		MemberRole:  campaigns.RolePlayer,
		IsDmGranted: true,
	}
	c := newTimelineTestContext(true)

	got := effectiveRole(c, cc)
	if got != int(campaigns.RolePlayer) {
		t.Errorf("effectiveRole for a co-DM in view-as-player mode = %d, want %d (campaigns.RolePlayer) — "+
			"the preview branch must win over the visibility promotion",
			got, int(campaigns.RolePlayer))
	}
}

// TestEffectiveRole_PlainPlayerIsNotPromoted is the negative control: a
// Player with no DM grant, not previewing, must still resolve to RolePlayer.
func TestEffectiveRole_PlainPlayerIsNotPromoted(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RolePlayer,
	}
	c := newTimelineTestContext(false)

	got := effectiveRole(c, cc)
	if got != int(campaigns.RolePlayer) {
		t.Errorf("effectiveRole for a plain Player = %d, want %d (campaigns.RolePlayer, unpromoted)",
			got, int(campaigns.RolePlayer))
	}
}

// TestEffectiveRole_OwnerNotPreviewingIsUnaffected pins that an actual Owner
// (not previewing) is unaffected by the fix — cc.VisibilityRole() for an
// Owner is already RoleOwner, so this must pass both before and after.
func TestEffectiveRole_OwnerNotPreviewingIsUnaffected(t *testing.T) {
	cc := &campaigns.CampaignContext{
		Campaign:   &campaigns.Campaign{ID: "camp-1"},
		MemberRole: campaigns.RoleOwner,
	}
	c := newTimelineTestContext(false)

	got := effectiveRole(c, cc)
	if got != int(campaigns.RoleOwner) {
		t.Errorf("effectiveRole for an Owner (not previewing) = %d, want %d (campaigns.RoleOwner)",
			got, int(campaigns.RoleOwner))
	}
}
