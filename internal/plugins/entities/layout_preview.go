// layout_preview.go serves Customize's page editor the blocks of a category's
// page layout drawn for one of that category's real pages, so the owner
// arranges the page they will actually get rather than grey boxes.

package entities

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

const (
	// layoutPreviewMaxBlocks bounds one request; a page layout holds at most
	// 20 rows of 4 columns of 10 blocks, or tile sections, well under this.
	layoutPreviewMaxBlocks = 200
	// layoutPreviewMaxBody bounds the request body, a little over the
	// largest layout ValidateLayout accepts.
	layoutPreviewMaxBody = 128 << 10
	// layoutPreviewPages is how many of the category's pages the editor
	// offers to show the layout with.
	layoutPreviewPages = 25
)

// layoutPreviewPage is one page the editor can show the layout with.
type layoutPreviewPage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// layoutPreviewResponse carries each block's HTML by block id. A block that
// draws nothing for this page, or whose type is unknown, is left out and
// the editor shows its name in its place.
type layoutPreviewResponse struct {
	EntityID string              `json:"entityId"`
	Pages    []layoutPreviewPage `json:"pages"`
	Blocks   map[string]string   `json:"blocks"`
}

// LayoutPreview draws blocks of a category's page layout as one of the
// category's pages shows them to the owner:
// POST /campaigns/:id/entity-types/:etid/layout-preview with
// {"entityId": "...", "blocks": [{id, type, config}]}. With no entityId it
// uses the most recently changed page. Nothing is saved.
func (h *Handler) LayoutPreview(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	etID, err := strconv.Atoi(c.Param("etid"))
	if err != nil {
		return apperror.NewBadRequest("invalid entity type ID")
	}
	et, err := h.service.GetEntityTypeByID(c.Request().Context(), etID)
	if err != nil {
		return err
	}
	if et.CampaignID != cc.Campaign.ID {
		return apperror.NewNotFound("entity type not found")
	}

	var body struct {
		EntityID string          `json:"entityId"`
		Blocks   []TemplateBlock `json:"blocks"`
	}
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, layoutPreviewMaxBody)).Decode(&body); err != nil {
		return apperror.NewBadRequest("invalid request body")
	}
	if len(body.Blocks) > layoutPreviewMaxBlocks {
		return apperror.NewBadRequest("too many blocks to preview at once")
	}

	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	list, _, err := h.service.List(ctx, cc.Campaign.ID, etID, int(cc.VisibilityRole()), userID,
		ListOptions{Page: 1, PerPage: layoutPreviewPages, Sort: "updated"})
	if err != nil {
		return err
	}
	out := layoutPreviewResponse{Pages: make([]layoutPreviewPage, 0, len(list)), Blocks: map[string]string{}}
	for _, e := range list {
		out.Pages = append(out.Pages, layoutPreviewPage{ID: e.ID, Name: e.Name})
	}

	pageID := body.EntityID
	if pageID == "" {
		if len(list) == 0 {
			return c.JSON(http.StatusOK, out)
		}
		pageID = list[0].ID
	} else {
		// The page must be one of this campaign's pages in this category, or
		// the editor could be pointed at any page through an unrelated layout.
		e, err := h.service.GetByID(ctx, pageID)
		if err != nil || e.CampaignID != cc.Campaign.ID || e.EntityTypeID != etID {
			return apperror.NewNotFound("entity not found")
		}
	}
	p, err := h.prepareShowFor(c, pageID)
	if err != nil {
		return err
	}
	out.EntityID = p.entity.ID

	rctx := c.Request().Context()
	for _, b := range body.Blocks {
		if b.ID == "" || out.Blocks[b.ID] != "" || h.blockRegistry == nil || !h.blockRegistry.IsValid(b.Type) {
			continue
		}
		if b.Type == "attributes" && !p.showAttributes {
			continue
		}
		if html := renderPreviewBlock(rctx, b, p); strings.TrimSpace(html) != "" {
			out.Blocks[b.ID] = html
		}
	}
	return c.JSON(http.StatusOK, out)
}

// renderPreviewBlock draws one block the way the page does. Each block gets
// its own singleton tracker: the editor shows every block it is sent, and
// ValidateLayout, not the preview, refuses a second singleton. A renderer
// that fails or panics yields nothing, so one broken block cannot blank the
// editor.
func renderPreviewBlock(ctx context.Context, b TemplateBlock, p *showPage) (html string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("layout preview: block renderer panicked",
				slog.String("block_type", b.Type), slog.Any("panic", r))
			html = ""
		}
	}()
	if b.Config == nil {
		b.Config = map[string]any{}
	}
	ctx = WithSingletonTracker(ctx)
	var sb strings.Builder
	if err := RenderBlock(ctx, b, p.cc, p.entity, p.entityType, p.csrfToken).Render(ctx, &sb); err != nil {
		slog.Warn("layout preview: block render failed",
			slog.String("block_type", b.Type), slog.Any("error", err))
		return ""
	}
	return sb.String()
}
