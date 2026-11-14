package entities

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/a-h/templ"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/templates/layouts"
)

// placeableBlockRenderer draws a block another plugin owns on an ordinary
// page: the same lazy-loading placeholder the campaign home uses, carrying the
// page's id so the fragment can verify the block is really placed there. The
// fragment decides what the viewer sees; nothing is read here.
func placeableBlockRenderer(blockType string) BlockRenderer {
	return func(ctx BlockRenderContext) templ.Component {
		if ctx.CC == nil || ctx.CC.Campaign == nil {
			return templ.NopComponent
		}
		on := ""
		if ctx.Entity != nil {
			on = ctx.Entity.ID
		}
		return campaigns.PageBlockFragment(blockType, ctx.CC.Campaign.ID, on)
	}
}

// registerPlaceableCategoryBlocks lets the same blocks sit on a category's
// home. That home has no page id to name, so a visitor sees them there only
// where the Guests home places the block.
func registerPlaceableCategoryBlocks() {
	for _, t := range campaigns.PlaceableBlockTypes() {
		blockType := t
		RegisterCategoryBlock(blockType, func(cc *campaigns.CampaignContext, _ *EntityType) templ.Component {
			if cc == nil || cc.Campaign == nil {
				return templ.NopComponent
			}
			return campaigns.PageBlockFragment(blockType, cc.Campaign.ID, "")
		})
	}
}

// layoutDrawsBlockForGuests reports whether layout draws blockType in a column
// or tile for a viewer who is not on the Director team. It mirrors what the
// page draws, and is deliberately stricter than "mentioned in the layout":
// a block marked for the Director team is not drawn for anyone else, and
// container blocks are not drawn by the server at all, so neither counts.
func layoutDrawsBlockForGuests(layout EntityTypeLayout, blockType string) bool {
	for _, row := range layout.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				if b.Type == blockType && blockVisibility(b) == "everyone" {
					return true
				}
			}
		}
		for _, t := range row.Tiles {
			if t.Type == blockType && blockVisibility(TemplateBlock{Config: t.Config}) == "everyone" {
				return true
			}
		}
	}
	return false
}

// BlockPlacementChecker implements campaigns.BlockPlacementChecker: it is how
// a fragment that cannot import this plugin learns whether the owner placed
// its block on a page the guest may open.
type BlockPlacementChecker struct {
	svc     EntityService
	layouts PageLayoutService // nil while named layouts are unavailable
}

// NewBlockPlacementChecker builds the checker. layouts may be nil.
func NewBlockPlacementChecker(svc EntityService, layouts PageLayoutService) *BlockPlacementChecker {
	return &BlockPlacementChecker{svc: svc, layouts: layouts}
}

var _ campaigns.BlockPlacementChecker = (*BlockPlacementChecker)(nil)

// GuestMaySee is true only when every step holds: the page exists in this
// campaign, the viewer may open it (the check a page view makes, at the same
// role), and the layout the page actually wears draws blockType for them.
// A page that is missing, in another campaign or hidden answers the same
// false, so the route cannot be used to probe for pages. Every error is false.
func (p *BlockPlacementChecker) GuestMaySee(ctx context.Context, campaignID, blockType, onEntityID, userID string, visibilityRole int) bool {
	if p == nil || p.svc == nil || onEntityID == "" || campaignID == "" {
		return false
	}
	entity, err := p.svc.GetByID(ctx, onEntityID)
	if err != nil || entity == nil || entity.CampaignID != campaignID {
		return false
	}
	access, err := p.svc.CheckEntityAccess(ctx, entity.ID, visibilityRole, userID)
	if err != nil || access == nil || !access.CanView {
		return false
	}
	et, err := p.svc.GetEntityTypeByID(ctx, entity.EntityTypeID)
	if err != nil || et == nil {
		return false
	}
	// Same choice wornLayout makes for a page view: the category's main layout
	// unless the page wears one of its named layouts.
	layout := et.Layout
	if p.layouts != nil {
		if pl, key := p.layouts.Worn(ctx, entity, et); !pl.Empty() && key != MainLayoutKey {
			layout = pl.LayoutFor(et, key)
		}
	}
	return layoutDrawsBlockForGuests(layout, blockType)
}

// pageHomeBlockTypes are the home blocks the campaigns plugin draws inline,
// or through its own fragment routes, that also sit on an ordinary page and a
// category's home. Each asks for nothing but the viewer's campaign context
// and its own saved settings, and decides for itself what that viewer may see,
// so a block is as safe on a page as on the home it was made for.
//
// Left out on purpose: text_block (already a page block), session_tracker
// (retired, the home still draws it as Coming up) and relations_graph (a
// legacy type no palette offers; Full Relations Graph replaced it).
var pageHomeBlockTypes = []string{
	"welcome_banner", "quick_actions", "category_grid", "recent_pages", "entity_list", "pinned_pages",
	"calendar_preview", "timeline_preview", "calendar_full", "timeline_full", "relations_graph_full",
	"map_full", "coming_up", "start_here", "at_a_glance", "party", "your_hero", "getting_started",
	"when_we_play", "where_we_play", "house_rules", "last_time", "activity_feed", "sync_status",
}

