package sessions

// The campaign home's "Getting started" block: the few things a new player
// does before their first night, each ticked off when it is true of them.
// What it knows comes from the viewer alone (a hero they play, times they
// have given), so it needs no storage of its own and goes away by itself when
// every step is done.

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// HeroChecker says whether a member plays a hero. Implemented in
// internal/app over the entities service, so this plugin reads no pages.
type HeroChecker interface {
	// PlayingHero reports whether heroes are claimed in this campaign at all
	// (tracked) and whether userID plays one (has).
	PlayingHero(ctx context.Context, campaignID, userID string) (tracked, has bool, err error)
}

// SetHeroChecker wires the check behind the Getting started block's hero step.
// Without it the step is left out.
func (h *Handler) SetHeroChecker(c HeroChecker) { h.heroes = c }

// startStep is one line of the block.
type startStep struct {
	Title, Hint, Label, Href string
	Done                     bool
}

// gettingStartedSteps words the steps; tracked is whether the hero step applies.
func gettingStartedSteps(campaignID string, heroTracked, hasHero, answered bool) []startStep {
	base := "/campaigns/" + campaignID
	var steps []startStep
	if heroTracked {
		steps = append(steps, startStep{
			Title: "Choose your hero", Hint: "Claim a character so the table knows who you play.",
			Label: "Choose a hero", Href: base + "/characters", Done: hasHero,
		})
	}
	steps = append(steps, startStep{
		Title: "Say when you are free", Hint: "It is how game nights get picked.",
		Label: "Set my times", Href: base + "/availability", Done: answered,
	})
	return steps
}

// GettingStartedBlock draws the block, or an empty answer that removes the
// placeholder when there is nothing left to do.
// GET /campaigns/:id/getting-started
func (h *Handler) GettingStartedBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	userID := auth.GetUserID(c)
	if !cc.IsMember || userID == "" {
		return c.HTML(http.StatusOK, "")
	}
	ctx := c.Request().Context()
	campaignID := cc.Campaign.ID
	mine, err := h.svc.GetMyAvailability(ctx, campaignID, userID)
	if err != nil {
		slog.Warn("getting started block: availability failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
		return c.HTML(http.StatusOK, "")
	}
	var tracked, has bool
	if h.heroes != nil {
		if tracked, has, err = h.heroes.PlayingHero(ctx, campaignID, userID); err != nil {
			slog.Warn("getting started block: hero check failed", slog.String("campaign_id", campaignID), slog.Any("error", err))
			tracked = false
		}
	}
	steps := gettingStartedSteps(campaignID, tracked, has, mine.Answered)
	left := 0
	for _, s := range steps {
		if !s.Done {
			left++
		}
	}
	if left == 0 {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, gettingStartedBlock(steps, len(steps)-left))
}
