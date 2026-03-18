package middleware

import (
	"context"
	"net/http"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// LayoutInjector is a function that copies layout-relevant data from the Echo
// context (populated by auth/campaign middleware) into Go's context.Context so
// Templ templates can read it. Registered once at startup in app/routes.go.
//
// This callback pattern avoids the middleware package importing any plugin types.
var LayoutInjector func(echo.Context, context.Context) context.Context

// IsHTMX returns true if the current request was initiated by HTMX and is NOT
// a boosted navigation. Boosted requests (hx-boost="true") behave like normal
// page navigations — they expect full page responses so hx-select can extract
// the target element. Handlers use this to decide whether to return a fragment
// or full page.
func IsHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true" &&
		c.Request().Header.Get("HX-Boosted") != "true"
}

// HTMXRedirect sends a redirect that works for both HTMX and normal requests.
// For HTMX requests it sets the HX-Redirect header and returns 204 No Content
// (so HTMX performs a client-side redirect). For normal requests it returns a
// standard 303 See Other redirect.
func HTMXRedirect(c echo.Context, url string) error {
	if IsHTMX(c) {
		c.Response().Header().Set("HX-Redirect", url)
		return c.NoContent(http.StatusNoContent)
	}
	return c.Redirect(http.StatusSeeOther, url)
}

// Render writes a Templ component to the response with the given status code.
// Before rendering, it runs the LayoutInjector (if registered) to copy
// session/campaign data into the Go context for Templ templates to access.
func Render(c echo.Context, statusCode int, component templ.Component) error {
	ctx := c.Request().Context()

	// Inject layout data from Echo context into Go context for Templ.
	if LayoutInjector != nil {
		ctx = LayoutInjector(c, ctx)
	}

	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response().WriteHeader(statusCode)
	return component.Render(ctx, c.Response().Writer)
}
