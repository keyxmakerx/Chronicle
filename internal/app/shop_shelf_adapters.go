package app

import (
	"context"
	"net/http"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/armory"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/widgets/relations"
)

// shelfCalendarService is the part of the calendar service today's shelf reads.
type shelfCalendarService interface {
	GetDefaultCalendarForViewer(ctx context.Context, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
	ListCalendars(ctx context.Context, campaignID string, v permissions.Viewer) ([]calendar.Calendar, error)
}

// shelfDayAdapter reads the shelf day from the default calendar. The shelf
// changes at dawn, so the hours before the day's first light still belong
// to the previous day. The day count is read with the Director's sight (a
// trusted in-process read, ADR-049's allowlist) so every viewer gets the
// same shelf; only a count leaves here, never the date. The hours to dawn
// tell the time of day, so they are given only to a viewer the calendar is
// visible to. That check is the calendar list, one query under the same
// visibility rule, rather than a second full read of the calendar.
type shelfDayAdapter struct {
	cal shelfCalendarService
}

func (a *shelfDayAdapter) ShelfDay(ctx context.Context, campaignID string, role int, userID string) (*armory.ShelfDay, error) {
	if a.cal == nil {
		return nil, nil
	}
	c, err := a.cal.GetDefaultCalendarForViewer(ctx, campaignID, permissions.SystemViewer(permissions.RoleOwner))
	if err != nil {
		if apperror.SafeCode(err) == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	if c == nil || c.CurrentMonth < 1 || c.CurrentDay < 1 {
		return nil, nil
	}
	d := shelfDayOf(c)
	if role < permissions.RoleOwner {
		seen, err := a.cal.ListCalendars(ctx, campaignID, permissions.RequestViewer(role, userID))
		if err != nil {
			return nil, err
		}
		if !calendarListed(seen, c.ID) {
			d.HoursToDawn = -1
		}
	}
	return d, nil
}

// calendarListed reports whether the calendar id is among cals.
func calendarListed(cals []calendar.Calendar, id string) bool {
	for i := range cals {
		if cals[i].ID == id {
			return true
		}
	}
	return false
}

// shelfDayOf works out the shelf day at the calendar's current time.
func shelfDayOf(c *calendar.Calendar) *armory.ShelfDay {
	hpd, mph := c.HoursPerDay, c.MinutesPerHour
	if hpd <= 0 {
		hpd = 24
	}
	if mph <= 0 {
		mph = 60
	}
	y, m, d := c.CurrentYear, c.CurrentMonth, c.CurrentDay
	h, mi := min(max(c.CurrentHour, 0), hpd-1), min(max(c.CurrentMinute, 0), mph-1)
	dawn := dawnHour(c, y, m, d, hpd)
	idx := int64(c.CurrentDayIndex())
	now := h*mph + mi
	var left int
	if h < dawn {
		idx--
		left = dawn*mph - now
	} else {
		// Tomorrow's dawn is read off today's: a day apart, the sun rises
		// within the same hour.
		left = (hpd+dawn)*mph - now
	}
	return &armory.ShelfDay{Index: idx, HoursToDawn: (left + mph - 1) / mph}
}

// dawnHour is the first hour of the date that is not night, or 0 on a date
// the sun never rises.
func dawnHour(c *calendar.Calendar, y, m, d, hpd int) int {
	for hr := 0; hr < hpd; hr++ {
		if c.DayPart(y, m, d, hr, 0) != calendar.DayPartNight {
			return hr
		}
	}
	return 0
}

// shopSellsAdapter lists a shop's "sells" listings from the relations widget.
type shopSellsAdapter struct {
	svc relations.RelationService
}

func (a *shopSellsAdapter) ListSells(ctx context.Context, campaignID, shopEntityID string) ([]armory.ShopListing, error) {
	rels, err := a.svc.ListByEntity(ctx, campaignID, shopEntityID)
	if err != nil {
		return nil, err
	}
	out := make([]armory.ShopListing, 0, len(rels))
	for _, r := range rels {
		if r.RelationType != "sells" || r.SourceEntityID != shopEntityID || r.CampaignID != campaignID {
			continue
		}
		out = append(out, armory.ShopListing{RelationID: r.ID, TargetEntityID: r.TargetEntityID, DmOnly: r.DmOnly, Metadata: r.Metadata})
	}
	return out, nil
}
