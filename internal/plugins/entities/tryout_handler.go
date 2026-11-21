package entities

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// tryout_handler.go is the web door of the try-out: a signed-in visitor on a
// look-around pass builds one practice hero that lives in their browser. Both
// routes only read. A visitor writes nothing to the server, so there is no
// save route here and none may be added: the hero is sent to the server only
// after the owner makes them a player, through the routes a player already has.

// SetVisitorPlanner wires the visitor's plan. Without it the try-out routes
// answer not found and the Characters page offers nothing to a visitor.
func (h *Handler) SetVisitorPlanner(p VisitorPlanner) { h.visitorPlanner = p }

// tryoutReady is the gate both routes and the Characters page share: the
// viewer is a signed-in non-member on a live look-around pass, the planner is
// wired, the campaign lets players claim heroes, and a page type exists to
// claim. Anything else is not found: an anonymous visitor, an ordinary
// signed-in reader of a public campaign, and an expired pass get no hint that
// the feature exists. The pass is checked against the clock here as well as
// in the middleware, so a context that outlives it admits no one.
func (h *Handler) tryoutReady(c *echo.Context, cc *campaigns.CampaignContext) error {
	live := cc.VisitUntil != nil && cc.VisitUntil.After(time.Now())
	if cc.IsMember || cc.IsAnonymous || !live || auth.GetUserID(c) == "" || h.visitorPlanner == nil {
		return apperror.NewNotFound("the try-out is not available here")
	}
	ctx := c.Request().Context()
	if !h.isAddonEnabled(ctx, cc.Campaign.ID, AddonPlayerCharacterClaiming) {
		return apperror.NewNotFound("the try-out is not available here")
	}
	target, _, err := h.practiceTarget(ctx, cc.Campaign.ID)
	if err != nil {
		return err
	}
	if target == nil {
		return apperror.NewNotFound("the try-out is not available here")
	}
	return nil
}

// TryoutNew renders the hero creator in try-out mode
// (GET /campaigns/:id/characters/try). A member is sent to the creator they
// have; the try-out is for people who are not members yet.
func (h *Handler) TryoutNew(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if cc.IsMember && cc.MemberRole >= campaigns.RolePlayer {
		return c.Redirect(http.StatusFound, practiceNewURL(cc.Campaign.ID, ""))
	}
	if err := h.tryoutReady(c, cc); err != nil {
		return err
	}
	// The page holds a stored hero's name and picks only in the visitor's
	// browser; nothing here reads or learns them.
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return middleware.Render(c, http.StatusOK, TryoutCreatorPage(cc, TryoutCreatorView{
		CampaignID: cc.Campaign.ID, UserID: auth.GetUserID(c),
	}))
}

// TryoutPlanAPI returns the visitor's plan (GET /campaigns/:id/characters/try/plan).
func (h *Handler) TryoutPlanAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.tryoutReady(c, cc); err != nil {
		return err
	}
	plan, err := h.visitorPlanner.VisitorPlan(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, tryoutPlan(plan))
}

// tryoutMount gathers the Characters page's try-out mount for this viewer: the
// visitor's Practice section, or a player's "you built this while you were
// looking around" prompt. nil for everyone else. It reads only who the viewer
// is, never anything stored in their browser.
func (h *Handler) tryoutMount(c *echo.Context, cc *campaigns.CampaignContext) *TryoutView {
	userID := auth.GetUserID(c)
	if userID == "" {
		return nil
	}
	ctx := c.Request().Context()
	v := &TryoutView{CampaignID: cc.Campaign.ID, UserID: userID, CSRFToken: middleware.GetCSRFToken(c)}
	switch {
	case !cc.IsMember:
		if h.tryoutReady(c, cc) != nil {
			return nil
		}
		v.Visitor = true
	case cc.MemberRole == campaigns.RolePlayer:
		if !h.isAddonEnabled(ctx, cc.Campaign.ID, AddonPlayerCharacterClaiming) {
			return nil
		}
		target, charTypes, err := h.practiceTarget(ctx, cc.Campaign.ID)
		if err != nil || target == nil {
			return nil
		}
		hero, err := h.ownedCharacter(ctx, cc.Campaign.ID, userID, charTypes)
		if err != nil {
			return nil
		}
		v.HasHero = hero != nil
		// A player who has a hero can only keep it as a practice hero.
		if v.HasHero && h.practice == nil {
			return nil
		}
	default:
		return nil
	}
	return v
}
