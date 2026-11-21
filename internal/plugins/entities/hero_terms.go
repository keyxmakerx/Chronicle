package entities

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// hero_terms.go serves the rules words the hero builder underlines: a hover
// meaning for a word like "slowed" in an entry's text. The words come from the
// game system's Rules book glossary through HeroTermsReader, wired at the
// composition root, so this plugin never reads system packages. Each builder
// mount (creator, practice, try-out) has its own route with the same access as
// its plan, so a word is only ever served to someone who may open that builder.

// HeroTerm is one rules word: its printed name, the kind of word it is and a
// plain-text meaning. The text is never HTML.
type HeroTerm struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Text     string `json:"text"`
}

// HeroTerms is the answer to a terms request. Terms is keyed by the word as
// written in {@category term|label} markup, lowercased with whitespace runs
// turned into "-"; the builder normalises the markup it meets the same way.
// Present is false when the system has no glossary, which leaves the URL off
// the mount.
type HeroTerms struct {
	Terms   map[string]HeroTerm `json:"terms"`
	Present bool                `json:"-"`
}

// HeroTermsReader supplies a campaign's rules words.
type HeroTermsReader interface {
	HeroTerms(ctx context.Context, campaignID string) (*HeroTerms, error)
}

// SetHeroTerms wires the source of the builder's rules words. Without it no
// mount carries a terms URL and the endpoints answer empty.
func (h *Handler) SetHeroTerms(r HeroTermsReader) { h.heroTerms = r }

// termsCacheControl keeps a campaign's words in the reader's own browser only:
// the glossary is the same for everyone but is served behind campaign access,
// so no shared cache may hold it. Five minutes lets a package update show
// without a hard refresh.
const termsCacheControl = "private, max-age=300"

// readTerms returns the campaign's words, never nil.
func (h *Handler) readTerms(ctx context.Context, campaignID string) (*HeroTerms, error) {
	if h.heroTerms == nil {
		return &HeroTerms{Terms: map[string]HeroTerm{}}, nil
	}
	t, err := h.heroTerms.HeroTerms(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	if t == nil || t.Terms == nil {
		return &HeroTerms{Terms: map[string]HeroTerm{}}, nil
	}
	return t, nil
}

// termsURLIf is url when the system has a glossary and "" otherwise, so a
// builder for a system without one never asks. A glossary that fails to read
// leaves the URL off rather than breaking the page: the words are an
// enhancement.
func (h *Handler) termsURLIf(ctx context.Context, campaignID, url string) string {
	t, err := h.readTerms(ctx, campaignID)
	if err != nil {
		slog.Warn("reading the rules words for the hero builder", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return ""
	}
	if !t.Present {
		return ""
	}
	return url
}

// serveTerms writes the words as the builder reads them. A system with no
// glossary is a normal 200 with no words, not an error.
func (h *Handler) serveTerms(c *echo.Context, campaignID string) error {
	t, err := h.readTerms(c.Request().Context(), campaignID)
	if err != nil {
		return err
	}
	c.Response().Header().Set("Cache-Control", termsCacheControl)
	return c.JSON(http.StatusOK, t)
}

// HeroTermsAPI returns the creator's rules words
// (GET /campaigns/:id/characters/new/terms), to whoever may read its plan.
func (h *Handler) HeroTermsAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.requireHeroPlanAccess(c, cc); err != nil {
		return err
	}
	return h.serveTerms(c, cc.Campaign.ID)
}

// PracticeTermsAPI returns the practice builder's rules words
// (GET /campaigns/:id/characters/practice/terms), to whoever may make a
// practice hero.
func (h *Handler) PracticeTermsAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if _, err := h.practiceMaker(c, cc); err != nil {
		return err
	}
	return h.serveTerms(c, cc.Campaign.ID)
}

// TryoutTermsAPI returns the try-out builder's rules words
// (GET /campaigns/:id/characters/try/terms), to a look-around visitor on a
// live pass. Read-only like the rest of the try-out.
func (h *Handler) TryoutTermsAPI(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	if err := h.tryoutReady(c, cc); err != nil {
		return err
	}
	return h.serveTerms(c, cc.Campaign.ID)
}
