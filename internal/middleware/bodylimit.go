package middleware

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

// BodyLimitFunc caps the request body at the size limit() returns, read on
// every request so a changed setting applies without a restart. skip chooses
// which requests it covers. A limit of 0 or less means no cap.
//
// It exists for upload routes whose cap depends on a live setting but which
// must be bounded before the CSRF middleware runs: CSRF reads the form field
// and so parses the whole multipart body before anyone is signed in.
// A declared length over the limit is refused with 413 straight away; an
// undeclared (chunked) body is cut off at the limit as it is read.
func BodyLimitFunc(limit func() int64, skip func(*echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if skip != nil && skip(c) {
				return next(c)
			}
			max := limit()
			if max <= 0 {
				return next(c)
			}
			req := c.Request()
			if req.ContentLength > max {
				return echo.ErrStatusRequestEntityTooLarge
			}
			req.Body = http.MaxBytesReader(c.Response(), req.Body, max)
			return next(c)
		}
	}
}
