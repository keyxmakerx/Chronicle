package decks

import (
	"bytes"
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// The routes behind the Card decks section and the draw overlay. Each reads
// through one service call; the player ones only reach the player types.

// ScreenComponent is the section body, for a caller (the DM Screen) that draws
// it inside its own card.
func ScreenComponent(v *ScreenView) templ.Component { return ScreenBody(v) }

// markup renders a component to a string for a JSON field.
func markup(ctx context.Context, c templ.Component) (string, error) {
	var buf bytes.Buffer
	if err := c.Render(ctx, &buf); err != nil {
		return "", apperror.NewInternal(err)
	}
	return buf.String(), nil
}

// ScreenFragment handles GET /campaigns/:id/decks/screen: the section body.
func (h *Handler) ScreenFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	view, err := h.svc.Screen(c.Request().Context(), cid, v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, ScreenBody(view))
}

// setPicksInput is the body of PUT /decks/screen/picks.
type setPicksInput struct {
	DeckIDs []string `json:"deckIds"`
}

// SetScreenPicks handles PUT /campaigns/:id/decks/screen/picks.
func (h *Handler) SetScreenPicks(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	var in setPicksInput
	if err := bind(c, &in, false); err != nil {
		return err
	}
	if err := h.svc.SetScreenPicks(c.Request().Context(), cid, v, in.DeckIDs); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

type shownJSON struct {
	CopyID string `json:"copyId"`
	HTML   string `json:"html"`
}

type drawViewJSON struct {
	DeckID    string      `json:"deckId"`
	Name      string      `json:"name"`
	Sub       string      `json:"sub"`
	Shape     string      `json:"shape"`
	Turn      string      `json:"turn"`
	Pile      *int        `json:"pile"`
	Fan       int         `json:"fan"`
	BackHTML  string      `json:"backHtml"`
	CanDraw   bool        `json:"canDraw"`
	Refusal   string      `json:"refusal"`
	DrawsLeft *int        `json:"drawsLeft"`
	HeroID    string      `json:"heroId"`
	Shown     []shownJSON `json:"shown"`
}

// DrawView handles GET /campaigns/:id/decks/:did/draw-view.
func (h *Handler) DrawView(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	dv, err := h.svc.DrawView(ctx, cid, c.Param("did"), v)
	if err != nil {
		return err
	}
	back, err := markup(ctx, CardBack(dv.Look, SizeHand))
	if err != nil {
		return err
	}
	out := drawViewJSON{DeckID: dv.DeckID, Name: dv.Name, Sub: dv.Sub, Shape: dv.Shape, Turn: dv.Turn, Pile: dv.Pile, Fan: dv.Fan,
		BackHTML: back, CanDraw: dv.CanDraw, Refusal: dv.Refusal, DrawsLeft: dv.DrawsLeft, HeroID: dv.HeroID, Shown: []shownJSON{}}
	for _, s := range dv.Shown {
		html, err := markup(ctx, drawnCard(dv.Look, s.Face))
		if err != nil {
			return err
		}
		out.Shown = append(out.Shown, shownJSON{CopyID: s.CopyID, HTML: html})
	}
	return c.JSON(http.StatusOK, out)
}

// ShownFaceFragment handles GET /campaigns/:id/decks/:did/copies/:copy/face.
func (h *Handler) ShownFaceFragment(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	card, err := h.svc.ShownFace(c.Request().Context(), cid, c.Param("did"), c.Param("copy"), v)
	if err != nil {
		return err
	}
	return middleware.Render(c, http.StatusOK, ShownFace(card))
}

type freshJSON struct {
	DeckID   string `json:"deckId"`
	CopyID   string `json:"copyId"`
	DeckName string `json:"deckName"`
	Sub      string `json:"sub"`
	BackHTML string `json:"backHtml"`
	Turn     string `json:"turn"`
	Shape    string `json:"shape"`
}

// FreshCards handles GET /campaigns/:id/decks/hand/fresh.
func (h *Handler) FreshCards(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	cards, err := h.svc.FreshCards(ctx, cid, v)
	if err != nil {
		return err
	}
	out := struct {
		Cards []freshJSON `json:"cards"`
	}{Cards: []freshJSON{}}
	for _, f := range cards {
		back, err := markup(ctx, CardBack(f.Look, SizeHand))
		if err != nil {
			return err
		}
		out.Cards = append(out.Cards, freshJSON{DeckID: f.DeckID, CopyID: f.CopyID, DeckName: f.DeckName, Sub: f.Sub,
			BackHTML: back, Turn: f.Look.Turn, Shape: f.Look.Shape})
	}
	return c.JSON(http.StatusOK, out)
}

// FreshSeen handles POST /campaigns/:id/decks/hand/fresh/:copy/seen.
func (h *Handler) FreshSeen(c *echo.Context) error {
	cid, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if err := h.svc.MarkFreshSeen(c.Request().Context(), cid, c.Param("copy"), v); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
