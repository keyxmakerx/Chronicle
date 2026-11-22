package decks

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// The Director's moment routes. Each asks for a Director before anything is
// read, so a player or scribe is told the page is not there, in the same words
// as the workshop's pages; the service then checks again.

// SayInput is the body of the say route.
type SayInput struct {
	Said bool `json:"said"`
}

// Moments handles GET /campaigns/:id/decks/moments.
func (h *Handler) Moments(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.Moments(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"moments": out})
}

// WhenOptions handles GET /campaigns/:id/decks/when-options.
func (h *Handler) WhenOptions(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.WhenOptions(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// MomentDeal handles POST /campaigns/:id/decks/moments/:mid/deal.
func (h *Handler) MomentDeal(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.DealMoment(c.Request().Context(), cid, c.Param("mid"), v)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// answered writes the moment an answer returns, or its error.
func answered(c *echo.Context, out *MomentView, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}

// MomentAnother handles POST /campaigns/:id/decks/moments/:mid/another.
func (h *Handler) MomentAnother(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.AnotherMoment(c.Request().Context(), cid, c.Param("mid"), v)
	return answered(c, out, err)
}

// MomentInvite handles POST /campaigns/:id/decks/moments/:mid/invite.
func (h *Handler) MomentInvite(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.InviteMoment(c.Request().Context(), cid, c.Param("mid"), v)
	return answered(c, out, err)
}

// MomentDismiss handles POST /campaigns/:id/decks/moments/:mid/dismiss.
func (h *Handler) MomentDismiss(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	out, err := h.svc.DismissMoment(c.Request().Context(), cid, c.Param("mid"), v)
	return answered(c, out, err)
}

// Say handles POST /campaigns/:id/decks/:did/cards/:cid/say.
func (h *Handler) Say(c *echo.Context) error {
	cid, v, err := h.director(c)
	if err != nil {
		return err
	}
	var in SayInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	out, err := h.svc.Say(c.Request().Context(), cid, c.Param("did"), c.Param("cid"), v, in.Said)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, out)
}
