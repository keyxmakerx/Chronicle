package sessions

import (
	"errors"
	"net/http"

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

	// A night counts as played only once its date has ended everywhere, so
	// nobody sees "Played" while still at the table.
	today := gameNightsToday()
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

// nightPage is what a game night's page in the calendar shows below the
// answers: the recap and the pages linked to the night.
type nightPage struct {
	// Recap is the plain text the recap was written as, for editing it.
	Recap     string          `json:"recap"`
	RecapHTML string          `json:"recapHtml"`
	Links     []nightPageLink `json:"links"`
}

// nightPageLink is one linked page, already filtered to what the viewer may
// see.
type nightPageLink struct {
	EntityID string `json:"entityId"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Role     string `json:"role"`
}

// NightPageAPI returns a game night's recap and linked pages for its page in
// the calendar. Linked pages go through the same visibility filter as the
// Sessions page (ADR-055 rule 3): a page the viewer can't open is absent,
// not named.
// GET /campaigns/:id/sessions/:sid/page
func (h *Handler) NightPageAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	session, err := h.requireSessionInCampaign(c, c.Param("sid"), cc.Campaign.ID)
	if err != nil {
		return err
	}
	visible, err := h.svc.FilterEntitiesForViewer(c.Request().Context(), cc.Campaign.ID, session.Entities, int(cc.VisibilityRole()), auth.GetUserID(c))
	if err != nil {
		return err
	}
	out := nightPage{Recap: strPtrVal(session.Recap), RecapHTML: strPtrVal(session.RecapHTML), Links: []nightPageLink{}}
	for _, e := range visible {
		out.Links = append(out.Links, nightPageLink{EntityID: e.EntityID, Name: e.EntityName, Slug: e.EntitySlug, Role: e.Role})
	}
	return c.JSON(http.StatusOK, out)
}
