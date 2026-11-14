package trackers

// The campaign home's "The world's clocks" block: trackers a Director shows
// to everyone, drawn small as a ring, a name and what the value means.
//
// Which trackers and pages a viewer may see is decided by Service.Shared,
// which reads as a plain player for everyone, the owner included. This file
// only words them, so the template holds nothing that was left out.

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/labstack/echo/v5"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/middleware"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// worldClocksShown is how many rings the block draws.
const worldClocksShown = 6

// clockRing is one tracker, already worded.
type clockRing struct {
	Name string
	// Label sits in the ring: "4/6", or just the number when unbounded.
	Label string
	// Pct is the filled share of the ring, 0 to 100; Bounded false draws an
	// empty ring with only the number.
	Pct     int
	Bounded bool
	Sub     string
	Href    string
}

// worldClocksView is what the template draws.
type worldClocksView struct {
	Rings []clockRing
	// More is how many are not drawn, worded; empty when all are.
	More string
	// Hint is the owner's one line when nothing is shown to everyone yet.
	Hint string
}

// ringFor words one shared tracker. A steps tracker counts its steps from 1
// ("step 3 of 6"); a clock and a bounded count measure from their floor.
func ringFor(campaignID string, t SharedTracker) clockRing {
	r := clockRing{Name: t.Name, Sub: t.Now, Href: "/campaigns/" + url.PathEscape(campaignID) + "/entities/" + url.PathEscape(t.EntityID)}
	switch {
	case t.Shape == ShapeSteps && t.Count > 0:
		r.Bounded, r.Pct = true, (t.Value+1)*100/t.Count
		r.Label = fmt.Sprintf("%d/%d", t.Value+1, t.Count)
	case t.Bounded:
		r.Bounded, r.Pct = true, (t.Value-t.Min)*100/(t.Max-t.Min)
		r.Label = fmt.Sprintf("%d/%d", t.Value, t.Max)
	default:
		r.Label = fmt.Sprint(t.Value)
	}
	if r.Pct < 0 {
		r.Pct = 0
	}
	if r.Pct > 100 {
		r.Pct = 100
	}
	return r
}

// buildWorldClocks words up to worldClocksShown trackers. owner picks the
// hint shown when there are none.
func buildWorldClocks(campaignID string, list []SharedTracker, owner bool) worldClocksView {
	var v worldClocksView
	for i, t := range list {
		if i >= worldClocksShown {
			v.More = fmt.Sprintf("%d more, on their pages", len(list)-worldClocksShown)
			break
		}
		v.Rings = append(v.Rings, ringFor(campaignID, t))
	}
	if len(list) == 0 && owner {
		v.Hint = "Nothing is shown to everyone yet. Set a tracker to Everyone and it appears here."
	}
	return v
}

// WorldClocksBlock draws the block. A guest of a public campaign gets it only
// where the owner placed it on the Guests home; players and guests get an
// empty answer when there is nothing to show, and the owner a one-line hint.
// GET /campaigns/:id/world-clocks
func (h *Handler) WorldClocksBlock(c *echo.Context) error {
	cc := campaigns.GetCampaignContext(c)
	if cc == nil {
		return apperror.NewMissingContext()
	}
	campaignID, v, err := viewerFrom(c)
	if err != nil {
		return err
	}
	if !cc.IsMember && !campaigns.GuestHomeHasBlock(cc.Campaign, campaigns.BlockWorldClocks) {
		return c.HTML(http.StatusOK, "")
	}
	list, err := h.svc.Shared(c.Request().Context(), campaignID, v)
	if err != nil {
		return c.HTML(http.StatusOK, "")
	}
	owner := v.Director && !campaigns.ViewingAsPlayer(c, cc)
	view := buildWorldClocks(campaignID, list, owner)
	if len(view.Rings) == 0 && view.Hint == "" {
		return c.HTML(http.StatusOK, "")
	}
	return middleware.Render(c, http.StatusOK, worldClocksBlock(view))
}
