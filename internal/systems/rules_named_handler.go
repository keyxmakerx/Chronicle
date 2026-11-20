package systems

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
)

// NamedEntry sends the reader to the entry a kind and a name lead to, so a
// sheet that knows only the name can link to the entry's own page.
// GET /campaigns/:id/systems/:mod/named/:kind?name=
func (h *SystemHandler) NamedEntry(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	to, err := h.rulesPages().NamedAddress(c.Request().Context(), mod, c.Param("kind"), c.QueryParam("name"), rulesReader(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	if middleware.IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", to)
		return c.NoContent(http.StatusOK)
	}
	return c.Redirect(http.StatusSeeOther, to)
}

// NamedEntryPeek is the peek of the entry a kind and a name lead to; ?hero=
// marks that character's traits, as on the entry's own peek.
// GET /campaigns/:id/systems/:mod/named/:kind/peek?name=&hero=
func (h *SystemHandler) NamedEntryPeek(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	v, err := h.rulesPages().PeekNamed(c.Request().Context(), mod, c.Param("kind"), c.QueryParam("name"),
		c.QueryParam("hero"), rulesReader(c, cc))
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return middleware.Render(c, http.StatusOK, EntryPeekPanel(v))
}

// ToldAPI answers which of a page's named entries carry a change the players
// were told about lately, so one request marks every link on a sheet.
// GET /campaigns/:id/systems/:mod/told?n=<kind>:<name>&n=...
// -> {"told": ["<kind>:<name>", ...]}
func (h *SystemHandler) ToldAPI(c *echo.Context) error {
	cc, mod, err := h.rulesTarget(c)
	if err != nil {
		return err
	}
	var refs []NamedRef
	for _, n := range c.QueryParams()["n"] {
		kind, name, ok := strings.Cut(n, ":")
		if ok && kind != "" && name != "" {
			refs = append(refs, NamedRef{Kind: kind, Name: name})
		}
		if len(refs) == maxToldAsked {
			break
		}
	}
	told, err := h.rulesPages().Told(c.Request().Context(), mod, refs, rulesReader(c, cc))
	if err != nil {
		return err
	}
	out := make([]string, 0, len(told))
	for _, t := range told {
		out = append(out, t.Kind+":"+t.Name)
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return c.JSON(http.StatusOK, map[string]any{"told": out})
}
