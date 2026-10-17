package maps

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The entity page's "Where it is" block answers one question: which map is
// this page on, and what does that map sit inside? The ladder is read from the
// pins the DM already placed in Maps (an entity pin, then the pins that open
// maps); nothing about it is stored on the page.
//
// Every pin is read through ListMarkers for the viewer, so a pin hidden from
// them (dm_only, rules, shadow, fog, an entity they cannot see) adds no step
// and no gap: a step is reached only over pins they can follow.

// WhereItIsBlockType is the entity page block's type key in saved layouts: an
// append-only namespace value, never rename.
const WhereItIsBlockType = "where_it_is"

const (
	// maxWhereStarts caps how many maps pinning the entity are read.
	maxWhereStarts = 50
	// maxWhereParents caps how many maps linking to one map are read per step.
	maxWhereParents = 50
)

// WhereStep is one map on the ladder.
type WhereStep struct {
	// Map is the viewer's version of the map (players get the player-safe
	// picture), so a thumbnail built from it cannot leak an original.
	Map Map
	// HasPin is set when PinX/PinY (percent of the picture) mark a spot to
	// outline: the entity's own pin on the first step, otherwise the pin on
	// this map that opens the closer step.
	HasPin     bool
	PinX, PinY float64
	// DMOnly is set when players cannot reach this step. Only a viewer who can
	// see DM-only content ever receives such a step.
	DMOnly bool
}

// WhereAlso is a map the entity is pinned on that the ladder does not pass
// through.
type WhereAlso struct {
	Map    Map
	DMOnly bool
}

// WhereLadder is the entity's place in the maps, closest first.
type WhereLadder struct {
	Steps []WhereStep
	Also  []WhereAlso
	// Truncated is set when the depth bound stopped a ladder that goes further.
	Truncated bool
}

// whereEdge is one pin that could be the next step out: Pin sits on From.
type whereEdge struct {
	From *Map
	Pin  *Marker
	// Open is whether players can follow it (always true for a player viewer,
	// who only ever sees what they can follow).
	Open bool
}

