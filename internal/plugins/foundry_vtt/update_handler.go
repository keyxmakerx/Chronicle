package foundry_vtt

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/packages"
)

// SetOwnerUpdates injects the owner's update flow. Without it the update
// screens render nothing and the actions answer 404, which is how a build
// without the update modes behaves.
func (h *Handler) SetOwnerUpdates(o OwnerUpdates) {
	h.owner = o
}

// ownerCampaign returns the campaign for an owner-only update endpoint. The
// route already requires the owner role; checking it here too keeps a player
// from ever being served the line or moving the campaign if a route were
// ever mounted without it.
func (h *Handler) ownerCampaign(c echo.Context) (*campaigns.CampaignContext, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return nil, apperror.NewMissingContext()
	}
	if cc.MemberRole < campaigns.RoleOwner {
		return nil, apperror.NewForbidden("only the campaign owner can do this")
	}
	if h.owner == nil {
		return nil, apperror.NewNotFound("updates are not available")
	}
	return cc, nil
}

// updateView is the form's choice of screen, defaulting to the dashboard line.
func updateView(c echo.Context) string {
	if c.FormValue("view") == updateViewRow {
		return updateViewRow
	}
	return updateViewBanner
}

// renderUpdate answers with the markup of the screen the action came from.
func (h *Handler) renderUpdate(c echo.Context, cc *campaigns.CampaignContext, view string, updated bool) error {
	v, err := h.owner.View(c.Request().Context(), cc.Campaign.ID)
	if err != nil {
		return err
	}
	if v != nil {
		v.CampaignName = cc.Campaign.Name
		if updated {
			v.Done = fmt.Sprintf("Updated to %s %s.", v.Package, v.Running)
		}
	}
	csrf := middleware.GetCSRFToken(c)
	if view == updateViewRow {
		return middleware.Render(c, http.StatusOK, AppsUpdateRow(v, csrf))
	}
	return middleware.Render(c, http.StatusOK, CampaignUpdateBanner(v, csrf))
}

// CampaignShowBannerHandler serves the owner's "new version is ready" line at
// the top of the campaign home page. Lazy-loaded by campaigns/show.templ.
// Never fails the page: a problem reads as "nothing to show".
//
// GET /campaigns/:id/foundry-vtt/show-banner-fragment
func (h *Handler) CampaignShowBannerHandler(c echo.Context) error {
	cc, err := h.ownerCampaign(c)
	if err != nil {
		return err
	}
	v, err := h.owner.View(c.Request().Context(), cc.Campaign.ID)
	if err != nil || v == nil {
		return c.NoContent(http.StatusOK)
	}
	v.CampaignName = cc.Campaign.Name
	return middleware.Render(c, http.StatusOK, CampaignUpdateBanner(v, middleware.GetCSRFToken(c)))
}

// AppsUpdateRowHandler serves the Foundry module row of the owner's Apps &
// game system page.
//
// GET /campaigns/:id/foundry-vtt/apps-row
func (h *Handler) AppsUpdateRowHandler(c echo.Context) error {
	cc, err := h.ownerCampaign(c)
	if err != nil {
		return err
	}
	v, err := h.owner.View(c.Request().Context(), cc.Campaign.ID)
	if err != nil || v == nil {
		return c.NoContent(http.StatusOK)
	}
	v.CampaignName = cc.Campaign.Name
	return middleware.Render(c, http.StatusOK, AppsUpdateRow(v, middleware.GetCSRFToken(c)))
}

// ownerActor is who is pressing the button: the owner themself, never an
// admin acting for them, so an admin hold applies to them too.
func ownerActor(c echo.Context) (packages.ActorInfo, error) {
	session := auth.GetSession(c)
	if session == nil {
		return packages.ActorInfo{}, apperror.NewUnauthorized("not authenticated")
	}
	return packages.ActorInfo{UserID: session.UserID, IP: c.RealIP(), UserAgent: c.Request().UserAgent()}, nil
}

// OwnerUpdateHandler moves the campaign onto the version the owner confirmed.
//
// POST /campaigns/:id/foundry-vtt/update   form: version, view
func (h *Handler) OwnerUpdateHandler(c echo.Context) error {
	cc, err := h.ownerCampaign(c)
	if err != nil {
		return err
	}
	actor, err := ownerActor(c)
	if err != nil {
		return err
	}
	if err := h.owner.Update(c.Request().Context(), cc.Campaign.ID, c.FormValue("version"), actor); err != nil {
		return err
	}
	return h.renderUpdate(c, cc, updateView(c), true)
}

// OwnerUpdateLaterHandler hides the prompt for the waiting version.
//
// POST /campaigns/:id/foundry-vtt/update/later   form: version, view
func (h *Handler) OwnerUpdateLaterHandler(c echo.Context) error {
	cc, err := h.ownerCampaign(c)
	if err != nil {
		return err
	}
	if err := h.owner.Later(c.Request().Context(), cc.Campaign.ID, c.FormValue("version")); err != nil {
		return err
	}
	return h.renderUpdate(c, cc, updateView(c), false)
}

// OwnerUpdateSwitchHandler keeps the campaign on a version the owner picked.
//
// POST /campaigns/:id/foundry-vtt/update/switch   form: version, view
func (h *Handler) OwnerUpdateSwitchHandler(c echo.Context) error {
	cc, err := h.ownerCampaign(c)
	if err != nil {
		return err
	}
	actor, err := ownerActor(c)
	if err != nil {
		return err
	}
	if err := h.owner.Switch(c.Request().Context(), cc.Campaign.ID, c.FormValue("version"), actor); err != nil {
		return err
	}
	return h.renderUpdate(c, cc, updateView(c), false)
}
