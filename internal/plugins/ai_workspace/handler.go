// Package ai_workspace owns Chronicle's AI co-pilot surface area:
// the Prompt builder, the AI Export feature, and the AI Import
// surface (parse, review, commit).
//
// The /campaigns/:id/ai-export/generate URL is preserved
// byte-for-byte from its original campaigns-plugin implementation.

package ai_workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/importer"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/prompt"
	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/records"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
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

	// records writes the non-page blocks (calendar events, tables, pins…).
	// Optional — nil leaves such blocks reported as not supported.
	records *records.Registry

	// lookups answers an AI's read-only `kind: lookup` blocks. Optional —
	// nil answers each one with "can't be looked up on this server".
	lookups *records.Lookups

	// featureGate names the campaign features switched off in Game &
	// features, by the kinds that write into them. Optional — nil treats
	// every feature as on.
	featureGate func(ctx context.Context, campaignID string) records.OffFeatures

	// importEras lets the review check an `era:` block's era and page before
	// anything is written. Optional — nil marks such blocks unavailable.
	importEras importer.EraLookup

	// importFolders lets the review check a page's `folder:` name. Optional —
	// nil refuses a page that names a folder.
	importFolders importer.FolderAccess
	// zone is the owner's time zone, for the Your AI page's times.
	zone func(ctx context.Context, userID string) *time.Location

	// memory remembers what AI Import applied in each campaign: the labelled
	// parts of a split answer, and a fingerprint of every block. Optional —
	// nil reads labels but can't tell what came before.
	memory ImportMemory

	// noteAccess keeps the owner's switches for which shared notes the AI
	// may change. Optional — nil leaves the switches off the page.
	noteAccess NoteAccessStore

	// connector makes and revokes the keys an AI app connects with.
	// Optional — nil leaves the Your AI page with copy and paste only.
	connector *connector.Service
	// baseURL is the site's public address, for the connector's address.
	baseURL string
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

// SetImportEras wires the era lookup the review uses for `era:` blocks. The
// committer gets its writer separately (Committer.SetEraWriter).
func (h *Handler) SetImportEras(l importer.EraLookup) {
	h.importEras = l
}

// SetImportFolders wires the folder lookup the review uses for `folder:` on
// pages. The committer gets its own through Committer.SetFolders.
func (h *Handler) SetImportFolders(f importer.FolderAccess) {
	h.importFolders = f
}

// SetRecords wires the record kinds.
func (h *Handler) SetRecords(r *records.Registry) {
	h.records = r
}

// SetLookups wires the read-only lookups.
func (h *Handler) SetLookups(l *records.Lookups) {
	h.lookups = l
}

// SetFeatureGate wires the switched-off feature check the prompt and the
// review use.
func (h *Handler) SetFeatureGate(g func(ctx context.Context, campaignID string) records.OffFeatures) {
	h.featureGate = g
}

// offFeatures is what the campaign has switched off, or nil.
func (h *Handler) offFeatures(ctx context.Context, campaignID string) records.OffFeatures {
	if h.featureGate == nil {
		return nil
	}
	return h.featureGate(ctx, campaignID)
}

// actorFor is the operator as the record kinds see them.
func actorFor(c echo.Context, cc *campaigns.CampaignContext) records.Actor {
	return records.Actor{UserID: auth.GetUserID(c), Role: cc.VisibilityRole()}
}

// viewerFor is the operator as page-link resolution checks access.
func viewerFor(c echo.Context, cc *campaigns.CampaignContext) importer.Viewer {
	return importer.Viewer{Role: cc.VisibilityRole(), UserID: auth.GetUserID(c)}
}

// rollTables is what a page's {roll{Name}} line can name: the campaign's
// rolling tables and those the paste's ticked table blocks make (include
// nil means all of them, for the review). Only the owner or a co-DM gets
// any, since the roller is a DM tool; for anyone else every such line stays
// text, with a warning.
func (h *Handler) rollTables(c echo.Context, cc *campaigns.CampaignContext, recs []importer.ParsedPage, include func(int) bool) map[string]string {
	if h.records == nil || !actorFor(c, cc).CanAuthorDmOnly() {
		return nil
	}
	var made []records.Record
	for i, p := range recs {
		if include == nil || include(i) {
			made = append(made, toRecord(i, p, nil))
		}
	}
	return h.records.Without(h.offFeatures(c.Request().Context(), cc.Campaign.ID)).RollTableIDs(c.Request().Context(), cc.Campaign.ID, made)
}

