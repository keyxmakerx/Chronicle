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
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
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

	// promptBuilder assembles the "Copy AI Prompt" output. Optional
	// — nil renders an explanatory error in the prompt modal so the
	// operator sees a clear message instead of a panic.
	promptBuilder *prompt.Service

	// importLookup is the narrow contract the import classifier
	// needs from the entities service. Optional — nil produces an
	// "import not configured" message at parse time. Wired in
	// app/routes.go alongside the renderer + prompt builder.
	importLookup importer.CampaignLookup

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

// SetPromptBuilder wires the prompt service. Called by app/routes.go
// after the cross-plugin Service dependencies (entity lister, tag
// lister, the renderer reused as the content Exporter) are wired.
// Optional in test fixtures — nil produces a "service unavailable"
// modal at GeneratePrompt time rather than a panic.
func (h *Handler) SetPromptBuilder(b *prompt.Service) {
	h.promptBuilder = b
}

// SetImportLookup wires the campaign lookup the import classifier
// uses to detect slug conflicts + unknown entity-type categories.
// Optional — nil produces a "service unavailable" review fragment
// at parse time.
func (h *Handler) SetImportLookup(l importer.CampaignLookup) {
	h.importLookup = l
}

// ParseImport accepts multipart-or-textarea markdown input,
// parses it into per-page ParsedPage structs, classifies each
// against live campaign state (conflict / new category / etc),
// and returns the review fragment that the operator inspects
// before committing. NO entity is created in this PR — Submit
// in the review screen is inert until Phase 5 wires the commit
// handler.
//
// POST /campaigns/:id/ai-workspace/import/parse
//
// Form fields (multipart/form-data):
//   - markdown_paste: textarea contents (single page or multi-page)
//   - markdown_files: zero-or-more .md file uploads
//
// Files and paste content are concatenated with `\n\n` separators
// before parsing — pasted text comes first, then each file in the
// order the operator selected them.
func (h *Handler) ParseImport(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.importLookup == nil {
		return middleware.Render(c, http.StatusOK,
			ImportReview(ImportReviewData{
				CampaignID: cc.Campaign.ID,
				SummaryCounts: ReviewSummary{Total: 0},
			}))
	}

	body, err := readImportBody(c)
	if err != nil {
		return apperror.NewBadRequest(err.Error())
	}
	if strings.TrimSpace(body) == "" {
		return apperror.NewBadRequest("no markdown content found — paste text or upload .md files")
	}

	pages := importer.Parse(body)
	cls, err := importer.NewClassifier(h.importLookup, cc.Campaign.ID).
		ClassifyAll(c.Request().Context(), pages)
	if err != nil {
		slog.Error("ai-workspace: classify failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewInternal(err)
	}

	summary := ReviewSummary{Total: len(pages)}
	for _, c := range cls {
		switch c.Status {
		case importer.StatusNew:
			summary.Selectable++
		case importer.StatusConflict:
			summary.Selectable++
			summary.Conflicts++
		case importer.StatusNewCategory:
			summary.Selectable++
			summary.NewCategories++
		case importer.StatusParseError:
			summary.ParseErrors++
		}
	}

	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(),
			cc.Campaign.ID, "campaign.ai_import.parsed",
			map[string]any{
				"total_pages":     summary.Total,
				"selectable":      summary.Selectable,
				"conflicts":       summary.Conflicts,
				"new_categories":  summary.NewCategories,
				"parse_errors":    summary.ParseErrors,
				"input_byte_size": len(body),
			})
	}

	return middleware.Render(c, http.StatusOK, ImportReview(ImportReviewData{
		CampaignID:    cc.Campaign.ID,
		Pages:         pages,
		Classes:       cls,
		SummaryCounts: summary,
	}))
}

// readImportBody concatenates the textarea + every uploaded .md
// file's content. Caps total input at 5 MB to prevent a single
// import from chewing memory.
const importBodyCap = 5 * 1024 * 1024

