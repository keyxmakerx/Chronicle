// routes.go registers the AI Workspace plugin's HTTP routes onto a
// campaign-scoped Echo group. Mirrors the foundry_vtt
// RegisterOwnerRoutes pattern (cg *echo.Group already enforces
// campaign membership; per-route role gates layer on top).
//
// V1 Phase 2 mounts only the migrated /ai-export/generate route.
// Phases 3-5 add /ai-workspace/prompt/generate +
// /ai-workspace/import/parse + /ai-workspace/import/commit on the
// same group.

package ai_workspace

import (
	"github.com/labstack/echo/v4"
)

// RegisterOwnerRoutes mounts the per-campaign routes the AI Workspace
// plugin owns. Caller passes the same /campaigns/:id group used by
// foundry_vtt + the RequireRole(RoleOwner) middleware.
//
// V1 Phase 2: only the relocated /ai-export/generate route. URL
// preserved byte-for-byte from the campaigns-plugin registration
// (PR #350) so operator bookmarks + external monitoring keep
// working. The owner-gate AST pin in
// internal/wire/ai_export_route_test.go is updated alongside this
// PR to point at the new file path.
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	cg.GET("/ai-export/generate", h.GenerateAIExport, requireOwner)
}