// exportActor is who the export lists records as. Safe mode, the default,
// lists them as a player sees them, so hidden events and pins, rolling
// tables and Director-only rules stay out, as for every other category.
func exportActor(c echo.Context, cc *campaigns.CampaignContext, mode aiexport.PrivacyMode) records.Actor {
	a := actorFor(c, cc)
	if mode == aiexport.PrivacyModeSafe {
		a.Role = int(campaigns.RolePlayer)
	}
	return a
}

// splitImport separates pages from records, keeping each list in input
// order so the review form's indexes match a re-parse at commit.
func splitImport(all []importer.ParsedPage) (pages, recs []importer.ParsedPage) {
	for _, p := range all {
		if p.IsRecord() && p.Status != importer.StatusParseError {
			recs = append(recs, p)
		} else {
			pages = append(pages, p)
		}
	}
	return pages, recs
}

// splitLookups takes out the `kind: lookup` blocks, which are answered
// instead of reviewed.
func splitLookups(all []importer.ParsedPage) (rest, lookups []importer.ParsedPage) {
	for _, p := range all {
		if strings.EqualFold(strings.TrimSpace(p.FrontMatter.Kind), records.KindLookup) && p.Status != importer.StatusParseError {
			lookups = append(lookups, p)
		} else {
			rest = append(rest, p)
		}
	}
	return rest, lookups
}

// answerLookups reads what the AI asked for, as the chosen privacy mode
// allows (Safe by default), and renders the answer panel. Nothing is
// written; the other pasted blocks are left out and counted.
func (h *Handler) answerLookups(c echo.Context, cc *campaigns.CampaignContext, blocks []importer.ParsedPage, skipped int) error {
	privacy := parsePrivacy(c.FormValue("lookup_privacy"))
	recs := make([]records.Record, len(blocks))
	for i, p := range blocks {
		recs[i] = toRecord(i, p, nil)
	}
	l := h.lookups
	if l == nil {
		l = &records.Lookups{}
	}
	answers := l.Answer(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, privacy), recs)
	md := records.Markdown(answers)
	if h.audit != nil {
		// Counts only — never what was asked or answered.
		failed := 0
		for _, a := range answers {
			if a.Error != "" {
				failed++
			}
		}
		h.audit.LogCampaignEvent(c.Request().Context(), cc.Campaign.ID, "campaign.ai_lookup.answered",
			map[string]any{
				"lookups":           len(answers),
				"unanswered":        failed,
				"privacy":           privacy.String(),
				"answer_byte_count": len(md),
			})
	}
	return middleware.Render(c, http.StatusOK, LookupAnswer(LookupView{
		CampaignID: cc.Campaign.ID,
		Answers:    answers,
		Markdown:   md,
		Privacy:    privacy.String(),
		Skipped:    skipped,
	}))
}

// toRecord builds a records.Record from a parsed block.
func toRecord(i int, p importer.ParsedPage, links *importer.PageLinks) records.Record {
	fields := make(map[string]any, len(p.Fields))
	for k, v := range p.Fields {
		fields[strings.ToLower(k)] = v
	}
	return records.Record{
		Index: i, Kind: p.FrontMatter.Kind, Action: p.FrontMatter.Action,
		Name: p.Name, Fields: fields, Body: p.Body, Links: links,
	}
}

