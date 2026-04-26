package entity_notes

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler is the HTTP boundary for entity_notes. Handlers stay thin:
// build a ViewerContext from CampaignContext + auth, decode the
// request body, delegate to the service, return JSON.
type Handler struct {
	svc Service
}

// NewHandler constructs an HTTP handler against a service.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

// viewerFrom collapses the framework-supplied CampaignContext into the
// audience-relevant facts the service needs. The IsScribe assertion
// excludes Owner because Owner > Scribe in the role enum and the ACL
// helpers explicitly check both flags; conflating them would make the
// dm_scribe vs dm_only asymmetry harder to read.
func viewerFrom(cc *campaigns.CampaignContext, userID string) ViewerContext {
	return ViewerContext{
		UserID:      userID,
		CampaignID:  cc.Campaign.ID,
		IsOwner:     cc.MemberRole == campaigns.RoleOwner,
		IsScribe:    cc.MemberRole == campaigns.RoleScribe,
		IsDMGranted: cc.IsDmGranted,
	}
}

// List returns notes attached to an entity that the viewer can see.
// GET /campaigns/:id/entities/:eid/notes
func (h *Handler) List(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	notes, err := h.svc.List(c.Request().Context(), entityID, viewer)
	if err != nil {
		return err
	}
	if notes == nil {
		notes = []Note{}
	}
	return c.JSON(http.StatusOK, notes)
}

// Create authors a new note. The author is taken from auth context;
// any author_user_id in the body is ignored.
// POST /campaigns/:id/entities/:eid/notes
func (h *Handler) Create(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}
	var req CreateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Create(c.Request().Context(), entityID, viewer, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, note)
}

// Get returns one note if the viewer can read it.
// GET /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Get(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")
	if noteID == "" {
		return apperror.NewBadRequest("note ID is required")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Get(c.Request().Context(), noteID, viewer)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// Update mutates a note. Author-only enforced inside the service.
// PUT /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Update(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")
	if noteID == "" {
		return apperror.NewBadRequest("note ID is required")
	}
	var req UpdateNoteRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return apperror.NewBadRequest("invalid JSON body")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	note, err := h.svc.Update(c.Request().Context(), noteID, viewer, req)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, note)
}

// Delete removes a note. Author-only enforced inside the service.
// DELETE /campaigns/:id/entities/:eid/notes/:nid
func (h *Handler) Delete(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	noteID := c.Param("nid")
	if noteID == "" {
		return apperror.NewBadRequest("note ID is required")
	}
	viewer := viewerFrom(cc, auth.GetUserID(c))
	if err := h.svc.Delete(c.Request().Context(), noteID, viewer); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
