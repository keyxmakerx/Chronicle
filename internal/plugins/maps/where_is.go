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
	// maxWhereStarts caps how many maps the viewer can see the entity pinned
	// on, and maxWhereParents how many maps they can see opening one map. The
	// caps count visible results, so pins hidden from the viewer never use them up.
	maxWhereStarts  = 50
	maxWhereParents = 50
	// maxWhereReads is the most marker lists one call reads (a DM's baseline
	// player read counts too), so the cost is bounded however many maps link
	// to each other. Past it the ladder stops and says so.
	maxWhereReads = 200
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
	// Truncated is set when the depth bound, a cap on visible maps or the read
	// budget kept the ladder or the Also list from being complete.
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

// whereWalk is one WhereIs call: the viewer, what has been read, and the
// budget. Marker lists are memoised per map and role, and each real read is
// counted against maxWhereReads.
type whereWalk struct {
	s          *mapService
	ctx        context.Context
	campaignID string
	role       int
	userID     string
	dm         bool
	byID       map[string]*Map
	memo       map[whereKey][]Marker
	reads      int
	truncated  bool
}

type whereKey struct {
	mapID  string
	player bool
}

// pinsOn is the map's pins for the viewer (or, with asPlayer, for an ordinary
// player). ok is false once the read budget is spent.
func (w *whereWalk) pinsOn(mapID string, asPlayer bool) (pins []Marker, ok bool, err error) {
	k := whereKey{mapID, asPlayer}
	if v, hit := w.memo[k]; hit {
		return v, true, nil
	}
	if w.reads >= maxWhereReads {
		w.truncated = true
		return nil, false, nil
	}
	w.reads++
	r, uid := w.role, w.userID
	if asPlayer {
		r, uid = int(permissions.RolePlayer), ""
	}
	v, err := w.s.ListMarkers(w.ctx, w.campaignID, mapID, r, uid)
	if err != nil {
		return nil, false, err
	}
	w.memo[k] = v
	return v, true, nil
}

// edgeOn finds, among the viewer's pins on map from, the pin that matches
// (the entity's pin, or a pin opening a map). A DM also learns whether players
// can follow it. ok is false when the budget ran out or nothing matches.
func (w *whereWalk) edgeOn(from *Map, match func(*Marker) bool) (*whereEdge, error) {
	pins, ok, err := w.pinsOn(from.ID, false)
	if err != nil || !ok {
		return nil, err
	}
	var open map[string]bool
	if w.dm {
		pp, ok, err := w.pinsOn(from.ID, true)
		if err != nil || !ok {
			return nil, err
		}
		open = make(map[string]bool, len(pp))
		for i := range pp {
			open[pp[i].ID] = true
		}
	}
	// With several matching pins the map is outlined at the first, and is open
	// to players if any of them is.
	var pick *whereEdge
	for i := range pins {
		if !match(&pins[i]) {
			continue
		}
		e := whereEdge{From: from, Pin: &pins[i], Open: !w.dm || open[pins[i].ID]}
		if pick == nil || (e.Open && !pick.Open) {
			c := e
			pick = &c
		}
	}
	return pick, nil
}

// WhereIs implements MapService. Cost: one query when the entity is on no map
// (the common page); otherwise at most maxWhereReads marker-list reads plus
// two queries per step, whatever the campaign holds.
func (s *mapService) WhereIs(ctx context.Context, campaignID, entityID string, role int, userID string) (*WhereLadder, error) {
	pinnedIDs, err := s.repo.ListMapsPinningEntity(ctx, campaignID, entityID)
	if err != nil {
		return nil, fmt.Errorf("list maps pinning entity: %w", err)
	}
	ladder := &WhereLadder{}
	if len(pinnedIDs) == 0 {
		return ladder, nil
	}
	ms, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps for where-is: %w", err)
	}
	w := &whereWalk{
		s: s, ctx: ctx, campaignID: campaignID, role: role, userID: userID,
		dm: permissions.CanSeeDmOnly(role), byID: make(map[string]*Map, len(ms)), memo: map[whereKey][]Marker{},
	}
	for i := range ms {
		w.byID[ms[i].ID] = &ms[i]
	}

	// The cap counts maps the viewer can see the entity on, so pins hidden
	// from them cannot crowd the visible ones out.
	var starts []whereEdge
	for _, id := range pinnedIDs {
		m := w.byID[id]
		if m == nil {
			continue
		}
		if len(starts) >= maxWhereStarts || w.reads >= maxWhereReads {
			w.truncated = true
			break
		}
		e, err := w.edgeOn(m, func(mk *Marker) bool { return mk.EntityID != nil && *mk.EntityID == entityID })
		if err != nil {
			return nil, err
		}
		if e != nil {
			starts = append(starts, *e)
		}
	}
	if len(starts) == 0 {
		ladder.Truncated = w.truncated
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
		next, err := w.nextEdge(cur.Map.ID, visited)
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
		more, err := w.nextEdge(steps[len(steps)-1].Map.ID, visited)
		if err != nil {
			return nil, err
		}
		if more != nil {
			w.truncated = true
		}
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
	ladder.Truncated = w.truncated
	return ladder, nil
}

// nextEdge picks the pin that takes the ladder one map further out from
// mapID: a pin, on a map not yet on the ladder, that opens mapID. Several
// maps may open it; the pick is deterministic (a map players can follow into
// first, then by name, then id), so the same page always draws the same ladder
// and a cycle cannot send it round again (visited). Reading stops at
// maxWhereParents visible candidates or when the read budget is spent, and
// either marks the walk truncated.
func (w *whereWalk) nextEdge(mapID string, visited map[string]bool) (*whereEdge, error) {
	srcs, err := w.s.repo.ListMapsLinkingTo(w.ctx, w.campaignID, mapID)
	if err != nil {
		return nil, fmt.Errorf("list maps linking to map: %w", err)
	}
	var cands []whereEdge
	for _, id := range srcs {
		from := w.byID[id]
		if from == nil || visited[id] {
			continue
		}
		if len(cands) >= maxWhereParents || w.reads >= maxWhereReads {
			w.truncated = true
			break
		}
		e, err := w.edgeOn(from, func(mk *Marker) bool { return mk.LinkedMapID != nil && *mk.LinkedMapID == mapID })
		if err != nil {
			return nil, err
		}
		if e != nil {
			cands = append(cands, *e)
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
