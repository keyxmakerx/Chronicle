package app

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/updatehealth"
)

// updateHealthCounter counts each finished request against the changed
// features (internal/updatehealth). A request that returns an error is left
// to the error handler, which knows its final status; here a request that
// returned no error counts as a use below 400 and as a failure at 500 and
// above (a recovered panic). 4xx answers are neither: a missing page or a
// refused permission says nothing about whether the feature works.
func updateHealthCounter() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			err := next(c)
			if err != nil {
				return err
			}
			switch status := middleware.ResponseState(c).Status; {
			case status >= http.StatusInternalServerError:
				updatehealth.Fail(c.Request().Method, c.Path(), http.StatusText(status))
			case status < http.StatusBadRequest:
				updatehealth.Use(c.Request().Method, c.Path())
			}
			return nil
		}
	}
}
