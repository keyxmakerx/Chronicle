// category_home_editor.go gives the Customizer's page editor a category's
// home (the category's landing page): where it starts from, which widgets it
// offers, and each widget drawn as the landing page draws it.

package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// categoryHomeTypes are the widgets CategoryBlockSwitch draws itself; the
// palette offers these and the types other plugins registered, nothing else.
var categoryHomeTypes = []string{
	campaigns.BlockCategoryHeader, campaigns.BlockSearchBar, "pinned_pages", campaigns.BlockEntityGrid,
	"recent_pages", "text_block", "calendar_preview", "timeline_preview",
}

// categoryHomePreviewPages is how many of the category's pages the preview
// draws its lists with, the same as the landing page's first page.
const categoryHomePreviewPages = 24

// defaultCategoryHomeLayout is where the editor starts while the category
// has no home of its own: the heading, search, pinned pages and every page.
func defaultCategoryHomeLayout() *campaigns.DashboardLayout {
	row := func(id, blockType string) campaigns.DashboardRow {
		return campaigns.DashboardRow{ID: "row-" + id, Columns: []campaigns.DashboardColumn{{
			ID: "col-" + id, Width: 12, Blocks: []campaigns.DashboardBlock{{ID: "blk-" + id, Type: blockType}},
		}}}
	}
	return &campaigns.DashboardLayout{Rows: []campaigns.DashboardRow{
		row("header", campaigns.BlockCategoryHeader),
		row("search", campaigns.BlockSearchBar),
		row("pinned", "pinned_pages"),
		row("pages", campaigns.BlockEntityGrid),
	}}
}

// categoryHomeEditorState is what the page editor draws a category's home
// from. Unsaved means the landing page still shows its built-in view, and
// Save stores the arrangement as the category's own.
func categoryHomeEditorState(cc *campaigns.CampaignContext, et *EntityType) string {
	base := fmt.Sprintf("/campaigns/%s", cc.Campaign.ID)
	layout := et.ParseCategoryDashboardLayout()
	unsaved := layout == nil
	if unsaved {
		layout = defaultCategoryHomeLayout()
	}
	desc := ""
	if et.Description != nil {
		desc = *et.Description
	}
	b, err := json.Marshal(map[string]any{
		"scope":       "cathome",
		"unsaved":     unsaved,
		"campaignId":  cc.Campaign.ID,
		"key":         strconv.Itoa(et.ID),
		"name":        et.Name,
		"plural":      et.NamePlural,
		"layout":      layout,
		"description": desc,
		"only":        append(append([]string(nil), categoryHomeTypes...), extraCategoryBlockTypes()...),
		"blockTypes":  base + "/entity-types/block-types?context=category",
		"preview":     fmt.Sprintf("%s/entity-types/%d/home-preview", base, et.ID),
	})
	if err != nil {
		return "{}"
	}
	return string(b)
}

// CategoryHomePreview draws the blocks the Customizer sends as the
// category's landing page draws them, with the description the draft holds:
// POST /campaigns/:id/entity-types/:etid/home-preview with
// {"description": "...", "blocks": [{id, type, config}]}. It answers
// {"blocks": {id: html}}; a block that draws nothing is left out.
func (h *Handler) CategoryHomePreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}
	ctx := c.Request().Context()
	et, err := h.service.GetEntityTypeByID(ctx, etID)
	if err != nil {
		return err
	}
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body struct {
		Description *string                    `json:"description"`
		Blocks      []campaigns.DashboardBlock `json:"blocks"`
	}
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, layoutPreviewMaxBody)).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if len(body.Blocks) > layoutPreviewMaxBlocks {
		return apperror.NewBadRequest("too many blocks to preview at once")
	}
	shown := *et
	if body.Description != nil {
		d := *body.Description
		if len([]rune(d)) > maxCategoryDescription {
			return apperror.NewBadRequest("a category description can be 2000 characters at most")
		}
		shown.Description = &d
	}

	list, total, err := h.service.List(ctx, cc.Campaign.ID, etID, int(cc.VisibilityRole()), auth.GetUserID(c),
		ListOptions{Page: 1, PerPage: categoryHomePreviewPages, Sort: "name"})
	if err != nil {
		return err
	}
	// Blocks read the sidebar's data from the render context, which
	// middleware.Render normally fills in.
	rctx := ctx
	if middleware.LayoutInjector != nil {
		rctx = middleware.LayoutInjector(c, ctx)
	}
	allowed := map[string]bool{}
	for _, t := range categoryHomeTypes {
		allowed[t] = true
	}
	out := map[string]string{}
	for _, b := range body.Blocks {
		if b.ID == "" || out[b.ID] != "" || (!allowed[b.Type] && !IsExtraCategoryBlock(b.Type)) {
			continue
		}
		if html := renderCategoryHomeBlock(rctx, cc, &shown, b, list, total); strings.TrimSpace(html) != "" {
			out[b.ID] = html
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"blocks": out})
}

// renderCategoryHomeBlock draws one block as the landing page does. A
// renderer that fails or panics yields nothing, so one broken block cannot
// blank the editor.
func renderCategoryHomeBlock(ctx context.Context, cc *campaigns.CampaignContext, et *EntityType, b campaigns.DashboardBlock, list []Entity, total int) (html string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("category home preview: block renderer panicked", slog.String("block_type", b.Type), slog.Any("panic", r))
			html = ""
		}
	}()
	if b.Config == nil {
		b.Config = map[string]any{}
	}
	var sb strings.Builder
	if err := CategoryBlockSwitch(cc, et, b, list, total).Render(ctx, &sb); err != nil {
		slog.Warn("category home preview: block render failed", slog.String("block_type", b.Type), slog.Any("error", err))
		return ""
	}
	return sb.String()
}
