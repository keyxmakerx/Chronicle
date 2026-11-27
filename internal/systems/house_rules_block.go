package systems

// The campaign home's "House rules" block: the titles of the rulebook's
// house-rules chapters, each opening to the chapter's one-line introduction
// and then the chapter's own page, torn out of the book in place (the reader
// widget's leaf mode). The home lives in the campaigns plugin, which
// may not read the rulebook, so its placeholder loads this fragment. What it
// lists is the rulebook as the viewer reads it: a chapter, page or block the
// Director marked Director-only is gone before the list is made.

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// houseRulesShown bounds the list; the rulebook holds the rest.
const houseRulesShown = 6

// houseRule is one chapter as the block lists it.
type houseRule struct {
	Title string
	Intro string
	// Href opens the full rulebook at this chapter; ChapterID is the chapter
	// the leaf draws.
	Href      string
	ChapterID string
}

// houseRulesView is what the block draws.
type houseRulesView struct {
	// Total counts every house chapter the viewer can read; Rules lists the
	// first houseRulesShown of them.
	Total int
	Rules []houseRule
	// BookHref opens the rulebook.
	BookHref string
	// BookURL is the book JSON every leaf reads, one fetch shared by all of
	// them. A Director viewing as a player asks for the player's book, so the
	// leaves show what a player is given.
	BookURL string
	// CampaignID is the campaign the leaves' widgets mount in.
	CampaignID string
}

// houseRulesFrom lists the house chapters of a book already filtered for its
// viewer. base is the rulebook's address; a chapter links to its id under it.
// asPlayer asks the leaves for the player's book.
func houseRulesFrom(b *Book, base, campaignID string, asPlayer bool) houseRulesView {
	view := houseRulesView{BookHref: base, BookURL: base + "/book", CampaignID: campaignID}
	if asPlayer {
		view.BookURL += "?view=player"
	}
	if b == nil {
		return view
	}
	for _, p := range b.Parts {
		if p.Title != housePartTitle {
			continue
		}
		for _, ch := range p.Chapters {
			view.Total++
			if len(view.Rules) >= houseRulesShown {
				continue
			}
			view.Rules = append(view.Rules, houseRule{
				Title:     strings.TrimSpace(ch.Title),
				Intro:     strings.TrimSpace(ch.Intro),
				Href:      base + "#" + url.PathEscape(ch.ID),
				ChapterID: ch.ID,
			})
		}
	}
	return view
}

// HouseRulesBlock draws the block, or an empty answer that removes the
// placeholder when the campaign has no house chapter the viewer can read.
// GET /campaigns/:id/house-rules
func (h *SystemHandler) HouseRulesBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	sysID := campaigns.CampaignSystemID(c.Request().Context(), h.addonLister, cc.Campaign)
	if sysID == "" || h.bookEdits == nil {
		return c.HTML(http.StatusOK, "")
	}
	ctx := c.Request().Context()
	pkg, svc, err := h.CampaignBookPackage(cc.Campaign.ID, sysID)
	if err != nil {
		// No rulebook is the ordinary case for a system without one.
		return c.HTML(http.StatusOK, "")
	}
	book, err := svc.Edition(ctx, cc.Campaign.ID, pkg)
	if err != nil {
		slog.Warn("house rules block: rulebook failed to load",
			slog.String("campaign_id", cc.Campaign.ID), slog.Any("error", err))
		return c.HTML(http.StatusOK, "")
	}
	// The Director previewing as a player reads what a player reads.
	asPlayer := campaigns.ViewingAsPlayer(c, cc)
	director := bookViewerIsDirector(cc) && !asPlayer
	base := "/campaigns/" + cc.Campaign.ID + "/systems/" + url.PathEscape(sysID)
	view := houseRulesFrom(FilterBook(book, director), base, cc.Campaign.ID, asPlayer)
	if len(view.Rules) == 0 {
		return c.HTML(http.StatusOK, "")
	}
	c.Response().Header().Set("Cache-Control", "private, no-store")
	return middleware.Render(c, http.StatusOK, houseRulesBlock(view))
}