func readImportBody(c echo.Context) (string, error) {
	var b strings.Builder
	pasted := strings.TrimSpace(c.FormValue("markdown_paste"))
	if pasted != "" {
		b.WriteString(pasted)
		b.WriteString("\n\n")
	}

	form, err := c.MultipartForm()
	if err != nil && err != http.ErrNotMultipart {
		return "", err
	}
	if form != nil {
		files := form.File["markdown_files"]
		for _, fh := range files {
			if b.Len()+int(fh.Size) > importBodyCap {
				return "", apperror.NewBadRequest(
					"total upload + paste exceeds 5 MB cap; split into smaller batches").Internal
			}
			f, err := fh.Open()
			if err != nil {
				return "", err
			}
			buf := make([]byte, fh.Size)
			if _, err := io.ReadFull(f, buf); err != nil {
				_ = f.Close()
				return "", err
			}
			_ = f.Close()
			b.Write(buf)
			b.WriteString("\n\n")
		}
	}

	return b.String(), nil
}

// GeneratePrompt renders the "Copy AI Prompt" markdown for the
// campaign owner and returns the modal fragment that displays it
// with a Copy button. Owner-gated at the route level
// (RequireRole(RoleOwner)). Reuses the same data-widget="ai-export"
// JS hook the Export modal uses (one widget, two consumers).
//
// GET /campaigns/:id/ai-workspace/prompt/generate
//
// Query params (all optional; defaults shown):
//   schema_types          ("on" → include entity-types section)
//   schema_categories     ("on" → include categories-in-use section)
//   schema_front_matter   ("on" → include front-matter example)
//   content_mode          "none" (default) | "all" | comma-separated
//                         category slugs
//   privacy               "safe" (default) | "permitted" | "everything"
//   gm_notes              "on" → include session GM notes in content
//   instruction           operator's free-text textarea contents
//
// Returns the prompt modal templ; any error from the builder surfaces
// in the modal's error region rather than a top-level apperror so
// the operator sees an in-modal failure they can correct.
func (h *Handler) GeneratePrompt(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.promptBuilder == nil {
		return middleware.Render(c, http.StatusOK,
			PromptModal("", "AI prompt builder is not configured on this server."))
	}

	in := prompt.Input{
		IncludeEntityTypes:        c.QueryParam("schema_types") == "on",
		IncludeCategoriesInUse:    c.QueryParam("schema_categories") == "on",
		IncludeFrontMatterExample: c.QueryParam("schema_front_matter") == "on",
		ContentMode:               strings.TrimSpace(c.QueryParam("content_mode")),
		Privacy:                   parsePrivacy(c.QueryParam("privacy")),
		IncludeSessionGMNotes:     c.QueryParam("gm_notes") == "on",
		OperatorInstruction:       c.QueryParam("instruction"),
	}
	if in.ContentMode == "" {
		in.ContentMode = "none"
	}

	userID := auth.GetUserID(c)
	out, err := h.promptBuilder.Build(c.Request().Context(),
		cc.Campaign.Name, userID, cc.Campaign.ID, in)
	if err != nil {
		slog.Error("ai-workspace: prompt generate failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return middleware.Render(c, http.StatusOK,
			PromptModal("", "Could not build prompt: "+err.Error()))
	}

	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(),
			cc.Campaign.ID, "campaign.ai_prompt.generated",
			map[string]any{
				"content_mode":        in.ContentMode,
				"privacy":             c.QueryParam("privacy"),
				"schema_types":        in.IncludeEntityTypes,
				"schema_categories":   in.IncludeCategoriesInUse,
				"schema_front_matter": in.IncludeFrontMatterExample,
				"include_gm_notes":    in.IncludeSessionGMNotes,
				"prompt_byte_count":   len(out),
			})
	}

	return middleware.Render(c, http.StatusOK, PromptModal(out, ""))
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
