package maps

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// HexHandler serves the hex layer's JSON API. It binds, calls the service and
// renders; who may see or write what is decided in HexService.
type HexHandler struct {
	svc HexService
}

// NewHexHandler creates a new HexHandler.
func NewHexHandler(svc HexService) *HexHandler {
	return &HexHandler{svc: svc}
}

// GetHexes returns the map's hex layer with the cells the viewer may receive.
// GET /campaigns/:id/maps/:mid/hexes
func (h *HexHandler) GetHexes(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	// VisibilityRole, not MemberRole: a co-DM grant must see unexplored hexes.
	view, err := h.svc.GetLayer(c.Request().Context(), cc.Campaign.ID, c.Param("mid"), cc.VisibilityRole())
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, view)
}

// hexCellRequest is one wire entry. Col and Row are pointers so a missing one
// can be refused instead of silently meaning hex 0; the rest are presence-aware.
type hexCellRequest struct {
	Col     *int                `json:"col"`
	Row     *int                `json:"row"`
	Terrain patch.Field[string] `json:"terrain"`
	Name    patch.Field[string] `json:"name"`
	Notes   patch.Field[string] `json:"notes"`
}

// PatchHexCells changes a batch of hexes. Absent keys keep, null clears.
// PATCH /campaigns/:id/maps/:mid/hexes/cells
func (h *HexHandler) PatchHexCells(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req struct {
		Cells []hexCellRequest `json:"cells"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	entries := make([]UpdateHexCellInput, len(req.Cells))
	for i, e := range req.Cells {
		if e.Col == nil || e.Row == nil {
			return apperror.NewBadRequest("every hex needs a col and a row")
		}
		entries[i] = UpdateHexCellInput{Col: *e.Col, Row: *e.Row, Terrain: e.Terrain, Name: e.Name, Notes: e.Notes}
	}
	res, err := h.svc.PatchCells(c.Request().Context(), cc.Campaign.ID, c.Param("mid"), HexActor{
		UserID: getUserID(c),
		Role:   int(cc.MemberRole),
		IsDM:   cc.CanAuthorDmOnly(),
	}, entries)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}

// PutHexLayer changes the layer row. The body is partial: an absent
// anchor_drawing_id keeps, null means the whole map, an id pins the hexes to
// that picture.
// PUT /campaigns/:id/maps/:mid/hexes/layer
func (h *HexHandler) PutHexLayer(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	var req struct {
		AnchorDrawingID patch.Field[string] `json:"anchor_drawing_id"`
	}
	if err := c.Bind(&req); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	res, err := h.svc.UpdateLayer(c.Request().Context(), cc.Campaign.ID, c.Param("mid"), HexActor{
		UserID: getUserID(c),
		Role:   int(cc.MemberRole),
		IsDM:   cc.CanAuthorDmOnly(),
	}, UpdateHexLayerInput{AnchorDrawingID: req.AnchorDrawingID})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, res)
}
