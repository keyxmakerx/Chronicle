package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/labstack/echo/v4"
)

// Recovery returns middleware that recovers from panics, logs the stack
// trace, and returns a 500 Internal Server Error to the client. This
// prevents a single panicking handler from crashing the entire server.
func Recovery() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) (returnErr error) {
			defer func() {
				if r := recover(); r != nil {
					// Log the panic with full stack trace for debugging.
					stack := debug.Stack()
					slog.Error("panic recovered",
						slog.Any("panic", r),
						slog.String("stack", string(stack)),
						slog.String("method", c.Request().Method),
						slog.String("path", c.Request().URL.Path),
					)

					// Return a generic error to the client.
					returnErr = c.String(
						http.StatusInternalServerError,
						fmt.Sprintf("Internal Server Error"),
					)
				}
			}()

			return next(c)
		}
	}
}
