package app

import (
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// pageSeenMarker is what the middleware writes; websocket.Hub satisfies it.
type pageSeenMarker interface {
	MarkBrowserSeen(campaignID, userID string)
}

// campaignPageSeen records that a signed-in member just loaded a campaign
// page, so the DM Screen can count players who have Chronicle open even on
// pages that never open a socket. Only GET page loads and HTMX fetches count
// (a background JSON poll does not mean someone is looking), only successful
// ones, and only for real members; the write is one map set under a mutex.
//
// It runs after the handler because the session and campaign membership are
// resolved by the route group's own middleware, which runs inside it.
func campaignPageSeen(m pageSeenMarker) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			err := next(c)
			if err != nil || c.Request().Method != "GET" || c.Response().Status >= 400 {
				return err
			}
			if !strings.HasPrefix(c.Path(), "/campaigns/:id") {
				return err
			}
			req := c.Request()
			if req.Header.Get("HX-Request") == "" && !strings.Contains(req.Header.Get("Accept"), "text/html") {
				return err
			}
			if cc := campaigns.GetCampaignContext(c); cc != nil && cc.IsMember && cc.Campaign != nil {
				m.MarkBrowserSeen(cc.Campaign.ID, auth.GetUserID(c))
			}
			return err
		}
	}
}
