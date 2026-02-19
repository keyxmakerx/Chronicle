package middleware

import (
	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"
)

// IsHTMX returns true if the current request was initiated by HTMX.
// Handlers use this to decide whether to return a fragment or full page.
func IsHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// Render writes a Templ component to the response with the given status code.
// This is a convenience wrapper used by all handlers for consistent rendering.
func Render(c echo.Context, statusCode int, component templ.Component) error {
	c.Response().Header().Set("Content-Type", "text/html; charset=utf-8")
	c.Response().WriteHeader(statusCode)
	return component.Render(c.Request().Context(), c.Response().Writer)
}
