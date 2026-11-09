// version_handler.go exposes a public, unauthenticated endpoint that returns
// the Chronicle build version. Used by external clients (Foundry VTT module
// dashboard) to display "Connected to Chronicle vX.Y.Z". The version is
// non-sensitive — no auth gate is necessary or desirable here.
package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// VersionHandler responds with the build version as JSON.
//
// GET /api/version  →  {"version": "<value>"} (200)
//
// Resolution lives in internal/hostinfo (this handler stays thin). Precedence:
// CHRONICLE_VERSION → the VCS revision compiled into the binary → the main
// module version → the literal "unknown".
func VersionHandler(c *echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"version": hostinfo.Version()})
}