// WhereIs implements MapService.
func (s *mapService) WhereIs(ctx context.Context, campaignID, entityID string, role int, userID string) (*WhereLadder, error) {
	ms, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps for where-is: %w", err)
	}
	byID := make(map[string]*Map, len(ms))
	for i := range ms {
		byID[ms[i].ID] = &ms[i]
	}
	dm := permissions.CanSeeDmOnly(role)

	// Reads are memoised: the same map is asked about once as the viewer and,
	// for a DM, once as the baseline player who tells which pins are DM-only.
	type key struct {
		mapID  string
		player bool
	}
	memo := make(map[key][]Marker)
	pinsOn := func(mapID string, asPlayer bool) ([]Marker, error) {
		k := key{mapID, asPlayer}
		if v, ok := memo[k]; ok {
			return v, nil
		}
		r, uid := role, userID
		if asPlayer {
			r, uid = int(permissions.RolePlayer), ""
		}
		v, err := s.ListMarkers(ctx, campaignID, mapID, r, uid)
		if err != nil {
			return nil, err
		}
		memo[k] = v
		return v, nil
	}
	// openPins is the set of pin ids on a map that an ordinary player sees.
	openPins := func(mapID string) (map[string]bool, error) {
		out := map[string]bool{}
		v, err := pinsOn(mapID, true)
		if err != nil {
			return nil, err
		}
		for i := range v {
			out[v[i].ID] = true
		}
		return out, nil
	}

	pinnedIDs, err := s.repo.ListMapsPinningEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list maps pinning entity: %w", err)
	}
	if len(pinnedIDs) > maxWhereStarts {
		pinnedIDs = pinnedIDs[:maxWhereStarts]
	}
	var starts []whereEdge
	for _, id := range pinnedIDs {
		m := byID[id]
		if m == nil {
			continue
		}
		pins, err := pinsOn(id, false)
		if err != nil {
			return nil, err
		}
		var open map[string]bool
		if dm {
			if open, err = openPins(id); err != nil {
				return nil, err
			}
		}
		// A map with several pins of the entity is outlined at the first, and
		// is open to players if any of them is.
		var pick *whereEdge
		for i := range pins {
			if pins[i].EntityID == nil || *pins[i].EntityID != entityID {
				continue
			}
			e := whereEdge{From: m, Pin: &pins[i], Open: !dm || open[pins[i].ID]}
			if pick == nil || (e.Open && !pick.Open) {
				c := e
				pick = &c
			}
		}
		if pick != nil {
			starts = append(starts, *pick)
		}
	}
	ladder := &WhereLadder{}
	if len(starts) == 0 {
		return ladder, nil
	}
	// The DM's ladder leads with a map players can see, so the part of it they
	// share with a player is the same walk a player takes.
	sort.SliceStable(starts, func(i, j int) bool { return starts[i].Open && !starts[j].Open })

	first := starts[0]
	visited := map[string]bool{first.From.ID: true}
	steps := []WhereStep{{Map: *first.From, HasPin: true, PinX: first.Pin.X, PinY: first.Pin.Y, DMOnly: !first.Open}}

	for len(steps) < MaxLinkTreeDepth {
		cur := steps[len(steps)-1]
		next, err := s.nextWhereEdge(ctx, campaignID, cur.Map.ID, byID, visited, dm, pinsOn, openPins)
		if err != nil {
			return nil, err
		}
		if next == nil {
			break
		}
		visited[next.From.ID] = true
		steps = append(steps, WhereStep{
			Map: *next.From, HasPin: true, PinX: next.Pin.X, PinY: next.Pin.Y,
			DMOnly: cur.DMOnly || !next.Open,
		})
	}
	if len(steps) == MaxLinkTreeDepth {
		more, err := s.nextWhereEdge(ctx, campaignID, steps[len(steps)-1].Map.ID, byID, visited, dm, pinsOn, openPins)
		if err != nil {
			return nil, err
		}
		ladder.Truncated = more != nil
	}

	for _, st := range starts[1:] {
		if !visited[st.From.ID] {
			ladder.Also = append(ladder.Also, WhereAlso{Map: *st.From, DMOnly: !st.Open})
			visited[st.From.ID] = true
		}
	}

	// Pictures go through ForViewer last so a player only ever holds the copy
	// with its shadows smudged in.
	for i := range steps {
		vm, err := s.ForViewer(ctx, &steps[i].Map, role)
		if err != nil {
			return nil, err
		}
		steps[i].Map = *vm
	}
	for i := range ladder.Also {
		vm, err := s.ForViewer(ctx, &ladder.Also[i].Map, role)
		if err != nil {
			return nil, err
		}
		ladder.Also[i].Map = *vm
	}
	ladder.Steps = steps
	return ladder, nil
}

// nextWhereEdge picks the pin that takes the ladder one map further out from
// mapID: a pin, on a map not yet on the ladder, that opens mapID. Several
// maps may open it; the pick is deterministic (a map players can follow into
// first, then by name, then id), so the same page always draws the same ladder
// and a cycle cannot send it round again (visited).
func (s *mapService) nextWhereEdge(
	ctx context.Context, campaignID, mapID string, byID map[string]*Map, visited map[string]bool, dm bool,
	pinsOn func(string, bool) ([]Marker, error), openPins func(string) (map[string]bool, error),
) (*whereEdge, error) {
	srcs, err := s.repo.ListMapsLinkingTo(ctx, campaignID, mapID)
	if err != nil {
		return nil, fmt.Errorf("list maps linking to map: %w", err)
	}
	if len(srcs) > maxWhereParents {
		srcs = srcs[:maxWhereParents]
	}
	var cands []whereEdge
	for _, id := range srcs {
		from := byID[id]
		if from == nil || visited[id] {
			continue
		}
		pins, err := pinsOn(id, false)
		if err != nil {
			return nil, err
		}
		var open map[string]bool
		if dm {
			if open, err = openPins(id); err != nil {
				return nil, err
			}
		}
		var pick *whereEdge
		for i := range pins {
			if pins[i].LinkedMapID == nil || *pins[i].LinkedMapID != mapID {
				continue
			}
			e := whereEdge{From: from, Pin: &pins[i], Open: !dm || open[pins[i].ID]}
			if pick == nil || (e.Open && !pick.Open) {
				c := e
				pick = &c
			}
		}
		if pick != nil {
			cands = append(cands, *pick)
		}
	}
	if len(cands) == 0 {
		return nil, nil
	}
	sort.SliceStable(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.Open != b.Open {
			return a.Open
		}
		an, bn := strings.ToLower(a.From.Name), strings.ToLower(b.From.Name)
		if an != bn {
			return an < bn
		}
		return a.From.ID < b.From.ID
	})
	return &cands[0], nil
}
