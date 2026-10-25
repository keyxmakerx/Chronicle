package middleware

import "github.com/labstack/echo/v4"

// BuildHeaderName carries the running build's token on every response.
const BuildHeaderName = "X-Chronicle-Build"

// BuildHeader stamps each response with the running build's token. A page
// keeps its first load's scripts across boosted navigation, which swaps only
// #main-content, so after a deploy an open tab would pair new page markup
// and CSS with old scripts. boot.js compares this header with the token the
// page loaded with and turns a mismatched boosted swap into a full load.
func BuildHeader(token string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set(BuildHeaderName, token)
			return next(c)
		}
	}
}
