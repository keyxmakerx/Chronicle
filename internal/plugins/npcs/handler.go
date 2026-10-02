// handler.go provides HTTP endpoints for the NPC gallery. Thin handlers
// that bind request parameters, call the NPC service, and render templ
// components or JSON responses.
package npcs

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"
	"github.com/labstack/echo/v4"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// VisibilityToggler toggles an entity's is_private flag and returns the new
// state, scoped to the caller's campaign. Injected from the entities plugin to
// avoid circular imports. campaignID is the authorization boundary: the entity
// must belong to it or the toggle fails with NotFound (SEC-IDOR-1).
type VisibilityToggler interface {
	TogglePrivate(ctx context.Context, entityID, campaignID string) (newPrivate bool, err error)
}

// Handler serves NPC gallery endpoints.
type Handler struct {
	svc        NPCService
	visToggler VisibilityToggler
}

// NewHandler creates a new NPC gallery handler.
func NewHandler(svc NPCService) *Handler {
	return &Handler{svc: svc}
}

// SetVisibilityToggler injects the entity visibility toggler.
func (h *Handler) SetVisibilityToggler(vt VisibilityToggler) {
	h.visToggler = vt
}

// Index used to render the standalone NPC gallery; that page folded into the
// unified entities Characters page (NPCs/Monsters section). The route now
// redirects so existing links/bookmarks keep working.
func (h *Handler) Index(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/characters", cc.Campaign.ID))
}

// NPCSection renders the NPCs/Monsters section embedded in the Characters page:
// a featured portrait row (entities bearing featureTag) above the full
// role-aware list. It satisfies entities.NPCSectionProvider (injected there), so
// the core entities plugin never imports this addon. Role matches CountAPI's
// cc.VisibilityRole(), which promotes a DM-granted co-DM to Owner, so a co-DM
// sees dm_only/custom-restricted NPCs here too, same as an Owner.
func (h *Handler) NPCSection(ctx context.Context, cc *campaigns.CampaignContext, userID, csrfToken, featureTag string) templ.Component {
	cid := cc.Campaign.ID
	role := cc.VisibilityRole()

	var featured []NPCCard
	if featureTag != "" {
		var err error
		if featured, _, err = h.svc.ListNPCs(ctx, cid, role, userID, NPCListOptions{Page: 1, PerPage: 24, Sort: "name", Tag: featureTag, IncludeDmOnlyTags: canSeeDmOnlyTags(cc)}); err != nil {
			slog.Warn("npc section: featured list failed", slog.String("campaign_id", cid), slog.Any("error", err))
		}
	}
	list, err := h.buildListView(ctx, cc, userID, "", "", 1)
	if err != nil {
		slog.Warn("npc section: list failed", slog.String("campaign_id", cid), slog.Any("error", err))
	}

	return NPCSectionComponent(cc, featured, list, csrfToken)
}

// canSeeDmOnlyTags mirrors the tags widget's rule for who may see GM-only tags,
// so the NPC filter and card chips never show a tag the tag manager would hide.
func canSeeDmOnlyTags(cc *campaigns.CampaignContext) bool {
	return cc.MemberRole >= campaigns.RoleOwner || cc.IsSiteAdmin || cc.IsDmGranted
}

// buildListView loads one page of the NPC list plus the filter options. A
// failed tag lookup only drops the filter; the list still renders.
func (h *Handler) buildListView(ctx context.Context, cc *campaigns.CampaignContext, userID, search, tag string, page int) (NPCListView, error) {
	cid := cc.Campaign.ID
	dmTags := canSeeDmOnlyTags(cc)

	tags, err := h.svc.ListTags(ctx, cid, cc.VisibilityRole(), userID, dmTags)
	if err != nil {
		slog.Warn("npc list: tag options failed", slog.String("campaign_id", cid), slog.Any("error", err))
		tags = nil
	}
	// The slug is only honoured when it is one of the offered options.
	search, tag, page = ParseNPCSectionQuery(search, tag, strconv.Itoa(page), tags)

	// Each request returns exactly page N; the client appends it to earlier pages.
	cards, total, err := h.svc.ListNPCs(ctx, cid, cc.VisibilityRole(), userID, NPCListOptions{
		Page: page, PerPage: NPCPageSize, Sort: "name", Search: search, Tag: tag, IncludeDmOnlyTags: dmTags,
	})
	if err != nil {
		return NPCListView{Search: search, Tag: tag, Tags: tags}, err
	}
	return NPCListView{Cards: cards, Pager: NewNPCPager(page, NPCPageSize, total), Search: search, Tag: tag, Tags: tags}, nil
}

