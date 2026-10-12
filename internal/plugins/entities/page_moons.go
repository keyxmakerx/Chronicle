package entities

// page_moons.go: a page that is a moon's lore page shows a chip under its
// name with the moon and its phase tonight. The calendar plugin knows which
// moons name the page, reached through PageMoons.

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// PageMoon is one moon whose lore page this is.
type PageMoon struct {
	Name         string
	CalendarID   string
	CalendarName string
	// Phase is the moon's named phase on the calendar's current date.
	Phase string
}

// PageMoons lists the moons, on calendars the viewer may open, whose lore
// page is the given page. Wired in internal/app.
type PageMoons interface {
	MoonsForPage(ctx context.Context, campaignID, entityID string, v permissions.Viewer) ([]PageMoon, error)
}

// SetPageMoons wires the moon chip. Unset, no page shows one.
func (h *Handler) SetPageMoons(p PageMoons) { h.pageMoons = p }

type pageMoonsKey struct{}

func withPageMoons(ctx context.Context, m []PageMoon) context.Context {
	return context.WithValue(ctx, pageMoonsKey{}, m)
}

func pageMoonsOf(ctx context.Context) []PageMoon {
	m, _ := ctx.Value(pageMoonsKey{}).([]PageMoon)
	return m
}

// loadPageMoons reads the page's moons; a failure shows no chip.
func (h *Handler) loadPageMoons(ctx context.Context, cc *campaigns.CampaignContext, entityID, userID string) []PageMoon {
	if h.pageMoons == nil {
		return nil
	}
	m, err := h.pageMoons.MoonsForPage(ctx, cc.Campaign.ID, entityID, permissions.RequestViewer(int(cc.VisibilityRole()), userID))
	if err != nil {
		return nil
	}
	return m
}
