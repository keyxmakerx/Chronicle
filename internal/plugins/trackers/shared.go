package trackers

// Shared trackers: the read behind the campaign home's "The world's clocks"
// block. It is deliberately narrower than Page: only trackers set to
// "everyone" and only pages a plain player may open, whoever asks.

import (
	"context"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// sharedPageScan bounds how many distinct pages one read checks, so a campaign
// with a great many tracked pages cannot make the home slow. It is an abuse
// limit, not a display one.
const sharedPageScan = 60

// SharedTracker is a shown-to-everyone tracker reduced to what a small ring
// needs. It carries no log, notes or kind id.
type SharedTracker struct {
	EntityID string
	Name     string
	Shape    string
	Value    int
	// Bounded is true when the value has a top to measure against; Min is
	// the bottom and Max the top (a clock's segments, a steps' last index).
	Bounded  bool
	Min, Max int
	// Count is the number of named steps, for a steps tracker.
	Count int
	// Now is what the value means: the band's or step's words.
	Now string
}

// Shared lists the placed trackers shown to everyone, by name.
func (s *service) Shared(ctx context.Context, campaignID string, v Viewer) ([]SharedTracker, error) {
	// Read as a player, never as the Director: the block is the table's view.
	if v.Director {
		v.UserID = ""
	}
	v.Director = false
	if v.VisibilityRole > permissions.RolePlayer {
		v.VisibilityRole = permissions.RolePlayer
	}
	kinds, _, err := s.kindMap(ctx, campaignID)
	if err != nil {
		return nil, err
	}
	all, err := s.repo.ListAllTrackers(ctx, campaignID)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	canView := map[string]bool{}
	var shown []Tracker
	for _, t := range all {
		if t.Visibility != SeeEveryone {
			continue
		}
		if _, ok := kinds[t.KindID]; !ok {
			continue
		}
		seen, checked := canView[t.EntityID]
		if !checked {
			if len(canView) >= sharedPageScan {
				continue
			}
			a, err := s.entities.Access(ctx, campaignID, t.EntityID, v)
			if err != nil {
				return nil, apperrorInternal(err)
			}
			seen = a.Exists && a.CanView
			canView[t.EntityID] = seen
		}
		if seen {
			shown = append(shown, t)
		}
	}
	out := []SharedTracker{}
	if len(shown) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(shown))
	for _, t := range shown {
		ids = append(ids, t.ID)
	}
	entries, err := s.repo.ListEntries(ctx, campaignID, ids)
	if err != nil {
		return nil, apperrorInternal(err)
	}
	for _, t := range shown {
		k := kinds[t.KindID]
		value, _ := fold(k, entries[t.ID])
		lo, hi, hasMax := bounds(k)
		st := SharedTracker{
			EntityID: t.EntityID, Name: t.Name, Shape: k.Shape, Value: value,
			Bounded: hasMax && (k.Shape == ShapeSteps || hi > lo), Min: lo, Max: hi,
			Count: len(k.Steps), Now: nowText(k, value),
		}
		out = append(out, st)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].EntityID < out[j].EntityID
	})
	return out, nil
}