// planRecords plans every record for the review screen.
func (h *Handler) planRecords(c echo.Context, cc *campaigns.CampaignContext, recs []importer.ParsedPage, links *importer.PageLinks, pasteSlugs map[string]bool) []RecordRow {
	rows := make([]RecordRow, len(recs))
	all := make([]records.Record, len(recs))
	for i, p := range recs {
		all[i] = toRecord(i, p, nil)
		all[i].Made = pasteSlugs
		rows[i] = RecordRow{
			Index: i, Label: all[i].Kind, Action: all[i].Action, Name: all[i].Name,
			Block: p.SourceIndex + 1, Reason: all[i].Str("reason"),
		}
	}
	if h.records == nil {
		for i := range rows {
			rows[i].Plan = records.Plan{Error: "this server cannot import " + all[i].Kind}
		}
		return rows
	}
	kinds, plans := h.records.PlanAll(c.Request().Context(), cc.Campaign.ID, actorFor(c, cc), all)
	off := h.offFeatures(c.Request().Context(), cc.Campaign.ID)
	for i, k := range kinds {
		if k != nil {
			rows[i].Label = k.Label()
			if t, ok := k.(records.Titled); ok && rows[i].Name == "" {
				rows[i].Name = t.Title(all[i])
			}
		}
		rows[i].Plan = plans[i]
		if f := off.For(all[i]); f != "" && rows[i].Plan.Error == "" {
			rows[i].Plan = records.Plan{Error: records.OffFeatureReason(f)}
		}
		if links != nil && rows[i].Plan.Error == "" {
			rows[i].Plan.Warnings = append(rows[i].Plan.Warnings, importer.LinkWarnings(links, recs[i].Body, pasteSlugs)...)
		}
	}
	return rows
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
				CampaignID:    cc.Campaign.ID,
				SummaryCounts: ReviewSummary{Total: 0},
			}))
	}

	body, err := readImportBody(c)
	// "Review what's here" in the tray reviews an answer's waiting parts
	// before the rest arrive.
	trayKey := strings.TrimSpace(c.FormValue("tray_review"))
	if err == nil && trayKey != "" && strings.TrimSpace(body) == "" {
		body, err = h.trayText(c.Request().Context(), cc.Campaign.ID, trayKey)
	}
	// Review on a request a connected AI made reviews its blocks, read from
	// the campaign's own waiting requests, never from the form.
	var requestID int64
	if err == nil && trayKey == "" && strings.TrimSpace(body) == "" && c.FormValue("ai_request") != "" {
		q, qerr := h.waitingRequest(c, cc, c.FormValue("ai_request"))
		if qerr != nil {
			return qerr
		}
		body, requestID = q.Body, q.ID
	}
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

	all := importer.Parse(body)
	// Several parts pasted together go in their own order, so a part
	// always comes after the parts it needs. The commit re-parses this text.
	if ordered, ok := importer.OrderParts(all); ok {
		body, all = ordered, importer.Parse(ordered)
	}
	// A part of an answer that isn't all in yet waits in the tray; the last
	// part brings the waiting ones with it.
	if trayKey == "" && requestID == 0 {
		var notice *trayNotice
		body, all, notice, err = h.trayStep(c.Request().Context(), cc.Campaign.ID, auth.GetUserID(c), body, all)
		if err != nil {
			return err
		}
		if notice != nil {
			return middleware.Render(c, http.StatusOK, TrayNotice(cc.Campaign.ID, *notice,
				h.trayAnswers(c.Request().Context(), cc.Campaign.ID)))
		}
	}
	lab := importer.TakeParts(all)
	parsed, lookups := splitLookups(lab.Blocks)
	if len(lookups) > 0 {
		if requestID != 0 {
			// The connector refuses these when they are sent; one here
			// would be answered with the owner's privacy, not the AI's.
			return apperror.NewBadRequest("this request holds lookups, which can't be reviewed; turn it down")
		}
		return h.answerLookups(c, cc, lookups, len(parsed))
	}
	pages, recs := splitImport(parsed)
	links := importer.NewPageLinks(c.Request().Context(), h.importLookup, cc.Campaign.ID, viewerFor(c, cc))
	rollTables := h.rollTables(c, cc, recs, nil)
	links.SetRollTables(rollTables)
	pasteSlugs := importer.PasteSlugs(pages)
	recRows := h.planRecords(c, cc, recs, links, pasteSlugs)
	classifier := importer.NewClassifier(h.importLookup, cc.Campaign.ID)
	if h.importEras != nil {
		classifier.WithEras(h.importEras, viewerFor(c, cc))
	}
	classifier.WithFolders(h.importFolders, pendingFolderNames(recs))
	classifier.WithRollTables(rollTables)
	if has := h.foundryHas(c.Request().Context(), cc.Campaign.ID, actorFor(c, cc)); has != nil {
		classifier.WithFoundryMonsters(has)
	}
	cls, err := classifier.ClassifyAll(c.Request().Context(), pages)
	if err != nil {
		slog.Error("ai-workspace: classify failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewInternal(err)
	}

	// An unknown @[Name] only warns; at commit it stays plain text.
	for i := range pages {
		if pages[i].Status != importer.StatusParseError && pages[i].FrontMatter.Action != importer.ActionDelete {
			pages[i].Warnings = append(pages[i].Warnings, importer.LinkWarnings(links, pages[i].Body, pasteSlugs)...)
			pages[i].Warnings = append(pages[i].Warnings, importer.RollWarnings(links, pages[i].Body, cls[i].ConflictEntity)...)
		}
	}
	notes := h.checkParts(c.Request().Context(), cc.Campaign.ID, lab)
	repeats := h.repeatedBlocks(c.Request().Context(), cc.Campaign.ID, lab.Blocks)
	for i := range recRows {
		recRows[i].Repeat = repeats[recs[i].SourceIndex]
	}

	summary := ReviewSummary{Total: len(pages) + len(recRows), Repeats: len(repeats)}
	for _, r := range recRows {
		if r.Plan.Error == "" {
			summary.Selectable++
		} else {
			summary.Blocked++
		}
	}
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
		case importer.StatusParseError, importer.StatusParentProblem:
			summary.ParseErrors++
		case importer.StatusFieldProblem:
			summary.Blocked++
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
				"blocked_records": summary.Blocked,
				"input_byte_size": len(body),
				"records":         len(recRows),
				"parts":           len(lab.Parts),
				"repeats":         summary.Repeats,
			})
	}

	host := pasteReviewHost
	if requestID != 0 {
		host = requestHost(requestID)
	}
	return middleware.Render(c, http.StatusOK, reviewIn(ImportReviewData{
		RequestID:      requestID,
		Host:           host,
		CampaignID:     cc.Campaign.ID,
		Pages:          pages,
		Classes:        cls,
		SummaryCounts:  summary,
		MarkdownSource: body,
		Records:        recRows,
		EngineSrc:      layouts.AssetURL("/static/js/widgets/chronicle_gen.js"),
		Parts:          notes,
		Repeats:        repeats,
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
	lab := importer.TakeParts(importer.Parse(source))
	pages, recs := splitImport(lab.Blocks)

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
		// The review screen sends "update"; a review page opened before
		// the option was renamed still sends "overwrite".
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
		// Numbers the director took from the game system's formulas.
		pages[i].FrontMatter.Fields = importer.UseSheetFormula(pages[i].FrontMatter.Fields, c.FormValue(prefix+"sheet_formula"))

		decisions[i] = importer.RowDecision{
			Include:         c.FormValue(prefix+"include") == "on",
			Name:            name,
			CategorySpec:    category,
			Subcategory:     pages[i].FrontMatter.Subcategory,
			Visibility:      visibility,
			ConflictMode:    conflict,
			Action:          action,
			DeleteConfirmed: deleteConfirmed,
			// Re-checked by the committer against the live page.
			ReplaceGMOnlyConfirmed: c.FormValue(prefix+"replace_gm_confirmed") == "on",
		}
	}

	early := h.commitEarlyFolders(c, cc, recs)

	userID := auth.GetUserID(c)
	var result importer.CommitResult
	var err error
	if len(pages) > 0 {
		result, err = h.importCommitter.Commit(c.Request().Context(), cc.Campaign.ID, importer.CommitInput{
			OwnerID:                userID,
			Pages:                  pages,
			Decisions:              decisions,
			CampaignDefaultPrivate: cc.Campaign.ParseSettings().DefaultsToPrivate(),
			Viewer:                 viewerFor(c, cc),
			RollTables: h.rollTables(c, cc, recs, func(i int) bool {
				return c.FormValue("rec_"+strconv.Itoa(i)+"_include") == "on"
			}),
		})
	}
	if err != nil {
		slog.Error("ai-workspace: commit failed",
			slog.String("campaign_id", cc.Campaign.ID),
			slog.Any("error", err))
		return apperror.NewInternal(err)
	}
	// Records run after pages, so a record may name a page this same
	// import just created. Folders are the exception: pages file themselves
	// into folders, so folders are made and renamed first.
	pageRows := len(result.Rows)
	recCounts := h.commitRecords(c, cc, recs, &result, early)
	done := h.rememberApplied(c.Request().Context(), cc.Campaign.ID, result, pageRows, pages, recs)
	h.markApplied(c.Request().Context(), cc.Campaign.ID, userID, lab, done)

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
				"records_applied":        recCounts.applied,
				"records_failed":         recCounts.failed,
			})
	}

	res := ImportResultData{CampaignID: cc.Campaign.ID, Result: result}
	if id, perr := strconv.ParseInt(c.FormValue("ai_request"), 10, 64); perr == nil && id > 0 && h.connector != nil {
		applied := result.Created + result.Renamed + result.Updated + result.Deleted
		outcome := fmt.Sprintf("%d applied, %d left out", applied, result.Skipped)
		if result.Failed > 0 {
			outcome += fmt.Sprintf(", %d failed", result.Failed)
		}
		if err := h.connector.Answered(c.Request().Context(), cc.Campaign.ID, id, outcome); err != nil {
			slog.Warn("ai-workspace: marking request answered",
				slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		}
		v, err := h.connectView(c, cc)
		if err != nil {
			return err
		}
		// The card the review sat in goes with the answered request, so the
		// result lands above the list instead.
		c.Response().Header().Set("HX-Retarget", "#"+requestResultHost)
		c.Response().Header().Set("HX-Reswap", "innerHTML")
		return middleware.Render(c, http.StatusOK, requestReviewed(res, v))
	}
	return middleware.Render(c, http.StatusOK, ImportResult(res))
}

