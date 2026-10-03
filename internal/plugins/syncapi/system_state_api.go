package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// SystemStateReader reads one page's per-system state document. Implemented in
// app over the system-state service so this plugin never imports it (T-B2).
// Both halves are returned unfiltered; the handler decides what leaves.
type SystemStateReader interface {
	ReadSystemState(ctx context.Context, campaignID, entityID, systemID, key string) (gm, public json.RawMessage, updatedAt *time.Time, err error)
}

// SetSystemStateReader wires the system-state read endpoint. Without it the
// endpoint answers 404.
func (h *APIHandler) SetSystemStateReader(r SystemStateReader) { h.systemState = r }

// systemStateResponse mirrors the browser route's shape so a client can use
// either. gm is omitted for callers who are not the DM team.
type systemStateResponse struct {
	SystemID  string           `json:"systemId"`
	Key       string           `json:"key"`
	Public    json.RawMessage  `json:"public"`
	GM        *json.RawMessage `json:"gm,omitempty"`
	IsGM      bool             `json:"isGm"`
	UpdatedAt *time.Time       `json:"updatedAt"`
}

// GetSystemState returns a page's per-system state. The page must be visible
// to the caller (same gate as GetEntity). The gm half is included only when
// the caller resolves to Owner after DM-grant promotion, the same bar the web
// route applies with CanAuthorDmOnly; every stored Bearer key is Owner-level.
// GET /api/v1/campaigns/:id/entities/:entityID/system-state/:system/:key
func (h *APIHandler) GetSystemState(c echo.Context) error {
	if h.systemState == nil {
		return apperror.NewNotFound("system state is not available")
	}
	ctx := c.Request().Context()
	campaignID, entityID := c.Param("id"), c.Param("entityID")
	role := h.resolveRole(c)

	entity, err := h.entitySvc.GetByID(ctx, entityID)
	if err != nil || entity.CampaignID != campaignID {
		return apperror.NewNotFound("entity not found")
	}
	userID := h.resolveUserID(c)
	visRole := h.visibilityRoleFor(ctx, campaignID, userID, role)
	access, accessErr := h.entitySvc.CheckEntityAccess(ctx, entity.ID, visRole, userID)
	if accessErr != nil || !access.CanView {
		return apperror.NewNotFound("entity not found")
	}

	systemID, key := c.Param("system"), c.Param("key")
	gm, public, updatedAt, err := h.systemState.ReadSystemState(ctx, campaignID, entityID, systemID, key)
	if err != nil {
		return err
	}
	isGM := visRole >= int(campaigns.RoleOwner)
	out := systemStateResponse{SystemID: systemID, Key: key, Public: public, IsGM: isGM, UpdatedAt: updatedAt}
	if isGM {
		out.GM = &gm
	}
	return c.JSON(http.StatusOK, out)
}
