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
	"strconv"
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

	// importCommitter is the Phase 5 orchestrator that creates
	// entities + entity types from the operator's confirmed
	// review-screen decisions. Optional — nil renders an
	// explanatory failure summary instead of crashing.
	importCommitter *importer.Committer

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

// SetImportCommitter wires the Phase-5 committer that creates
// entities (+ categories) from the operator's confirmed review-
// screen decisions. Optional — nil renders an explanatory failure
// result instead of crashing.
func (h *Handler) SetImportCommitter(c *importer.Committer) {
	h.importCommitter = c
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
		// readImportBody already returns friendly wording (5MB cap
		// hint, etc.); pass through verbatim to the BadRequest
		// surface. Technical detail (file-read failure paths) is
		// stowed in slog by callers up the stack.
		slog.Warn("ai-workspace: import body read failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewBadRequest(err.Error())
	}
	if strings.TrimSpace(body) == "" {
		return apperror.NewBadRequest("Nothing to import — paste markdown into the textarea or drop one or more .md files.")
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
		CampaignID:     cc.Campaign.ID,
		Pages:          pages,
		Classes:        cls,
		SummaryCounts:  summary,
		MarkdownSource: body,
	}))
}

// CommitImport runs the actual entity-creation pass after the
// operator reviews the parsed pages and submits the form. Per-row
// autonomy: one row's failure does not abort N+1..M. Owner-gated
// at the route level (RequireRole(RoleOwner)).
//
// POST /campaigns/:id/ai-workspace/import/commit
//
// Form fields (URL-encoded form data; review screen's <form>):
//   - markdown_source           hidden; original input markdown
//   - bulk_default_category     "" or entity-type slug
//   - bulk_default_visibility   "private" (default) | "dm_only" | "public"
//   - bulk_default_conflict     "rename" (default) | "skip" | "overwrite"
//   - page_N_include            "on" or absent
//   - page_N_name               text
//   - page_N_category           slug or "new:<slug>"
//   - page_N_visibility         enum
//   - page_N_conflict           enum (only for conflict rows)
//
// Returns the import_result fragment.
func (h *Handler) CommitImport(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.importCommitter == nil {
		return middleware.Render(c, http.StatusOK,
			ImportResult(ImportResultData{
				CampaignID: cc.Campaign.ID,
				Result: importer.CommitResult{
					Failed: 1,
					Rows: []importer.RowOutcome{{
						Status: importer.StatusFailed,
						Reason: "AI Import commit not configured on this server.",
					}},
				},
			}))
	}

	source := c.FormValue("markdown_source")
	if strings.TrimSpace(source) == "" {
		return apperror.NewBadRequest("Your import session expired. Please paste your markdown again.")
	}

	// Re-parse from scratch so the indexes match what the review
	// templ rendered. The classifier-supplied dropdowns embedded
	// the right values in the form, but the bodies need to come
	// from the markdown source.
	pages := importer.Parse(source)

	bulkCategory := strings.TrimSpace(c.FormValue("bulk_default_category"))
	bulkVisibility := strings.TrimSpace(c.FormValue("bulk_default_visibility"))
	if bulkVisibility == "" {
		bulkVisibility = "private"
	}
	bulkConflict := strings.TrimSpace(c.FormValue("bulk_default_conflict"))
	if bulkConflict == "" {
		bulkConflict = "rename"
	}

	decisions := make([]importer.RowDecision, len(pages))
	for i := range pages {
		prefix := "page_" + strconv.Itoa(i) + "_"
		category := strings.TrimSpace(c.FormValue(prefix + "category"))
		if category == "" {
			category = bulkCategory
		}
		conflict := strings.TrimSpace(c.FormValue(prefix + "conflict"))
		if conflict == "" {
			conflict = bulkConflict
		}
		// V1.5 backward-compat alias (C-AI-WORKSPACE-V1-G): the
		// review-screen rename Overwrite→Update means new submissions
		// use "update"; in-flight sessions opened before V1.5 still
		// send "overwrite". Accept both for one release; remove the
		// alias in V2.
		if conflict == "overwrite" {
			conflict = "update"
		}
		visibility := strings.TrimSpace(c.FormValue(prefix + "visibility"))
		if visibility == "" {
			visibility = bulkVisibility
		}
		name := strings.TrimSpace(c.FormValue(prefix + "name"))
		if name == "" {
			name = pages[i].Name
		}
		// V1.5: per-row action verb. Defaults to the AI-suggested
		// action from front-matter when the form field is absent
		// (i.e. UI didn't expose an override). Final fallback is
		// "create" — preserves V1 behavior for any path that never
		// goes through the V1.5 review screen.
		action := strings.TrimSpace(c.FormValue(prefix + "action"))
		if action == "" {
			action = pages[i].FrontMatter.Action
		}
		if action == "" {
			action = importer.ActionCreate
		}
		// V1.5: per-row Delete confirmation gate. Submit handler
		// believes the form-encoded value; committer re-checks
		// belt-and-suspenders for any client-side bypass.
		deleteConfirmed := c.FormValue(prefix+"delete_confirmed") == "on"

		decisions[i] = importer.RowDecision{
			Include:         c.FormValue(prefix+"include") == "on",
			Name:            name,
			CategorySpec:    category,
			Subcategory:     pages[i].FrontMatter.Subcategory,
			Visibility:      visibility,
			ConflictMode:    conflict,
			Action:          action,
			DeleteConfirmed: deleteConfirmed,
		}
	}

	userID := auth.GetUserID(c)
	result, err := h.importCommitter.Commit(c.Request().Context(), cc.Campaign.ID, importer.CommitInput{
		OwnerID:   userID,
		Pages:     pages,
		Decisions: decisions,
	})
	if err != nil {
		slog.Error("ai-workspace: commit failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewInternal(err)
	}

	if h.audit != nil {
		// Counts ONLY — no names, no body content. Per dispatch.
		// V1.5 (C-AI-WORKSPACE-V1-G) renames `overwrote` → `updated`
		// for vocabulary consistency with the new action verb; adds
		// `deleted` for action=delete completions. Counts-only
		// discipline preserved (V1-E).
		h.audit.LogCampaignEvent(c.Request().Context(),
			cc.Campaign.ID, "campaign.ai_import.committed",
			map[string]any{
				"created":                result.Created,
				"renamed":                result.Renamed,
				"updated":                result.Updated,
				"deleted":                result.Deleted,
				"skipped":                result.Skipped,
				"failed":                 result.Failed,
				"new_categories_created": len(result.NewCategoriesCreated),
				"new_categories_failed":  len(result.NewCategoriesFailed),
			})
	}

	return middleware.Render(c, http.StatusOK, ImportResult(ImportResultData{
		CampaignID: cc.Campaign.ID,
		Result:     result,
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
		return "", apperror.NewBadRequest(
			"Could not read the uploaded files. Check the file sizes and try again.").Internal
	}
	if form != nil {
		files := form.File["markdown_files"]
		for _, fh := range files {
			if b.Len()+int(fh.Size) > importBodyCap {
				return "", apperror.NewBadRequest(
					"Your paste + uploaded files exceed the 5 MB import limit. Try fewer pages per import.").Internal
			}
			f, err := fh.Open()
			if err != nil {
				return "", apperror.NewBadRequest(
					"Could not read the uploaded file — try again or paste the contents instead.").Internal
			}
			buf := make([]byte, fh.Size)
			if _, err := io.ReadFull(f, buf); err != nil {
				_ = f.Close()
				return "", apperror.NewBadRequest(
					"The uploaded file finished early or was incomplete. Try uploading it again.").Internal
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
			PromptModal("", "Could not build the prompt. Try again in a moment."))
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
			AIExportModal("", "Could not generate the export. Try again in a moment."))
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