// waitingRequest is one of the campaign's waiting requests, by the id the
// page sent.
func (h *Handler) waitingRequest(c echo.Context, cc *campaigns.CampaignContext, raw string) (*connector.Request, error) {
	if h.connector == nil {
		return nil, apperror.NewBadRequest("connecting an AI is not available on this Chronicle site")
	}
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return nil, apperror.NewBadRequest("that request isn't here any more")
	}
	return h.connector.WaitingRequest(c.Request().Context(), cc.Campaign.ID, id)
}

type recordCounts struct{ applied, failed int }

// commitRecords applies each included record and appends its outcome to
// the result. A delete needs its own confirmation tick, checked here again
// whatever the browser sent; every record is re-planned inside Apply, so
// nothing the review screen showed is trusted at commit.
func (h *Handler) commitRecords(c echo.Context, cc *campaigns.CampaignContext, recs []importer.ParsedPage, result *importer.CommitResult, early map[int]importer.RowOutcome) recordCounts {
	var n recordCounts
	// Pages are saved by now, so a record's @[Name] resolves against the
	// campaign alone.
	var links *importer.PageLinks
	if h.importLookup != nil {
		links = importer.NewPageLinks(c.Request().Context(), h.importLookup, cc.Campaign.ID, viewerFor(c, cc))
	}
	off := h.offFeatures(c.Request().Context(), cc.Campaign.ID)
	base := len(result.Rows)
	for i, p := range recs {
		out, done := early[i]
		if !done {
			out = h.commitRecord(c, cc, i, p, links, off)
		}
		out.Index = base + i
		switch out.Status {
		case importer.StatusFailed:
			result.Failed++
			n.failed++
		case importer.StatusSkipped:
			result.Skipped++
		case importer.StatusDeleted:
			result.Deleted++
			n.applied++
		case importer.StatusUpdated:
			result.Updated++
			n.applied++
		default:
			result.Created++
			n.applied++
		}
		result.Rows = append(result.Rows, out)
	}
	return n
}

