package decks

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// The player's fragments. Each reads through one service call that returns a
// player type, and renders from it; nothing here loads a Director type. A
// fragment a viewer may not read is a 404, like a deck or copy that is not
// there.

// HandFragment handles GET /campaigns/:id/decks/hand: the Hand leaf.
func (h *Handler) HandFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	view, err := h.svc.Hand(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, HandPanel(handPage{CampaignID: cid, View: view}))
}

// HeroStripFragment handles GET /campaigns/:id/decks/hand/hero/:eid: the Cards
// part of a hero's page.
func (h *Handler) HeroStripFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	view, err := h.svc.HeroHand(c.Request().Context(), cid, c.Param("eid"), v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, HeroStrip(stripPage{CampaignID: cid, View: view}))
}

// OpenCardFragment handles GET /campaigns/:id/decks/hand/:did/copies/:copy: a
// face-up card at full size with its controls.
func (h *Handler) OpenCardFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	card, err := h.svc.Open(c.Request().Context(), cid, c.Param("did"), c.Param("copy"), v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, HandOpened(openedPage{CampaignID: cid, Card: card}))
}

// TurnFaceFragment handles POST /campaigns/:id/decks/hand/:did/copies/:copy/turn:
// it turns the copy over and answers with its face, in one request.
func (h *Handler) TurnFaceFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	ctx, did, copyID := c.Request().Context(), c.Param("did"), c.Param("copy")
	card, err := h.svc.TurnFace(ctx, cid, did, copyID, v)
	// The draw overlay reads the card at open size, with its leaf. A card
	// drawn from a deck whose cards arrive face up is already turned, so the
	// overlay's turn reads it through the ordinary open route instead.
	if c.QueryParam("size") == "open" {
		if err != nil {
			opened, oerr := h.svc.Open(ctx, cid, did, copyID, v)
			if oerr != nil {
				return err
			}
			card = opened
		}
		return middleware.Render(c, http.StatusOK, ShownFace(card))
	}
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, TurnedFace(card))
}
