package dmscreen

import (
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// RegisterPrefsRoutes mounts each screen's preference routes behind that
// screen's own rule (ScreenAllowed); the service asks again. Every route
// reads or writes only the caller's own row: no route takes a user id.
func RegisterPrefsRoutes(e *echo.Echo, h *PrefsHandler, campaignSvc campaigns.CampaignService, authSvc auth.AuthService) {
	cg := e.Group("/campaigns/:id",
		auth.RequireAuth(authSvc),
		campaigns.RequireCampaignAccess(campaignSvc),
	)
	for _, screen := range Screens {
		gate := screenGate(screen)
		base := "/screen-prefs/" + screen
		cg.GET(base, h.Get(screen), gate)
		cg.PUT(base, h.Put(screen), gate)
		cg.GET(base+"/kits.css", h.KitsCSS(screen), gate)
	}
}

// screenGate refuses, before the handler runs, anyone ScreenAllowed refuses.
func screenGate(screen string) echo.MiddlewareFunc {
	return campaigns.RequireCapability(func(cc *campaigns.CampaignContext) bool {
		return ScreenAllowed(cc, screen) == nil
	}, "this screen's layouts are not yours to change")
}