// commitRecord applies one record, or says why it was not.
// A record for a feature in off is refused again here, since the feature
// may have been switched off after the review.
func (h *Handler) commitRecord(c echo.Context, cc *campaigns.CampaignContext, i int, p importer.ParsedPage, links *importer.PageLinks, off records.OffFeatures) importer.RowOutcome {
	prefix := "rec_" + strconv.Itoa(i) + "_"
	rec := toRecord(i, p, links)
	out := importer.RowOutcome{Name: rec.Name}
	k, ok := h.records.Get(rec.Kind)
	if ok {
		out.Name = k.Label() + ": " + rec.Name
	}
	switch {
	case c.FormValue(prefix+"include") != "on":
		out.Status, out.Reason = importer.StatusSkipped, "Excluded by operator"
	case h.records == nil || !ok:
		out.Status, out.Reason = importer.StatusFailed, "This server cannot import "+rec.Kind
	case rec.Action == records.ActionDelete && c.FormValue(prefix+"delete_confirmed") != "on":
		out.Status, out.Reason = importer.StatusFailed, "Removal was not confirmed"
	case records.RefuseGMMarkers(rec) != nil:
		out.Status, out.Reason = importer.StatusFailed, importer.SecretNotInTextReason
	case off.For(rec) != "":
		out.Status, out.Reason = importer.StatusFailed, records.OffFeatureReason(off.For(rec))
	default:
		rec.Generated = c.FormValue(prefix + "generated")
		if err := k.Apply(c.Request().Context(), cc.Campaign.ID, actorFor(c, cc), rec); err != nil {
			slog.Warn("ai-workspace: record failed",
				slog.String("campaign_id", cc.Campaign.ID),
				slog.String("kind", rec.Kind), slog.Any("error", err))
			out.Status, out.Reason = importer.StatusFailed, recordError(err)
			break
		}
		switch rec.Action {
		case records.ActionDelete:
			out.Status = importer.StatusDeleted
		case records.ActionUpdate:
			out.Status = importer.StatusUpdated
		default:
			out.Status = importer.StatusCreated
		}
	}
	return out
}

