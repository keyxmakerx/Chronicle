// The browser HTTP surface for trackers. Handlers bind, call the service and
// shape the response; the service owns every rule, so a mis-wired route
// cannot widen access.
package trackers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the tracker routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c echo.Context) (string, Viewer, error) {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return "", Viewer{}, apperror.NewMissingContext()
	}
	return cc.Campaign.ID, Viewer{
		UserID:         auth.GetUserID(c),
		MemberRole:     int(cc.MemberRole),
		VisibilityRole: cc.VisibilityRole(),
		Director:       cc.CanAuthorDmOnly(),
	}, nil
}

// bind reads a JSON body of at most MaxBodyBytes into dst.
func bind(c echo.Context, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > MaxBodyBytes {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// Page handles GET /campaigns/:id/trackers/page/:eid; ?owner= limits it to
// the kinds owned under that prefix.
func (h *Handler) Page(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var out *PageView
	if owner := c.QueryParam("owner"); owner != "" {
		out, err = h.svc.OwnedPage(c.Request().Context(), cid, c.Param("eid"), owner, v)
	} else {
		out, err = h.svc.Page(c.Request().Context(), cid, c.Param("eid"), v)
	}
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Pages handles GET /campaigns/:id/trackers/pages.
func (h *Handler) Pages(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Pages(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Place handles POST /campaigns/:id/trackers/page/:eid.
func (h *Handler) Place(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in PlaceInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Place(c.Request().Context(), cid, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Log handles POST /campaigns/:id/trackers/page/:eid/log.
func (h *Handler) Log(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in LogInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Log(c.Request().Context(), cid, c.Param("eid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Patch handles PATCH /campaigns/:id/trackers/:tid.
func (h *Handler) Patch(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in TrackerPatch
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Patch(c.Request().Context(), cid, c.Param("tid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Remove handles DELETE /campaigns/:id/trackers/:tid.
func (h *Handler) Remove(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Remove(c.Request().Context(), cid, c.Param("tid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type strikeBody struct {
	Struck bool `json:"struck"`
}

// Strike handles POST /campaigns/:id/trackers/entries/:entry/strike.
func (h *Handler) Strike(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	id, perr := strconv.ParseInt(c.Param("entry"), 10, 64)
	if perr != nil || id <= 0 {
		return apperror.NewNotFound("log line")
	}
	var in strikeBody
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.Strike(c.Request().Context(), cid, id, in.Struck, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// ListKinds handles GET /campaigns/:id/tracker-kinds.
func (h *Handler) ListKinds(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	kinds, types, err := h.svc.Kinds(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"kinds": kinds, "types": types})
}

// CreateKind handles POST /campaigns/:id/tracker-kinds.
func (h *Handler) CreateKind(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in KindInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.CreateKind(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// UpdateKind handles PUT /campaigns/:id/tracker-kinds/:kid.
func (h *Handler) UpdateKind(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in KindInput
	if err := bind(c, &in); err != nil {
		return err
	}
	out, err := h.svc.UpdateKind(c.Request().Context(), cid, c.Param("kid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteKind handles DELETE /campaigns/:id/tracker-kinds/:kid.
func (h *Handler) DeleteKind(c echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.DeleteKind(c.Request().Context(), cid, c.Param("kid"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
