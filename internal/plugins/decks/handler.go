// The JSON HTTP surface for decks. Handlers bind, call the service and render;
// the service owns every rule, so a mis-wired route cannot widen access.
package decks

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler serves the deck routes.
type Handler struct{ svc Service }

// NewHandler builds the handler.
func NewHandler(svc Service) *Handler { return &Handler{svc: svc} }

func viewerFrom(c *echo.Context) (string, Viewer, error) {
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

// bind reads a JSON body of at most MaxBodyBytes into dst. An empty body is
// refused unless optional, for verbs that need nothing.
func bind(c *echo.Context, dst any, optional bool) error {
	// One byte past the cap so an oversize body is refused, not truncated.
	raw, err := io.ReadAll(io.LimitReader(c.Request().Body, MaxBodyBytes+1))
	if err != nil || len(raw) > MaxBodyBytes || (len(raw) == 0 && !optional) {
		return apperror.NewBadRequest("request body is missing or too large")
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		// A refusal that already names the field (an unknown look key) is
		// passed on as it is.
		var ae *apperror.AppError
		if errors.As(err, &ae) {
			return ae
		}
		return apperror.NewBadRequest("request body is not valid JSON")
	}
	return nil
}

// List handles GET /campaigns/:id/decks.
func (h *Handler) List(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.List(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Create handles POST /campaigns/:id/decks.
func (h *Handler) Create(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in CreateDeckInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.Create(c.Request().Context(), cid, v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// Get handles GET /campaigns/:id/decks/:did.
func (h *Handler) Get(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Get(c.Request().Context(), cid, c.Param("did"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Update handles PUT /campaigns/:id/decks/:did.
func (h *Handler) Update(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in UpdateDeckInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.Update(c.Request().Context(), cid, c.Param("did"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Delete handles DELETE /campaigns/:id/decks/:did.
func (h *Handler) Delete(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.Delete(c.Request().Context(), cid, c.Param("did"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// SetPlayers handles PUT /campaigns/:id/decks/:did/players.
func (h *Handler) SetPlayers(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in SetPlayersInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.SetPlayers(c.Request().Context(), cid, c.Param("did"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// CreateCard handles POST /campaigns/:id/decks/:did/cards.
func (h *Handler) CreateCard(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in CreateCardInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.CreateCard(c.Request().Context(), cid, c.Param("did"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, out)
}

// UpdateCard handles PUT /campaigns/:id/decks/:did/cards/:cid.
func (h *Handler) UpdateCard(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in UpdateCardInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.UpdateCard(c.Request().Context(), cid, c.Param("did"), c.Param("cid"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DeleteCard handles DELETE /campaigns/:id/decks/:did/cards/:cid.
func (h *Handler) DeleteCard(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.DeleteCard(c.Request().Context(), cid, c.Param("did"), c.Param("cid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Draw handles POST /campaigns/:id/decks/:did/draw.
func (h *Handler) Draw(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in DrawInput
	if err := bind(c, &in, true); err != nil {
		return err
	}
	out, err := h.svc.Draw(c.Request().Context(), cid, c.Param("did"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Act handles POST /campaigns/:id/decks/:did/copies/:copy/:verb.
func (h *Handler) Act(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in ActInput
	if err := bind(c, &in, true); err != nil {
		return err
	}
	out, err := h.svc.Act(c.Request().Context(), cid, c.Param("did"), c.Param("copy"), c.Param("verb"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// DealCard handles POST /campaigns/:id/decks/:did/deal-card.
func (h *Handler) DealCard(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in DealCardInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.DealCard(c.Request().Context(), cid, c.Param("did"), v, in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Shuffle handles POST /campaigns/:id/decks/:did/shuffle.
func (h *Handler) Shuffle(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Shuffle(c.Request().Context(), cid, c.Param("did"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Gather handles POST /campaigns/:id/decks/:did/gather.
func (h *Handler) Gather(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Gather(c.Request().Context(), cid, c.Param("did"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// Table handles GET /campaigns/:id/decks/table.
func (h *Handler) Table(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Table(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
