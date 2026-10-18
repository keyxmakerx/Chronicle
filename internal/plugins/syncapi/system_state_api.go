package syncapi

import (
	"context"
	"encoding/json"
	"io"
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

// SystemStateWriter replaces the halves of a page's per-system state that are
// non-nil and keeps the rest. Implemented in app over the system-state service,
// which owns validation, the size cap, the enabled-system check and the
// WebSocket publish; this plugin never imports it (T-B2).
type SystemStateWriter interface {
	WriteSystemState(ctx context.Context, campaignID, entityID, systemID, key string, gm, public *json.RawMessage, userID string) (gmOut, publicOut json.RawMessage, updatedAt *time.Time, err error)
}

// SetSystemStateWriter wires the system-state write endpoint. Without it the
// endpoint answers 404.
func (h *APIHandler) SetSystemStateWriter(w SystemStateWriter) { h.systemStateWriter = w }

// maxSystemStateBodyBytes bounds a PUT body: two halves at the service's
// 16 KiB cap plus envelope. The service enforces the per-half cap; this only
// stops an unbounded read.
const maxSystemStateBodyBytes = 2*16*1024 + 1024

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

// PutSystemState replaces the halves present in the body and keeps the rest.
// It applies GetSystemState's page gate, then requires Owner after DM-grant
// promotion (the bar the web route applies with CanAuthorDmOnly): a key that
// is not Owner-level must never write, whatever the route permission says.
// PUT /api/v1/campaigns/:id/entities/:entityID/system-state/:system/:key
func (h *APIHandler) PutSystemState(c echo.Context) error {
	if h.systemStateWriter == nil {
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
	if visRole < int(campaigns.RoleOwner) {
		return apperror.NewForbidden("only the campaign owner or a member with DM access may do this")
	}

	gm, public, err := bindSystemStateBody(c)
	if err != nil {
		return err
	}
	systemID, key := c.Param("system"), c.Param("key")
	gmOut, publicOut, updatedAt, err := h.systemStateWriter.WriteSystemState(ctx, campaignID, entityID, systemID, key, gm, public, userID)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, systemStateResponse{
		SystemID: systemID, Key: key, Public: publicOut, GM: &gmOut, IsGM: true, UpdatedAt: updatedAt,
	})
}

// bindSystemStateBody reads {"public":{...}?, "gm":{...}?} with the browser
// route's rules (replicated, since this plugin cannot import systemstate).
// Members decode raw because a typed pointer would turn an explicit null into
// "absent"; null must reach the service so it is rejected. Unknown members are
// rejected so a misspelt half name cannot become a silent no-op.
func bindSystemStateBody(c echo.Context) (gm, public *json.RawMessage, err error) {
	body, rerr := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, maxSystemStateBodyBytes))
	if rerr != nil {
		return nil, nil, apperror.NewBadRequest("request body is missing or too large")
	}
	var members map[string]json.RawMessage
	if jerr := json.Unmarshal(body, &members); jerr != nil || members == nil {
		return nil, nil, apperror.NewBadRequest("body must be a JSON object")
	}
	for name, raw := range members {
		raw := raw
		switch name {
		case "gm":
			gm = &raw
		case "public":
			public = &raw
		default:
			return nil, nil, apperror.NewValidation("unknown member " + name + "; use gm and/or public")
		}
	}
	return gm, public, nil
}
