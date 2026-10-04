// Package ai_workspace owns Chronicle's AI co-pilot surface area:
// the Prompt builder, the AI Export feature, and the AI Import
// surface (parse, review, commit).
//
// The /campaigns/:id/ai-export/generate URL is preserved
// byte-for-byte from its original campaigns-plugin implementation.

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
	// renderer is the internal/aiexport orchestrator, constructed in
	// app/routes.go from every plugin's lister Service.
	renderer *aiexport.Service

	// promptBuilder assembles the "Copy AI Prompt" output. Optional
	// — nil renders an explanatory error in the prompt modal instead
	// of a panic.
	promptBuilder *prompt.Service

	// importLookup is the narrow contract the import classifier
	// needs from the entities service. Optional — nil produces an
	// "import not configured" message at parse time.
	importLookup importer.CampaignLookup

	// importCommitter creates entities + entity types from the
	// operator's confirmed review-screen decisions. Optional — nil
	// renders an explanatory failure summary instead of crashing.
	importCommitter *importer.Committer

	// audit is invoked once per successful Generate. Narrow
	// single-method surface rather than the concrete audit package,
	// to avoid pulling in unused machinery.
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

// NewHandler constructs the Handler. A nil renderer produces a
// "service unavailable" modal at GenerateAIExport time rather than
// a panic.
func NewHandler(renderer *aiexport.Service) *Handler {
	return &Handler{renderer: renderer}
}

// SetAuditLogger wires the audit hook. Optional — see AuditLogger
// docstring.
func (h *Handler) SetAuditLogger(a AuditLogger) {
	h.audit = a
}

// SetPromptBuilder wires the prompt service. Optional in test
// fixtures — nil produces a "service unavailable" modal at
// GeneratePrompt time rather than a panic.
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

// SetImportCommitter wires the committer that creates entities (+
// categories) from the operator's confirmed review-screen decisions.
// Optional — nil renders an explanatory failure result instead of
// crashing.
func (h *Handler) SetImportCommitter(c *importer.Committer) {
	h.importCommitter = c
}

// ParseImport accepts multipart-or-textarea markdown input,
// parses it into per-page ParsedPage structs, classifies each
// against live campaign state (conflict / new category / etc),
// and returns the review fragment that the operator inspects
// before committing. No entity is created here — Submit in the
// review screen invokes CommitImport separately.
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
		// readImportBody already returns operator-friendly wording;
		// pass it through verbatim.
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

// CommitImport runs the entity-creation pass after the operator reviews the
// parsed pages and submits the form. Per-row autonomy: one row's failure
// does not abort the rest. Owner-gated at the route level.
//
// POST /campaigns/:id/ai-workspace/import/commit
//
// Form fields: markdown_source, bulk_default_category,
// bulk_default_visibility (private|dm_only|public|"" for campaign default),
// bulk_default_conflict (rename|skip|overwrite), and per row
// page_N_include/name/category/visibility/conflict. An empty visibility
// (bulk or per-row) means no override was made, so new pages follow the
// campaign's configured DefaultVisibility (#729) rather than a hardcoded
// choice.
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
	// Empty means "no bulk override" — left for each row to resolve
	// against its own front matter, and ultimately the campaign default
	// (see Committer.commitRow). This used to default to "private",
	// hardcoding a choice nobody made and fighting public-default
	// campaigns (#729).
	bulkVisibility := strings.TrimSpace(c.FormValue("bulk_default_visibility"))
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
		// The review screen sends "update"; accept the legacy
		// "overwrite" value from in-flight sessions. TODO: drop this
		// alias once no in-flight sessions predate the rename.
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
		// Per-row action verb: form value, else the AI-suggested
		// action from front-matter, else "create".
		action := strings.TrimSpace(c.FormValue(prefix + "action"))
		if action == "" {
			action = pages[i].FrontMatter.Action
		}
		if action == "" {
			action = importer.ActionCreate
		}
		// Per-row delete confirmation; the committer re-checks this
		// independently against any client-side bypass.
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
		OwnerID:                userID,
		Pages:                  pages,
		Decisions:              decisions,
		CampaignDefaultPrivate: cc.Campaign.ParseSettings().DefaultsToPrivate(),
	})
	if err != nil {
		slog.Error("ai-workspace: commit failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewInternal(err)
	}

	if h.audit != nil {
		// Counts only — no names, no body content.
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

// GeneratePrompt renders the "Copy AI Prompt" markdown for the campaign
// owner and returns the modal fragment with a Copy button. Owner-gated at
// the route level. Reuses the data-widget="ai-export" JS hook the Export
// modal uses.
//
// GET /campaigns/:id/ai-workspace/prompt/generate
//
// Query params: schema_types/schema_categories/schema_front_matter ("on"),
// content_mode ("none"|"all"|category slugs), privacy
// ("safe"|"permitted"|"everything"), gm_notes ("on"), instruction (free text).
//
// A builder error surfaces in the modal's error region, not a top-level
// apperror, so the operator sees an in-modal failure they can correct.
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
// URL is preserved byte-for-byte so operator bookmarks and external
// monitoring keep working.
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
// The tab is Settings' "Data & AI": the campaign's own export and import
// first, then the AI tools. It takes the place of the built-in Data tab,
// which campaigns shows only when this plugin is not wired. Slot 50 sits
// between API keys (40) and Activity (60).
func (h *Handler) SettingsTabFactory() func(*campaigns.CampaignContext) campaigns.SettingsTab {
	return func(cc *campaigns.CampaignContext) campaigns.SettingsTab {
		return campaigns.SettingsTab{
			ID:        "ai-workspace",
			Label:     "Data & AI",
			Icon:      "fa-solid fa-database",
			MinRole:   campaigns.RoleOwner,
			SortOrder: 50,
			Content:   SettingsTabBody(cc),
		}
	}
}

// parsePrivacy maps the form-string ("safe" / "permitted" /
// "everything") to the typed aiexport.PrivacyMode constant. Unknown
// values fall back to Safe (most-restrictive default) so an
// unrecognised value can't silently widen the privacy filter.
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