// isEarlyFolder is a folder block that makes or changes a folder, which
// must be done before the pages that name it.
func isEarlyFolder(p importer.ParsedPage) bool {
	return strings.EqualFold(strings.TrimSpace(p.FrontMatter.Kind), "folder") && p.FrontMatter.Action != records.ActionDelete
}

// commitEarlyFolders applies the folder blocks that pages may depend on, in
// paste order, and keeps their outcomes for the result list.
func (h *Handler) commitEarlyFolders(c echo.Context, cc *campaigns.CampaignContext, recs []importer.ParsedPage) map[int]importer.RowOutcome {
	early := map[int]importer.RowOutcome{}
	if h.records == nil {
		return early
	}
	for i, p := range recs {
		if isEarlyFolder(p) {
			early[i] = h.commitRecord(c, cc, i, p, nil, nil)
		}
	}
	return early
}

// pendingFolderNames are the category-and-name keys of folders this paste makes
// or renames to, so the review accepts a page that files itself in one of its
// own category only.
func pendingFolderNames(recs []importer.ParsedPage) map[string]bool {
	out := map[string]bool{}
	for _, p := range recs {
		if !isEarlyFolder(p) {
			continue
		}
		r := toRecord(0, p, nil)
		for _, n := range []string{r.Name, r.Str("rename_to")} {
			if n != "" {
				out[importer.PendingFolderKey(r.Str("category"), n)] = true
			}
		}
	}
	return out
}

