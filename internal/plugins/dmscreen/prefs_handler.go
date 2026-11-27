package dmscreen

import (
	"encoding/json"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// PrefsHandler serves a person's own screen preferences and the style kits'
// tokens the Style drawer previews with.
type PrefsHandler struct {
	svc PrefsService
}

// NewPrefsHandler returns a PrefsHandler backed by svc.
func NewPrefsHandler(svc PrefsService) *PrefsHandler {
	return &PrefsHandler{svc: svc}
}

// maxPrefsBody caps a PUT body a little above the stored document's cap, so
// an oversized one is refused before it is decoded.
const maxPrefsBody = maxPrefsBytes + 4<<10

// Get handles GET /campaigns/:id/screen-prefs/<screen>.
func (h *PrefsHandler) Get(screen string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cc := campaigns.GetCampaignContext(c)
		if cc == nil {
			return apperror.NewMissingContext()
		}
		v, err := h.svc.Get(c.Request().Context(), cc, auth.GetUserID(c), screen)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, v)
	}
}

// Put handles PUT /campaigns/:id/screen-prefs/<screen>: a partial update of
// the caller's own document (UpdateScreenPrefsInput).
func (h *PrefsHandler) Put(screen string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cc := campaigns.GetCampaignContext(c)
		if cc == nil {
			return apperror.NewMissingContext()
		}
		var in UpdateScreenPrefsInput
		if err := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, maxPrefsBody)).Decode(&in); err != nil {
			return apperror.NewBadRequest("invalid JSON body")
		}
		v, err := h.svc.Update(c.Request().Context(), cc, auth.GetUserID(c), screen, in)
		if err != nil {
			return err
		}
		return c.JSON(http.StatusOK, v)
	}
}

// KitsCSS handles GET /campaigns/:id/screen-prefs/<screen>/kits.css: every
// style kit's tokens and faces keyed on the dock's own data-dms-kit, so a
// kit picked in the Style drawer shows at once. The drawer fetches it once,
// when it first opens.
func (h *PrefsHandler) KitsCSS(screen string) echo.HandlerFunc {
	return func(c *echo.Context) error {
		cc := campaigns.GetCampaignContext(c)
		if cc == nil {
			return apperror.NewMissingContext()
		}
		if err := h.svc.Allowed(cc, screen); err != nil {
			return err
		}
		// The campaign's look reaches a context only on the way to a page
		// render; the tokens must be the ones that page is drawn with.
		ctx := c.Request().Context()
		if middleware.LayoutInjector != nil {
			ctx = middleware.LayoutInjector(c, ctx)
		}
		c.Response().Header().Set("Cache-Control", "private, no-cache")
		return c.Blob(http.StatusOK, "text/css; charset=utf-8", []byte(layouts.DockKitsCSS(ctx)))
	}
}
