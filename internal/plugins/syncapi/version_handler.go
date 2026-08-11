// version_handler.go exposes a public, unauthenticated endpoint that returns
// the Chronicle build version. Used by external clients (Foundry VTT module
// dashboard) to display "Connected to Chronicle vX.Y.Z". The version is
// non-sensitive — no auth gate is necessary or desirable here.
package syncapi

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/hostinfo"
)

// VersionHandler responds with the build version as JSON.
//
// GET /api/version  →  {"version": "<value>"} (200)
//
// The resolution rule lives in internal/hostinfo (this handler stays thin, and
// the host.build diagnostic prints the same answer by calling the same
// function). Precedence: CHRONICLE_VERSION → the VCS revision compiled into the
// binary → the main module version → the literal "unknown".
//
// WHY the fallback chain exists: the endpoint used to read CHRONICLE_VERSION
// and nothing else, and that variable is set by no Dockerfile, no compose file,
// no Makefile and no workflow — so every image ever shipped answered the
// literal string "unknown", and the Foundry dashboard displayed it. The VCS
// revision fills the gap on any binary built where `git` was available.
// "unknown" is still returned, honestly, when nothing identifies the build at
// all (which is the case for images from the current Docker builder, since
// golang:1.24-alpine ships no git and Go then skips stamping silently).
func VersionHandler(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"version": hostinfo.Version()})
}
