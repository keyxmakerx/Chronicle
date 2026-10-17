package records

import (
	"context"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/patch"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/calendar"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// TierLister reads the campaign's event importance levels (its tier
// vocabulary, or the platform's default trio). The campaigns service is one.
type TierLister interface {
	GetEventTierDefinitions(ctx context.Context, campaignID string) ([]campaigns.TierDefinition, error)
}

type calendarByID interface {
	GetCalendarForViewer(ctx context.Context, calendarID, campaignID string, v permissions.Viewer) (*calendar.Calendar, error)
}

// eventCalendar is the calendar an event row writes to: the one its
// `calendar:` names, else the main one. A calendar the operator cannot see
// reads as missing, so its name is never confirmed.
func (k EventKind) eventCalendar(ctx context.Context, campaignID string, a Actor, r Record) (*calendar.Calendar, error) {
	name := r.Str(fieldCalendar)
	if name == "" {
		return defaultCalendar(ctx, k.Svc, campaignID, a)
	}
	if p := pendingCalendar(ctx); p != nil && sameName(p.Name, name) {
		return p, nil
	}
	l, lok := k.Svc.(calendarLister)
	g, gok := k.Svc.(calendarByID)
	if !lok || !gok {
		return nil, badRequestf("calendar: only the main calendar can be written to here; leave calendar out")
	}
	cals, err := l.ListCalendars(ctx, campaignID, a.Viewer())
	if err != nil {
		return nil, badRequestf("could not read the campaign's calendars; try again in a moment")
	}
	var names []string
	for _, c := range cals {
		if sameName(c.Name, name) {
			cal, err := g.GetCalendarForViewer(ctx, c.ID, campaignID, a.Viewer())
			if err != nil || cal == nil {
				break
			}
			return cal, nil
		}
		names = append(names, c.Name)
	}
	if len(names) == 0 {
		return nil, badRequestf("calendar: there is no calendar called %s", quote(name))
	}
	return nil, badRequestf("calendar: there is no calendar called %s; the calendars are %s", quote(name), strings.Join(names, ", "))
}

// eventExtras are an event row's importance, category and linked page. Each
// is absent (keep), cleared with `none` (update only), or set; a name that
// matches nothing is refused with the names that do exist.
type eventExtras struct {
	tier, page patch.Field[string]
	kindID     patch.Field[int]
	labels     []string // for the review summary
}

// readEventExtras checks the keys against the campaign's importance levels,
// the calendar's event categories and the pages the operator may open.
func (k EventKind) readEventExtras(ctx context.Context, campaignID string, a Actor, cal *calendar.Calendar, r Record) (eventExtras, error) {
	var x eventExtras
	clears := func(key string) (bool, error) {
		if !strings.EqualFold(r.Str(key), pageNone) {
			return false, nil
		}
		if r.Action != ActionUpdate {
			return false, badRequestf("%s: none only applies to action: update; leave %s out instead", key, key)
		}
		return true, nil
	}

	if r.Has("importance") {
		want := r.Str("importance")
		if want == "" {
			return x, badRequestf("importance needs a level, or none to go back to the usual one")
		}
		if none, err := clears("importance"); err != nil {
			return x, err
		} else if none {
			x.tier = patch.Null[string]()
			x.labels = append(x.labels, "usual importance")
		} else {
			if k.Tiers == nil {
				return x, badRequestf("importance can't be set here; leave it out")
			}
			defs, err := k.Tiers.GetEventTierDefinitions(ctx, campaignID)
			if err != nil {
				return x, badRequestf("could not read the importance levels; try again in a moment")
			}
			var names []string
			for _, d := range defs {
				if sameName(d.Slug, want) || sameName(d.Name, want) {
					x.tier = patch.Of(d.Slug)
					x.labels = append(x.labels, d.Name+" importance")
					break
				}
				names = append(names, d.Name)
			}
			if !x.tier.Present() {
				return x, badRequestf("importance: %s isn't one of this campaign's levels (%s)", quote(want), strings.Join(names, ", "))
			}
		}
	}

	if r.Has("category") {
		want := r.Str("category")
		if want == "" {
			return x, badRequestf("category needs a category name, or none to clear it")
		}
		if none, err := clears("category"); err != nil {
			return x, err
		} else if none {
			x.kindID = patch.Null[int]()
			x.labels = append(x.labels, "no category")
		} else {
			var names []string
			for _, ek := range cal.EventKinds {
				if sameName(ek.Slug, want) || sameName(ek.Name, want) {
					x.kindID = patch.Of(ek.ID)
					x.labels = append(x.labels, "category "+ek.Name)
					break
				}
				names = append(names, ek.Name)
			}
			if !x.kindID.Present() {
				if len(names) == 0 {
					return x, badRequestf("category: this campaign has no event categories yet; make them on the Calendar page, or leave category out")
				}
				return x, badRequestf("category: %s isn't one of the event categories (%s)", quote(want), strings.Join(names, ", "))
			}
		}
	}

	if r.Has("page") {
		ref, err := readPageRef(r, "page")
		if err != nil {
			return x, err
		}
		if ref.none {
			if r.Action != ActionUpdate {
				return x, badRequestf("page: none only applies to action: update; leave page out instead")
			}
			x.page = patch.Null[string]()
			x.labels = append(x.labels, "no linked page")
		} else {
			if err := resolvePageRef(ctx, k.Pages, campaignID, a, &ref, "page"); err != nil {
				return x, err
			}
			x.page = patch.Of(ref.id)
			x.labels = append(x.labels, "linked to "+ref.name)
		}
	}
	return x, nil
}

// summary adds the extras to a review line.
func (x eventExtras) summary(s string) string {
	if len(x.labels) == 0 {
		return s
	}
	return s + "; " + strings.Join(x.labels, ", ")
}

func (x eventExtras) toCreate(in *calendar.CreateEventInput) {
	if v, ok := x.tier.Get(); ok {
		in.Tier = &v
	}
	if v, ok := x.kindID.Get(); ok {
		in.KindID = &v
	}
	if v, ok := x.page.Get(); ok {
		in.EntityID = &v
	}
}

func (x eventExtras) toUpdate(in *calendar.UpdateEventInput) {
	in.Tier, in.KindID, in.EntityID = x.tier, x.kindID, x.page
}