// PageHomeBlockTypes lists the older home blocks that may sit on any page.
func PageHomeBlockTypes() []string {
	return append([]string(nil), pageHomeBlockTypes...)
}

// pageBlockFrameClass keeps a home block inside the narrow column a page may
// put it in: the home's responsive grids count the window, not the column, so
// their cards would otherwise squeeze or overflow.
const pageBlockFrameClass = "min-w-0 [&_.cb-rows]:!grid-cols-[repeat(auto-fill,minmax(9rem,1fr))] [&_.cb-shortcuts]:!grid-cols-[repeat(auto-fill,minmax(9rem,1fr))]"

// drawHomeBlock draws one home block through the campaigns home's own
// switch, so its content and its own visibility rules are exactly the
// home's, whichever page asks. config is the block's saved settings from the
// layout it sits in, and the block type is the registered one, never anything
// taken from a request.
func drawHomeBlock(cc *campaigns.CampaignContext, id, blockType string, config map[string]any, recent func(context.Context) []campaigns.RecentEntity) templ.Component {
	if cc == nil || cc.Campaign == nil {
		return templ.NopComponent
	}
	block := campaigns.DashboardBlock{ID: id, Type: blockType, Config: config}
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		var pages []campaigns.RecentEntity
		if recent != nil {
			pages = recent(ctx)
		}
		// Drawn aside first: a block that shows this viewer nothing leaves no
		// frame behind in the column.
		var sb strings.Builder
		if err := campaigns.DashboardBlockSwitch(cc, block, pages).Render(ctx, &sb); err != nil {
			return err
		}
		if strings.TrimSpace(sb.String()) == "" {
			return nil
		}
		_, err := io.WriteString(w, `<div class="`+pageBlockFrameClass+`">`+sb.String()+`</div>`)
		return err
	})
}

// homeBlockRenderer is the page renderer for a home block. Recent Pages is
// the one block that needs pages the home handler preloads; here they are
// read the same way, for this viewer, when the block draws.
func (r *BlockRegistry) homeBlockRenderer(blockType string) BlockRenderer {
	return func(rc BlockRenderContext) templ.Component {
		var recent func(context.Context) []campaigns.RecentEntity
		if blockType == campaigns.BlockRecentPages {
			recent = func(ctx context.Context) []campaigns.RecentEntity {
				return r.recentPagesFor(ctx, rc.CC, rc.UserID, rc.Block.Config)
			}
		}
		return drawHomeBlock(rc.CC, rc.Block.ID, blockType, rc.Block.Config, recent)
	}
}

// recentPagesFor lists the recently updated pages this viewer may open. An
// owner looking as a player reads as a player. A failed or missing read draws
// the block empty, never an error on the page.
func (r *BlockRegistry) recentPagesFor(ctx context.Context, cc *campaigns.CampaignContext, userID string, config map[string]any) []campaigns.RecentEntity {
	r.mu.RLock()
	lister := r.recentLister
	r.mu.RUnlock()
	if lister == nil || cc == nil || cc.Campaign == nil {
		return nil
	}
	role := cc.VisibilityRole()
	if layouts.IsViewingAsPlayer(ctx) {
		role = int(campaigns.RolePlayer)
	}
	limit := BlockConfigLimit(config, "limit", 8)
	if limit > 12 {
		limit = 12
	}
	pages, err := lister.ListRecentForDashboard(ctx, cc.Campaign.ID, role, userID, limit)
	if err != nil {
		slog.Warn("recent pages block: read failed", slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return nil
	}
	return pages
}

// offerOnPages makes an already registered home block available on ordinary
// pages: it adds the page context and the home-block renderer and touches
// nothing else, so the label, icon, addon gate, settings and tile sizes stay
// what the block registered. It reports false for an unregistered type.
func (r *BlockRegistry) offerOnPages(blockType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[blockType]
	if !ok {
		return false
	}
	has := false
	for _, c := range e.meta.Contexts {
		if c == "template" {
			has = true
		}
	}
	if !has {
		e.meta.Contexts = append(append([]string(nil), e.meta.Contexts...), "template")
	}
	e.renderer = r.homeBlockRenderer(blockType)
	r.entries[blockType] = e
	return true
}

// registerHomeCategoryBlocks lets the same blocks sit on a category's home.
// The ones CategoryBlockSwitch already draws itself in a category's own way
// (the pinned, recent, calendar and timeline blocks) keep that version. A
// block's settings come from the category's saved layout.
func registerHomeCategoryBlocks() {
	own := map[string]bool{}
	for _, t := range categoryHomeTypes {
		own[t] = true
	}
	for _, t := range pageHomeBlockTypes {
		if own[t] {
			continue
		}
		blockType := t
		RegisterCategoryBlockWithConfig(blockType, func(cc *campaigns.CampaignContext, _ *EntityType, config map[string]any) templ.Component {
			return drawHomeBlock(cc, "", blockType, config, nil)
		})
	}
}