// recordError shows a refusal's own wording (the kinds and services word
// them for people) and hides anything else, such as a database error.
func recordError(err error) string {
	if errors.Is(err, importer.ErrSecretNotInText) {
		return importer.SecretNotInTextReason
	}
	var ae *apperror.AppError
	if errors.As(err, &ae) && ae.Code < 500 {
		return ae.Message
	}
	return "Could not be saved; try again in a moment"
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
// content_mode ("none"|"all"|"house-rules"|category slugs), privacy
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
	if c.QueryParam("instructions_saved") == "on" {
		in.Instructions = prompt.InstructionsSaved
	}
	if in.ContentMode == "" {
		in.ContentMode = "none"
	}
	// A switched-off feature's kinds are left out, and named, so the AI
	// doesn't write what the owner couldn't open.
	off := h.offFeatures(c.Request().Context(), cc.Campaign.ID)
	in.Today = time.Now().Format("Monday 2 January 2006")
	if h.lookups != nil {
		in.Lookups = records.LookupDoc()
		in.PageIndex = h.lookups.PageIndex(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, in.Privacy))
		if off[records.KindCalendar] == "" {
			in.WorldToday = h.lookups.WorldToday(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, in.Privacy))
		}
	}
	if h.records != nil {
		reg := h.records.Without(off)
		in.Capabilities = reg.Capabilities()
		if note := off.Note(); note != "" {
			in.Capabilities += "\n" + note + "\n"
		}
		if in.IncludeFrontMatterExample {
			in.RecordDocs = reg.Docs()
		}
		if in.ContentMode == prompt.ContentHouseRules {
			in.RecordDocs, in.RecordContext = h.houseRulesContext(c, cc, in.Privacy)
		} else if in.ContentMode == prompt.ContentCalendar {
			in.RecordDocs, in.RecordContext = h.calendarContext(c, cc, in.Privacy, reg)
		} else if in.ContentMode == "all" || hasCategory(in.ContentMode, recordsCategory) {
			in.RecordContext = reg.ExportAll(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, in.Privacy))
		}
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

// PromptInstructions returns the standing half of the prompt as plain text,
// for the "Copy AI instructions" button: every block format, the lookups and
// the rules, plus the campaign's page types, but none of its content, so an
// operator can save it in their AI once and paste only short requests.
//
// GET /campaigns/:id/ai-workspace/prompt/instructions (owner only)
func (h *Handler) PromptInstructions(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if h.promptBuilder == nil {
		return apperror.NewBadRequest("AI prompt builder is not configured on this server")
	}
	out, err := h.promptBuilder.Build(c.Request().Context(), cc.Campaign.Name, auth.GetUserID(c), cc.Campaign.ID, h.instructionsInput())
	if err != nil {
		slog.Error("ai-workspace: prompt instructions failed",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return apperror.NewInternal(err)
	}
	if h.audit != nil {
		h.audit.LogCampaignEvent(c.Request().Context(), cc.Campaign.ID, "campaign.ai_prompt.instructions_copied",
			map[string]any{"prompt_byte_count": len(out)})
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.String(http.StatusOK, out)
}

// instructionsInput is the standing half of the prompt: every block format,
// the lookups and the rules, plus the campaign's page types.
func (h *Handler) instructionsInput() prompt.Input {
	in := prompt.Input{
		IncludeEntityTypes:        true,
		IncludeFrontMatterExample: true,
		ContentMode:               "none",
		Instructions:              prompt.InstructionsOnly,
	}
	if h.lookups != nil {
		in.Lookups = records.LookupDoc()
	}
	if h.records != nil {
		in.Capabilities = h.records.Capabilities()
		in.RecordDocs = h.records.Docs()
	}
	return in
}

// ConnectorInstructions answers a connected AI's how_to_write_changes: the
// same standing instructions "Copy AI instructions" gives, written for the
// connection's owner.
func (h *Handler) ConnectorInstructions(ctx context.Context, campaignID, campaignName, userID string) (string, error) {
	if h.promptBuilder == nil {
		return "", errors.New("ai-workspace: prompt builder not configured")
	}
	return h.promptBuilder.Build(ctx, campaignName, userID, campaignID, h.instructionsInput())
}

// ConnectorCheck is what a connected AI is told about the changes it asks
// for before they wait for the owner: for now, new pages filed in a folder
// their category doesn't have. The AI can fix its own request, where the
// owner would have to work out the right category at review. It looks up
// categories and folders only, never pages.
func (h *Handler) ConnectorCheck(ctx context.Context, campaignID, changes string) ([]string, error) {
	if h.importLookup == nil || h.importFolders == nil {
		return nil, nil
	}
	pages, recs := splitImport(importer.Parse(changes))
	slips, err := importer.NewClassifier(h.importLookup, campaignID).
		WithFolders(h.importFolders, pendingFolderNames(recs)).
		NewPageFolderSlips(ctx, pages)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(slips))
	for _, s := range slips {
		out = append(out, folderSlipLine(s))
	}
	return out, nil
}

// folderSlipLine tells the AI what is wrong with a page's folder and how to
// fix it, in the request's own terms.
func folderSlipLine(s importer.FolderSlip) string {
	head := fmt.Sprintf("block %d (%s): ", s.Block, s.Page)
	folder := strconv.Quote(s.Folder)
	var b strings.Builder
	b.WriteString(head)
	if s.NewCategory {
		fmt.Fprintf(&b, "the category %s is new, so it has no folder %s", strconv.Quote(s.Category), folder)
	} else {
		fmt.Fprintf(&b, "the %s category has no folder %s", s.Category, folder)
	}
	switch {
	case len(s.Elsewhere) == 1:
		fmt.Fprintf(&b, "; %s has one. Use type: %s, or leave the folder out", s.Elsewhere[0], s.Elsewhere[0])
	case len(s.Elsewhere) > 1:
		fmt.Fprintf(&b, "; %s each have one. Use the type of the one you mean, or leave the folder out", strings.Join(s.Elsewhere, ", "))
	case s.NewCategory:
		b.WriteString(". Use an existing category, or leave the folder out")
	default:
		b.WriteString(". Make it first with a folder block in this request, or leave the folder out")
	}
	return b.String()
}

// calendarContext is the Calendar prompt's format and context: the docs of
// the calendar kinds (always, since the AI must write those formats) and the
// calendar and weather-kinds lookups in the chosen privacy, which say how to
// set up a main calendar when there is none.
func (h *Handler) calendarContext(c echo.Context, cc *campaigns.CampaignContext, privacy aiexport.PrivacyMode, reg *records.Registry) (doc, answers string) {
	doc = reg.DocsFor(records.CalendarKinds...)
	if h.lookups == nil {
		return doc, ""
	}
	ans := h.lookups.Answer(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, privacy), records.CalendarLookups())
	return doc, records.Markdown(ans)
}

