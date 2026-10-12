// Package calendar - moon_lore.go answers the entity page's question "which
// moons is this page the lore of, and what do they look like tonight?". A
// moon's lore page is the one link between a moon and an entity; this is the
// read from the entity's side, so the page can show a chip without the
// entities plugin ever touching a calendar table.
package calendar

import (
	"context"
	"fmt"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// EntityMoonRef is a moon as seen from its lore page: enough to draw a chip
// ("Luna, Waxing Gibbous tonight") and link into that calendar, with the
// phase already worked out so the caller needs no calendar maths.
type EntityMoonRef struct {
	MoonID       int    `json:"moon_id"`
	MoonName     string `json:"moon_name"`
	CalendarID   string `json:"calendar_id"`
	CalendarName string `json:"calendar_name"`
	// PhaseName is the moon's named phase on the calendar's current date.
	PhaseName string `json:"phase_name"`
}

// MoonsForEntity lists the moons in campaignID whose lore page is entityID,
// across every calendar v may open. A moon hidden from players is left out
// for anyone who does not skip the per-user layer, the same rule
// finishCalendarForViewer applies, so a chip can never name a moon the
// calendar itself would not show that viewer.
//
// It does not judge whether v may see the entity: the entity page that asks
// has already shown it to them.
func (s *calendarService) MoonsForEntity(ctx context.Context, campaignID, entityID string, v permissions.Viewer) ([]EntityMoonRef, error) {
	cals, err := s.ListCalendars(ctx, campaignID, v)
	if err != nil {
		return nil, err
	}
	var out []EntityMoonRef
	for i := range cals {
		cal := &cals[i]
		moons, err := s.calRepo.GetMoons(ctx, cal.ID)
		if err != nil {
			return nil, fmt.Errorf("load moons: %w", err)
		}
		var mine []Moon
		for _, m := range filterMoonsForViewer(moons, v) {
			if m.LoreEntityID != nil && *m.LoreEntityID == entityID {
				mine = append(mine, m)
			}
		}
		if len(mine) == 0 {
			continue
		}
		// Months and weekdays are only needed to turn today's date into the
		// day counter moon phases read, so they are loaded only for a
		// calendar that has a moon to name.
		if err := s.loadCalendarGeometry(ctx, cal); err != nil {
			return nil, err
		}
		day := cal.CurrentDayIndex()
		for j := range mine {
			out = append(out, EntityMoonRef{
				MoonID:       mine[j].ID,
				MoonName:     mine[j].Name,
				CalendarID:   cal.ID,
				CalendarName: cal.Name,
				PhaseName:    mine[j].MoonPhaseName(day),
			})
		}
	}
	return out, nil
}
