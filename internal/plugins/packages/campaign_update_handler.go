package packages

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// OwnerReminder tells a campaign's owner that a version is waiting for them.
// It is implemented outside this plugin by the package types that have a way
// to reach owners; without one, Remind owner is not offered.
type OwnerReminder interface {
	RemindOwner(ctx context.Context, pkg *Package, campaignID, version string, actor ActorInfo) error
}

// SetCampaignUpdates injects the per-campaign update service. Without it the
// Campaigns tab shows the plain usage list and the actions answer 404.
func (h *Handler) SetCampaignUpdates(svc CampaignUpdateService) {
	h.updates = svc
}

// SetOwnerReminder injects the way to remind an owner.
func (h *Handler) SetOwnerReminder(r OwnerReminder) {
	h.reminder = r
}

// adminCampaignAction returns the actor for a per-campaign admin action. The
// route group already requires a site admin; Admin here tells the service the
// action is on an owner's behalf, which is what lets it pass an admin hold.
func (h *Handler) adminCampaignAction(c echo.Context) (ActorInfo, error) {
	if h.updates == nil {
		return ActorInfo{}, apperror.NewNotFound("campaign updates are not available")
	}
	session := auth.GetSession(c)
	if session == nil {
		return ActorInfo{}, apperror.NewUnauthorized("not authenticated")
	}
	return ActorInfo{UserID: session.UserID, IP: c.RealIP(), UserAgent: c.Request().UserAgent(), Admin: true}, nil
}

// HoldCampaign keeps a campaign on its current version against its owner's
// Update (POST /admin/packages/:id/campaigns/:cid/hold).
func (h *Handler) HoldCampaign(c echo.Context) error {
	actor, err := h.adminCampaignAction(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, cid := c.Param("id"), c.Param("cid")
	if _, err := h.updates.SetAdminHold(ctx, cid, id, true, actor); err != nil {
		return err
	}
	h.recordActivity(c, "package.campaign_held", "package", id, h.packageLabel(ctx, id))
	return h.backToPage(c)
}

// ReleaseCampaign lets go of a hold (DELETE /admin/packages/:id/campaigns/:cid/hold).
func (h *Handler) ReleaseCampaign(c echo.Context) error {
	actor, err := h.adminCampaignAction(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	id, cid := c.Param("id"), c.Param("cid")
	if _, err := h.updates.SetAdminHold(ctx, cid, id, false, actor); err != nil {
		return err
	}
	h.recordActivity(c, "package.campaign_released", "package", id, h.packageLabel(ctx, id))
	return h.backToPage(c)
}

// MoveCampaign moves a campaign to a version the admin picked
// (PUT /admin/packages/:id/campaigns/:cid/version, form: version).
func (h *Handler) MoveCampaign(c echo.Context) error {
	actor, err := h.adminCampaignAction(c)
	if err != nil {
		return err
	}
	var input InstallVersionInput
	if err := c.Bind(&input); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request")
	}
	ctx := c.Request().Context()
	id, cid := c.Param("id"), c.Param("cid")
	if _, err := h.updates.SwitchVersion(ctx, cid, id, input.Version, actor); err != nil {
		return err
	}
	h.recordActivity(c, "package.campaign_moved", "package", id, h.packageLabel(ctx, id))
	return h.backToPage(c)
}

// RemindCampaignOwner nudges the owner of a campaign that has a version
// waiting (POST /admin/packages/:id/campaigns/:cid/remind).
func (h *Handler) RemindCampaignOwner(c echo.Context) error {
	actor, err := h.adminCampaignAction(c)
	if err != nil {
		return err
	}
	if h.reminder == nil {
		return apperror.NewNotFound("reminders are not available")
	}
	ctx := c.Request().Context()
	id, cid := c.Param("id"), c.Param("cid")
	pkg, err := h.service.GetPackage(ctx, id)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if pkg == nil {
		return apperror.NewNotFound("package not found")
	}
	st, err := h.updates.CampaignState(ctx, cid, id)
	if err != nil {
		return err
	}
	// Only a version that is actually waiting is worth a reminder, and it is
	// read here rather than taken from the request.
	if st.HeldVersion == "" {
		return apperror.NewConflict("nothing is waiting for this owner")
	}
	if err := h.reminder.RemindOwner(ctx, pkg, cid, st.HeldVersion, actor); err != nil {
		return err
	}
	h.recordActivity(c, "package.campaign_owner_reminded", "package", id, h.packageLabel(ctx, id))
	return h.backToPage(c)
}
