// version_handler.go exposes a public, unauthenticated endpoint that returns
// the Chronicle build version. Used by external clients (Foundry VTT module
// dashboard) to display "Connected to Chronicle vX.Y.Z". The version is
// non-sensitive — no auth gate is necessary or desirable here.
package syncapi

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

// VersionHandler responds with the build version as JSON.
// The single source of truth is the CHRONICLE_VERSION environment variable
// (also consumed by pre_migration_backup.writeManifest); we deliberately
// avoid introducing a second build-time symbol that could drift.
//
// GET /api/version  →  {"version": "<value>"} (200)
// Falls back to {"version": "unknown"} when the env var is unset, mirroring
// the manifest writer's behavior so version reporting is consistent across
// surfaces.
func VersionHandler(c echo.Context) error {
	v := os.Getenv("CHRONICLE_VERSION")
	if v == "" {
		v = "unknown"
	}
	return c.JSON(http.StatusOK, map[string]string{"version": v})
}