// SectionFragment handles GET /campaigns/:id/npcs/section?q=&tag=&page=, the
// HTMX endpoint behind the search box, tag filter and "Show more". Page 1
// returns the whole list part; later pages return just the next cards plus an
// out-of-band footer so the client appends them. Direct (non-HTMX) hits go back
// to the Characters page.
func (h *Handler) SectionFragment(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if !middleware.IsHTMX(c) {
		return c.Redirect(http.StatusFound, fmt.Sprintf("/campaigns/%s/characters", cc.Campaign.ID))
	}

	page, _ := strconv.Atoi(c.QueryParam("page"))
	view, err := h.buildListView(c.Request().Context(), cc, auth.GetUserID(c), c.QueryParam("q"), c.QueryParam("tag"), page)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if page > 1 {
		return middleware.Render(c, http.StatusOK, NPCMoreFragment(cc, view, middleware.GetCSRFToken(c)))
	}
	return middleware.Render(c, http.StatusOK, NPCResults(cc, view, middleware.GetCSRFToken(c)))
}

// ToggleReveal handles POST /campaigns/:id/npcs/:eid/reveal.
// Toggles an NPC's is_private flag. Only Scribe+ can reveal/hide NPCs.
func (h *Handler) ToggleReveal(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	entityID := c.Param("eid")
	if entityID == "" {
		return apperror.NewBadRequest("entity ID is required")
	}

	if h.visToggler == nil {
		return apperror.NewInternal(nil)
	}

	// Scope the toggle to this campaign so a Scribe in one campaign can't flip
	// visibility on another campaign's entity by UUID (SEC-IDOR-1). A campaign
	// mismatch surfaces as NotFound from the toggler; return it verbatim (a 404)
	// rather than masking it as a 500.
	newPrivate, err := h.visToggler.TogglePrivate(c.Request().Context(), entityID, cc.Campaign.ID)
	if err != nil {
		return err
	}

	if middleware.IsHTMX(c) {
		return middleware.Render(c, http.StatusOK, RevealBadge(entityID, cc.Campaign.ID, newPrivate, middleware.GetCSRFToken(c)))
	}
	return c.JSON(http.StatusOK, map[string]any{
		"entity_id":  entityID,
		"is_private": newPrivate,
	})
}

// CountAPI returns the NPC count as JSON at GET /campaigns/:id/npcs/count.
// Used by the sidebar badge and layout blocks.
func (h *Handler) CountAPI(c echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}

	userID := auth.GetUserID(c)
	// See the matching comment on NPCSection: co-DMs are promoted for
	// visibility so this badge count agrees with what NPCSection shows them.
	count, err := h.svc.CountNPCs(c.Request().Context(), cc.Campaign.ID, cc.VisibilityRole(), userID)
	if err != nil {
		return apperror.NewInternal(err)
	}

	return c.JSON(http.StatusOK, map[string]int{"count": count})
}

// GalleryBlock fetches NPC cards for embedding in entity page layout blocks.
// Returns a compact card list limited by the block config.
func (h *Handler) GalleryBlock(ctx context.Context, campaignID string, role int, userID string, limit int) ([]NPCCard, error) {
	opts := DefaultNPCListOptions()
	opts.PerPage = limit

	cards, _, err := h.svc.ListNPCs(ctx, campaignID, role, userID, opts)
	return cards, err
}
