package tags

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// ListGrants returns the visibility grants on a tag.
// GET /campaigns/:id/tags/:tagId/grants (Owner only).
func (h *Handler) ListGrants(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	grants, err := h.grantSvc.ListByTag(c.Request().Context(), cc.Campaign.ID, tagID)
	if err != nil {
		return err
	}
	if grants == nil {
		grants = []TagPermission{}
	}
	return c.JSON(http.StatusOK, grants)
}

// CreateGrant adds a visibility grant to a tag.
// POST /campaigns/:id/tags/:tagId/grants (Owner only).
func (h *Handler) CreateGrant(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}

	var req CreateTagPermissionRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}

	grant, err := h.grantSvc.Create(c.Request().Context(), cc.Campaign.ID, tagID,
		req.SubjectType, req.SubjectID, auth.GetUserID(c))
	if err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "tag.grant.created", req.SubjectType+":"+req.SubjectID)

	return c.JSON(http.StatusCreated, grant)
}

// DeleteGrant removes a visibility grant from a tag.
// DELETE /campaigns/:id/tags/:tagId/grants/:grantId (Owner only).
func (h *Handler) DeleteGrant(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	tagID, err := strconv.Atoi(c.Param("tagId"))
	if err != nil {
		return apperror.NewBadRequest("invalid tag ID")
	}
	grantID, err := strconv.Atoi(c.Param("grantId"))
	if err != nil {
		return apperror.NewBadRequest("invalid grant ID")
	}

	if err := h.grantSvc.Delete(c.Request().Context(), cc.Campaign.ID, tagID, grantID); err != nil {
		return err
	}

	h.logAudit(c, cc.Campaign.ID, "tag.grant.deleted", strconv.Itoa(grantID))

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
