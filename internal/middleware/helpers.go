package middleware

import (
	"context"

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
