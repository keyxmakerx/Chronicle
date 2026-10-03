// The browser HTTP surface for system state. Handlers only bind, call the
// service and shape the response; the one decision they own is the audience
// split: the gm half leaves the server only for callers who may author
// DM-only content.
package systemstate

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// maxBodyBytes bounds a PUT body: two halves at their cap plus envelope.
const maxBodyBytes = 2*MaxHalfBytes + 1024

// EntityAccess answers whether the viewer may see an entity and which campaign
// owns it, using the entities plugin's own visibility rules so a private page
// stays hidden here too. Implemented in app (T-B2: no entities import here).
type EntityAccess interface {
	ResolveViewableEntity(ctx context.Context, entityID string, role int, userID string) (campaignID string, canView bool, err error)
}

// Handler serves the system-state routes.
type Handler struct {
	svc    Service
	access EntityAccess
}

// NewHandler builds the handler.
func NewHandler(svc Service, access EntityAccess) *Handler {
	return &Handler{svc: svc, access: access}
}

// StateResponse is the wire shape of GET and PUT.
type StateResponse struct {
	SystemID string          `json:"systemId"`
	Key      string          `json:"key"`
	Public   json.RawMessage `json:"public"`
	// GM is omitted entirely (not null, not {}) for callers without DM
	// access, so its absence reveals nothing about whether data exists.
	GM        *json.RawMessage `json:"gm,omitempty"`
	IsGM      bool             `json:"isGm"`
	UpdatedAt *time.Time       `json:"updatedAt"`
}

// NewStateResponse shapes a State for a caller. includeGM must come from the
// caller's verified capability, never from the request.
func NewStateResponse(systemID, key string, st State, includeGM bool) StateResponse {
	out := StateResponse{
		SystemID:  systemID,
		Key:       key,
		Public:    st.Public,
		IsGM:      includeGM,
		UpdatedAt: st.UpdatedAt,
	}
	if includeGM {
		gm := st.GM
		out.GM = &gm
	}
	return out
}

// Get returns the state of one page for any member who can view the page.
//
// GET /campaigns/:id/entities/:eid/system-state/:system/:key
func (h *Handler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	systemID, key, entityID := c.Param("system"), c.Param("key"), c.Param("eid")
	if err := h.requireViewable(c, cc, entityID); err != nil {
		return err
	}
	st, err := h.svc.Get(c.Request().Context(), cc.Campaign.ID, entityID, systemID, key)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, NewStateResponse(systemID, key, st, cc.CanAuthorDmOnly()))
}

// Put replaces the halves present in the body and keeps the rest. The route is
// gated to the DM team; the handler re-derives the audience from the same
// capability rather than trusting the gate alone.
//
// PUT /campaigns/:id/entities/:eid/system-state/:system/:key
func (h *Handler) Put(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !cc.CanAuthorDmOnly() {
		return apperror.NewForbidden("only the campaign owner or a member with DM access may do this")
	}
	systemID, key, entityID := c.Param("system"), c.Param("key"), c.Param("eid")
	gm, public, err := bindPutBody(c)
	if err != nil {
		return err
	}
	if err := h.requireViewable(c, cc, entityID); err != nil {
		return err
	}
	st, err := h.svc.Put(c.Request().Context(), cc.Campaign.ID, entityID, systemID, key, gm, public, auth.GetUserID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, NewStateResponse(systemID, key, st, true))
}

// requireViewable applies the show-page gate: the entity must belong to this
// campaign and be visible to the viewer. Both failures are a plain 404.
func (h *Handler) requireViewable(c echo.Context, cc *campaigns.CampaignContext, entityID string) error {
	owner, canView, err := h.access.ResolveViewableEntity(c.Request().Context(), entityID, int(cc.VisibilityRole()), auth.GetUserID(c))
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == http.StatusNotFound {
			return err
		}
		return apperror.NewInternal(err)
	}
	if owner != cc.Campaign.ID || !canView {
		return apperror.NewNotFound("entity not found")
	}
	return nil
}

// bindPutBody reads {"public":{...}?, "gm":{...}?}. It decodes into raw
// members instead of typed pointers because encoding/json turns an explicit
// null into a nil pointer, which would silently read as "absent"; here null
// must reach the service so it is rejected. Unknown members are rejected so a
// misspelt half name cannot become a silent no-op.
func bindPutBody(c echo.Context) (gm, public *json.RawMessage, err error) {
	body, rerr := io.ReadAll(http.MaxBytesReader(c.Response(), c.Request().Body, maxBodyBytes))
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
