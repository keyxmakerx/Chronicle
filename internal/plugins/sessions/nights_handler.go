package sessions

import (
	"errors"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ListGameNightsAPI returns the game nights in a date window, each with the
// whole table's answers, for the calendar's day card and event page.
// GET /campaigns/:id/sessions/nights?from=YYYY-MM-DD&to=YYYY-MM-DD
func (h *Handler) ListGameNightsAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	ctx := c.Request().Context()
	// Without the member list there is no honest roster: a missing member
	// would read as someone who was never asked.
	if h.memberLister == nil {
		return apperror.NewInternal(errors.New("sessions: member lister not wired"))
	}
	list, err := h.memberLister.ListMembers(ctx, cc.Campaign.ID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	members := make([]NightMember, 0, len(list))
	for _, m := range list {
		members = append(members, NightMember{UserID: m.UserID, Name: m.DisplayName})
	}

	// A night counts as played only once its date has ended in every zone
	// (UTC-14 is the last), so nobody sees "Played" while still at the table.
	today := time.Now().UTC().Add(-14 * time.Hour).Format("2006-01-02")
	nights, err := h.svc.ListGameNights(ctx, cc.Campaign.ID, c.QueryParam("from"), c.QueryParam("to"), today, members)
	if err != nil {
		return err
	}
	viewer := auth.GetUserID(c)
	isOwner := cc.MemberRole >= campaigns.RoleOwner
	for i := range nights {
		nights[i].ForViewer(viewer, isOwner)
	}
	return c.JSON(http.StatusOK, nights)
}
