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
// ai_workspace_import_commit_route_test.go,
// ai_workspace_import_tray_route_test.go, ai_page_route_test.go).
func RegisterOwnerRoutes(cg *echo.Group, h *Handler, requireOwner echo.MiddlewareFunc) {
	// The Your AI page (setup and history), and the AI button's counts and
	// panel, shown on every campaign page.
	cg.GET("/ai", h.AIPage, requireOwner)
	cg.GET("/ai/activity", h.AIActivity, requireOwner)
	cg.GET("/ai/chip", h.AIChip, requireOwner)
	cg.GET("/ai/panel", h.AIPanel, requireOwner)
	cg.POST("/ai/connection", h.ConnectAI, requireOwner)
	cg.POST("/ai/connection/disconnect", h.DisconnectAI, requireOwner)
	cg.POST("/ai/connection/pause", h.PauseAI, requireOwner)
	cg.POST("/ai/connection/resume", h.ResumeAI, requireOwner)
	cg.POST("/ai/connection/access", h.ChangeAIAccess, requireOwner)
	cg.POST("/ai/notes", h.ChangeNoteAccess, requireOwner)
	cg.POST("/ai/alerts/:aid", h.AnswerAIAlert, requireOwner)
	cg.POST("/ai/cut-off", h.CutOffAI, requireOwner)
	cg.POST("/ai/players", h.SetPlayersAI, requireOwner)
	cg.POST("/ai/players/:kid", h.PlayerKeyAction, requireOwner)
	cg.GET("/ai/requests", h.AIRequests, requireOwner)
	cg.POST("/ai/requests/:rid/turn-down", h.TurnDownRequest, requireOwner)

	cg.GET("/ai-export/generate", h.GenerateAIExport, requireOwner)
	cg.GET("/ai-workspace/prompt/generate", h.GeneratePrompt, requireOwner)
	cg.GET("/ai-workspace/prompt/instructions", h.PromptInstructions, requireOwner)
	cg.POST("/ai-workspace/import/parse", h.ParseImport, requireOwner)
	cg.POST("/ai-workspace/import/commit", h.CommitImport, requireOwner)
	cg.GET("/ai-workspace/import/tray", h.ImportTrayList, requireOwner)
	cg.POST("/ai-workspace/import/tray/clear", h.ClearImportTray, requireOwner)
}

// RegisterPlayerRoutes mounts a player's own AI: the AI button and panel on
// their screen, their key and their AI's changes. requireMember admits
// anyone in the campaign; each handler works on the signed-in person's own
// key and requests only, and on their own character only.
func RegisterPlayerRoutes(cg *echo.Group, h *Handler, requireMember echo.MiddlewareFunc) {
	cg.GET("/ai/me/chip", h.PlayerChip, requireMember)
	cg.GET("/ai/me/panel", h.PlayerPanel, requireMember)
	cg.GET("/ai/me/copy", h.PlayerCopy, requireMember)
	cg.POST("/ai/me/key", h.PlayerConnect, requireMember)
	cg.POST("/ai/me/key/disconnect", h.PlayerDisconnect, requireMember)
	cg.POST("/ai/me/paste", h.PlayerPaste, requireMember)
	cg.POST("/ai/me/apply", h.PlayerApply, requireMember)
	cg.GET("/ai/me/requests/:rid", h.PlayerReviewRequest, requireMember)
	cg.POST("/ai/me/requests/:rid/turn-down", h.PlayerTurnDown, requireMember)
}
