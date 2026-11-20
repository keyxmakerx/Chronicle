package entities

// The campaign home's "Start here" block: one page the owner picks, shown with
// its name and first line. The home lives in the campaigns plugin, which may
// not read pages, so its placeholder loads this fragment.

import (
	"html"
	"net/http"
	"regexp"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// startHereLineLen caps the first line, so a page that opens with a long
// paragraph still reads as a lead, not as the page itself.
const startHereLineLen = 180

// startHereView is what the block draws.
type startHereView struct {
	CampaignID string
	// Empty means no page to show: the owner gets a slot that says how to
	// pick one, and Gone says the picked page no longer exists.
	Empty bool
	Gone  bool
	Name  string
	Line  string
	Href  string
	// DMOnly marks a page players can't see, for the owner's amber edge.
	DMOnly bool
}

// blockBreak splits served HTML at the end of each block, so the first line
// is the first paragraph rather than the whole entry run together.
var blockBreak = regexp.MustCompile(`(?i)</(?:p|li|div|blockquote|pre|h[1-6])>|<br\s*/?>`)

// headingOpen marks a chunk that is a heading: a page's first line is its
// first paragraph, never a section title like "Overview".
var headingOpen = regexp.MustCompile(`(?i)^\s*<h[1-6][\s>]`)

// startHereFirstLine is the first paragraph of an entry's served HTML as plain
// text, cut at a word near startHereLineLen. Headings are skipped. The caller
// serves the HTML first, so secrets and hidden page names are already gone.
func startHereFirstLine(entryHTML string) string {
	for _, chunk := range blockBreak.Split(entryHTML, -1) {
		if headingOpen.MatchString(chunk) {
			continue
		}
		plain := html.UnescapeString(htmlTagRegex.ReplaceAllString(chunk, " "))
		plain = strings.Join(strings.Fields(plain), " ")
		if plain == "" {
			continue
		}
		r := []rune(plain)
		if len(r) <= startHereLineLen {
			return plain
		}
		cut := string(r[:startHereLineLen])
		if i := strings.LastIndex(cut, " "); i > startHereLineLen/2 {
			cut = cut[:i]
		}
		return strings.TrimRight(cut, " ,;:") + "…"
	}
	return ""
}

// homePage is a page a home block shows, as one viewer may read it.
type homePage struct {
	Entity *Entity
	// HTML is the entry as this viewer is served it, secrets and hidden page
	// names already taken out when they must not read them.
	HTML     string
	Director bool
	AsPlayer bool
	// Gone says the picked page no longer exists here. Entity nil without
	// Gone means nothing was picked or the viewer can't open it.
	Gone bool
}

// loadHomePage resolves the page a home block was pointed at for the
// viewer. Blocks that show one picked page share it, so they cannot drift on
// who may open what.
func (h *Handler) loadHomePage(c *echo.Context, cc *campaigns.CampaignContext, pageID string) homePage {
	ctx := c.Request().Context()
	userID := auth.GetUserID(c)
	asPlayer := campaigns.ViewingAsPlayer(c, cc)
	hp := homePage{
		Director: cc.CanAuthorDmOnly() && !asPlayer,
		AsPlayer: asPlayer,
	}
	role := int(cc.VisibilityRole())
	if asPlayer {
		role = int(campaigns.RolePlayer)
	}
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return hp
	}
	entity, err := h.service.GetByID(ctx, pageID)
	if err != nil || entity == nil || entity.CampaignID != cc.Campaign.ID {
		hp.Gone = true
		return hp
	}
	access, err := h.service.CheckEntityAccess(ctx, entity.ID, role, userID)
	if err != nil || !access.CanView {
		return hp
	}
	if et, err := h.service.GetEntityTypeByID(ctx, entity.EntityTypeID); err == nil {
		entity = h.currentEraEntity(ctx, cc, entity, et, userID)
	}
	hp.Entity = entity
	if entity.EntryHTML != nil {
		_, served, _ := h.servedText(c, cc, nil, entity.EntryHTML)
		hp.HTML = deref(served)
		// servedText keeps secrets for the owner; viewing as a player they
		// must read what a player reads.
		if asPlayer {
			hp.HTML = sanitize.StripSecretsHTML(hp.HTML)
		}
	}
	return hp
}

// StartHere draws the block for the page in ?page=. A page the viewer can't
// open, or no page at all, is an empty answer that removes the placeholder,
// except for the owner and co-DMs, who get the slot that says how to pick one.
// GET /campaigns/:id/start-here
func (h *Handler) StartHere(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	hp := h.loadHomePage(c, cc, c.QueryParam("page"))
	view := startHereView{CampaignID: cc.Campaign.ID, Empty: true, Gone: hp.Gone}
	if hp.Entity == nil {
		if !hp.Director {
			return c.HTML(http.StatusOK, "")
		}
		return middleware.Render(c, http.StatusOK, startHereBlock(view))
	}
	view.Empty = false
	view.Name = hp.Entity.Name
	view.Href = "/campaigns/" + cc.Campaign.ID + "/entities/" + hp.Entity.ID
	view.DMOnly = hp.Director && hp.Entity.IsPrivate
	view.Line = startHereFirstLine(hp.HTML)
	return middleware.Render(c, http.StatusOK, startHereBlock(view))
}