// houseRulesContext is the house-rules prompt's format and context: the
// house-rule kind's doc (always, since the AI must write that format) and
// today's house rules in the chosen privacy.
func (h *Handler) houseRulesContext(c echo.Context, cc *campaigns.CampaignContext, privacy aiexport.PrivacyMode) (doc, rules string) {
	k, ok := h.records.Get("house-rule")
	if !ok {
		return "", ""
	}
	doc = "### kind: " + k.Name() + " (" + k.Label() + ")\n\n" + strings.TrimSpace(k.Doc())
	out, err := k.Export(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, privacy))
	if err != nil {
		return doc, "<!-- " + k.Label() + "s: could not be listed -->"
	}
	if strings.TrimSpace(out) == "" {
		return doc, ""
	}
	return doc, "## " + k.Label() + "s\n\n" + out
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
	// A form sends one categories value per ticked box (a link may send
	// one comma-separated value); read them all, not just the first.
	rawCats := strings.Join(c.QueryParams()["categories"], ",")
	if raw := rawCats; raw != "" {
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
	if h.records != nil && (rawCats == "" || hasCategory(rawCats, recordsCategory)) {
		if more := h.records.ExportAll(c.Request().Context(), cc.Campaign.ID, exportActor(c, cc, opts.Privacy)); more != "" {
			markdown += "\n\n" + more
		}
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

// recordsCategory is the export and prompt checkbox for everything the
// record kinds list (weather, tables, pins, house rules…).
const recordsCategory = "more"

// hasCategory reports whether a comma-separated category list names cat.
func hasCategory(list, cat string) bool {
	for _, s := range strings.Split(list, ",") {
		if strings.TrimSpace(s) == cat {
			return true
		}
	}
	return false
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

// foundryHas is the review's check for a new creature page Foundry already
// has: where Foundry keeps a monster of that name, or "". Nil when there is
// no list to check, or the operator may not see it.
func (h *Handler) foundryHas(ctx context.Context, campaignID string, a records.Actor) func(string) string {
	if h.lookups == nil || h.lookups.Monsters == nil || !a.CanAuthorDmOnly() {
		return nil
	}
	rep, err := h.lookups.Monsters.Monsters(ctx, campaignID, a)
	if err != nil || rep.ReportedAt == nil {
		return nil
	}
	return func(name string) string {
		if m := rep.Find(name); m != nil && !m.Linked {
			return m.Where()
		}
		return ""
	}
}
