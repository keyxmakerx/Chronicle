// Package ai_workspace owns Chronicle's AI co-pilot surface area:
// the Prompt builder (Phase 3), the AI Export feature (V1 Phase 2 —
// migrated from internal/aiexport), and the AI Import surface
// (Phase 4-5).
//
// V1 Phase 2 ships only the Export side: the renderer package +
// settings-tab contribution + the GenerateAIExport handler. URL
// /campaigns/:id/ai-export/generate is preserved byte-for-byte from
// the campaigns-plugin implementation (PR #350). Wire snapshot
// reflects the file-column move (campaigns/routes.go →
// ai_workspace/routes.go); URL + method + auth chain unchanged.
//
// Per cordinator/decisions/2026-05-26-ai-workspace-plugin-design.md
// (locked vision) + cordinator/reports/chronicle/2026-05-26-c-ai-
// workspace-scoping.md §4 Phase 2.

package ai_workspace

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// Handler is the HTTP boundary for the AI Workspace plugin. Holds
// references to the cross-cutting renderer + the audit logger
// the campaigns handler used pre-migration. Per Chronicle's thin-
// handler convention: parse request, call service, render response.
type Handler struct {
	// renderer is the (relocated) internal/aiexport orchestrator.
	// Constructed in app/routes.go from every plugin's lister Service.
	renderer *aiexport.Service

	// audit is invoked once per successful Generate. Same shape as
	// the campaigns-plugin audit hook the V1 PR #350 used; we keep
	// the surface narrow (one method) rather than importing the
	// concrete audit package and pulling in unused machinery.
	audit AuditLogger
}

// AuditLogger is the narrow contract the plugin needs for audit
// events. Implemented by app/routes.go via a small adapter against
// the existing audit service (same shape as campaigns' campaignAudit
// adapter). Optional — nil disables audit emission, which is the
// default in test fixtures.
type AuditLogger interface {
	LogCampaignEvent(ctx context.Context, campaignID, action string, details map[string]any)
}

// NewHandler constructs the Handler. renderer is required for Phase 2;
// nil produces a "service unavailable" modal at GenerateAIExport time
// rather than a panic.
func NewHandler(renderer *aiexport.Service) *Handler {
	return &Handler{renderer: renderer}
}

// SetAuditLogger wires the audit hook. Optional — see AuditLogger
// docstring.
func (h *Handler) SetAuditLogger(a AuditLogger) {
	h.audit = a
}

// GenerateAIExport renders the AI-export markdown for the campaign
// owner and returns the modal fragment that displays it with a Copy
// button. Owner-gated at the route level (RequireRole(RoleOwner)).
//
// GET /campaigns/:id/ai-export/generate?privacy=safe&categories=...&gm_notes=on
//
// Functionally identical to the campaigns-plugin handler from PR #350;
// moved into this plugin per V1-B. Same query-param shape, same
// modal output, same error-in-modal failure surface. URL preserved
// byte-for-byte so operator bookmarks + external monitoring continue
// to work.
func (h *Handler) GenerateAIExport(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.renderer == nil {
		return middleware.Render(c, http.StatusOK,
			AIExportModal("", "AI-export is not configured on this server."))
	}

	opts := aiexport.Options{
		Privacy:               parsePrivacy(c.QueryParam("privacy")),
		IncludeSessionGMNotes: c.QueryParam("gm_notes") == "on",
	}
	if raw := c.QueryParam("categories"); raw != "" {
		for _, s := range strings.Split(raw, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				opts.Categories = append(opts.Categories, aiexport.Category(s))
			}
		}
	}

	userID := auth.GetUserID(c)
	markdown, err := h.renderer.Generate(c.Request().Context(),
		cc.Campaign.Name, userID, cc.Campaign.ID, opts)
	if err != nil {
		slog.Error("ai-workspace: export generate failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return middleware.Render(c, http.StatusOK,
			AIExportModal("", "Could not generate export: "+err.Error()))
	}

	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(),
			cc.Campaign.ID, "campaign.ai_export.generated",
			map[string]any{
				"privacy":             c.QueryParam("privacy"),
				"category_count":      len(opts.Categories),
				"include_gm_notes":    opts.IncludeSessionGMNotes,
				"markdown_byte_count": len(markdown),
			})
	}

	return middleware.Render(c, http.StatusOK, AIExportModal(markdown, ""))
}

// SettingsTabFactory returns the factory that the plugin registers
// with the campaigns settings tab registry. campaigns invokes the
// factory per-request with the live CampaignContext, so the tab's
// content closure can bind cc.Campaign.ID into the form URLs etc.
//
// Slot 55 lands the tab between AI Export (50, retired in this PR)
// and Activity (60) — see PR #351's TestRegisterSettingsTab_MergesAndSorts
// which already pins this slot for AI Workspace.
func (h *Handler) SettingsTabFactory() func(*campaigns.CampaignContext) campaigns.SettingsTab {
	return func(cc *campaigns.CampaignContext) campaigns.SettingsTab {
		return campaigns.SettingsTab{
			ID:        "ai-workspace",
			Label:     "AI Workspace",
			Icon:      "fa-solid fa-wand-magic-sparkles",
			MinRole:   campaigns.RoleOwner,
			SortOrder: 55,
			Content:   SettingsTabBody(cc),
		}
	}
}

// parsePrivacy maps the form-string ("safe" / "permitted" /
// "everything") to the typed aiexport.PrivacyMode constant. Unknown
// values fall back to Safe (most-restrictive default) so a future
// UI bug shipping an unrecognised value can't silently downgrade
// the privacy filter. Same behavior as the pre-migration adapter
// in app/routes.go.
func parsePrivacy(s string) aiexport.PrivacyMode {
	switch s {
	case "permitted":
		return aiexport.PrivacyModePermitted
	case "everything":
		return aiexport.PrivacyModeEverything
	default:
		return aiexport.PrivacyModeSafe
	}
}
