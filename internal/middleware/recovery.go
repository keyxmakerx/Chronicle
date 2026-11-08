package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/observability"
)

// Recovery returns middleware that recovers from panics, logs the stack
// trace, and returns a 500 Internal Server Error to the client. This
// prevents a single panicking handler from crashing the entire server.
func Recovery() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) (returnErr error) {
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

					// Also record it where an admin can read it without shell
					// access (host.errors). This hook is separate from
					// app.errorHandler because a recovered panic never reaches
					// that handler: the c.String below writes the 500 straight
					// to the response and returns nil, so Echo sees no error.
					//
					// The panic VALUE goes in; the stack does not — it's
					// already in the log line above, and kilobytes of frames
					// per entry would blow the ring's memory budget.
					observability.RecordPanic(
						c.Request().Method,
						c.Path(),
						c.Request().URL.Path,
						fmt.Sprint(r),
					)

					// Return a generic error to the client.
					returnErr = c.String(
						http.StatusInternalServerError,
						"Internal Server Error",
					)
				}
			}()

			return next(c)
		}
	}
}
