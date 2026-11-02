package app

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/calendar/calendarai"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// aiCalendarPages is calendarai.PageResolver over the entities service: a
// page by name, as the actor may open it.
type aiCalendarPages struct{ svc entities.EntityService }

var _ calendarai.PageResolver = aiCalendarPages{}

// OpenablePage refuses a missing page, one of another campaign and one the
// actor may not open alike, so the refusal never confirms a hidden page.
func (p aiCalendarPages) OpenablePage(ctx context.Context, campaignID, name string, a calendarai.Actor) (string, error) {
	if p.svc == nil {
		return "", nil
	}
	e, err := p.svc.GetBySlug(ctx, campaignID, entities.Slugify(name))
	if err != nil || e == nil || e.CampaignID != campaignID {
		return "", nil
	}
	seen, err := p.svc.FilterViewableEntityIDs(ctx, campaignID, []string{e.ID}, a.Role, a.UserID)
	if err != nil {
		return "", err
	}
	if !seen[e.ID] {
		return "", nil
	}
	return e.ID, nil
}

func (aiCalendarPages) Slug(name string) string { return entities.Slugify(name) }
