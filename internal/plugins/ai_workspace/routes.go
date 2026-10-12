// routes.go registers the AI Workspace plugin's HTTP routes onto a
// campaign-scoped Echo group (cg already enforces campaign
// membership; per-route role gates layer on top).

package ai_workspace

import (
	"github.com/labstack/echo/v4"
)

// RegisterOwnerRoutes mounts the per-campaign routes the AI Workspace
// plugin owns. Caller passes the same /campaigns/:id group +
// RequireRole(RoleOwner) middleware used elsewhere. /ai-export/generate's
// URL is preserved byte-for-byte from its original campaigns-plugin
// registration so operator bookmarks + external monitoring keep
// working. Each route's owner-gate is AST-pinned in internal/wire/
// (ai_export_route_test.go, ai_workspace_prompt_route_test.go,
// ai_workspace_import_parse_route_test.go,
// ai_workspace_import_commit_route_test.go).
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	cg.GET("/ai-export/generate", h.GenerateAIExport, requireOwner)
	cg.GET("/ai-workspace/prompt/generate", h.GeneratePrompt, requireOwner)
	cg.GET("/ai-workspace/prompt/instructions", h.PromptInstructions, requireOwner)
	cg.POST("/ai-workspace/import/parse", h.ParseImport, requireOwner)
	cg.POST("/ai-workspace/import/commit", h.CommitImport, requireOwner)
}
