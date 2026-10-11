package maps

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// A pin can open another map of its campaign (Marker.LinkedMapID). This file
// holds what is built from those links: the trail a viewer followed, the tree
// of linked maps, and the check that a link names a map it may.
//
// Who can follow a link is the pin's own visibility. Nothing here decides
// that again: every edge of the tree comes out of ListMarkers for the viewer,
// so a link on a pin hidden from them (dm_only, rules, shadow, fog) is never
// even read for them.

// Bounds on what one request may build. Links form a graph that can cycle
// (A opens B, B opens A); each map appears in the tree once, and these cap the
// work and the answer even on a campaign with very many links.
const (
	// MaxLinkTreeNodes caps how many maps one tree holds.
	MaxLinkTreeNodes = 300
	// MaxLinkTreeDepth caps how deep a branch goes.
	MaxLinkTreeDepth = 16
	// MaxTrailSteps caps how many maps a trail names before the current one.
	MaxTrailSteps = 16
	// maxTrailInput caps how many ids of a ?from= list are read at all.
	maxTrailInput = 32
)

// TrailStep is one map on the trail at the top of the viewer.
type TrailStep struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LinkMapOption is a map the pin form's "Opens map" chooser offers.
type LinkMapOption struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ThumbURL string `json:"thumb_url,omitempty"`
}

// LinkVia says who can follow the link that leads to a node: the visibility
// and rules of the pin carrying it. Sent only to viewers who can see DM-only
// content, the only ones the tree tells who else can follow.
type LinkVia struct {
	Visibility   string   `json:"visibility"`
	AllowedUsers []string `json:"allowed_users,omitempty"`
	DeniedUsers  []string `json:"denied_users,omitempty"`
}

// LinkNode is one map in the tree of linked maps.
type LinkNode struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	ThumbURL string     `json:"thumb_url,omitempty"`
	Via      *LinkVia   `json:"via,omitempty"`
	Children []LinkNode `json:"children,omitempty"`

	// The viewer's version of the map's picture, turned into ThumbURL by the
	// handler (signing a media address belongs to the web layer).
	imageID        *string
	playerImageURL string
}

// ThumbSource returns the viewer's version of the node's picture: the player
// copy's address, or the original's media id, or neither.
func (n LinkNode) ThumbSource() (mediaID string, playerURL string) {
	if n.imageID != nil {
		mediaID = *n.imageID
	}
	return mediaID, n.playerImageURL
}

// LinkTree is the campaign's linked maps as one viewer can follow them. Roots
// are maps no visible link leads to, then one map per group reachable only
// through a cycle; a map reached twice is listed under the first map that
// reaches it.
type LinkTree struct {
	Roots []LinkNode `json:"roots"`
	// Count is how many maps the tree holds.
	Count int `json:"count"`
	// Truncated is set when a bound stopped the tree from holding every link.
	Truncated bool `json:"truncated"`
}

// linkEdge is one visible link: the pin on From opens To.
type linkEdge struct {
	From, To string
	Pin      *Marker
}

// foldTrail turns a ?from= list into the trail shown above current. It keeps
// only ids that name a map of the campaign (known: id -> name), never trusts
// the list for anything but display, and keeps the trail a simple path: coming
// back to the current map, or to a map already on the trail, cuts the loop.
// Returns nil when nothing usable is left, so a plain visit shows no trail.
func foldTrail(from []string, current string, known map[string]string) []TrailStep {
	if len(from) > maxTrailInput {
		from = from[:maxTrailInput]
	}
	var steps []TrailStep
	for _, id := range from {
		name, ok := known[id]
		if !ok {
			continue
		}
		if id == current {
			steps = steps[:0]
			continue
		}
		cut := -1
		for i, s := range steps {
			if s.ID == id {
				cut = i
				break
			}
		}
		if cut >= 0 {
			steps = steps[:cut+1]
			continue
		}
		steps = append(steps, TrailStep{ID: id, Name: name})
	}
	if len(steps) == 0 {
		return nil
	}
	if len(steps) > MaxTrailSteps {
		steps = steps[len(steps)-MaxTrailSteps:]
	}
	name, ok := known[current]
	if !ok {
		return nil
	}
	return append(steps, TrailStep{ID: current, Name: name})
}

// ParseTrailParam splits a ?from= value into ids. It bounds the input before
// splitting so an oversized query cannot cost more than a short one.
func ParseTrailParam(raw string) []string {
	if raw == "" {
		return nil
	}
	if len(raw) > maxTrailInput*40 {
		raw = raw[:maxTrailInput*40]
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
		if len(out) == maxTrailInput {
			break
		}
	}
	return out
}

// buildLinkTree arranges visible links into a tree. maps lists the campaign's
// maps in their sort order (already the viewer's version, for thumbnails);
// edges are the links this viewer can follow, in pin order. withVia adds who
// can follow each link, for viewers allowed to know.
//
// A link to a map outside maps, or from a map to itself, is dropped. Each map
// is placed once (seen), which is what keeps a cycle from looping; the depth
// and node bounds stop the work on a huge or adversarial campaign.
func buildLinkTree(maps []Map, edges []linkEdge, withVia bool) LinkTree {
	byID := make(map[string]*Map, len(maps))
	for i := range maps {
		byID[maps[i].ID] = &maps[i]
	}
	kids := make(map[string][]linkEdge)
	incoming := make(map[string]bool)
	inGraph := make(map[string]bool)
	dup := make(map[[2]string]bool)
	for _, e := range edges {
		if e.From == e.To || byID[e.From] == nil || byID[e.To] == nil {
			continue
		}
		k := [2]string{e.From, e.To}
		if dup[k] {
			continue
		}
		dup[k] = true
		kids[e.From] = append(kids[e.From], e)
		incoming[e.To] = true
		inGraph[e.From] = true
		inGraph[e.To] = true
	}

	tree := LinkTree{Roots: []LinkNode{}}
	seen := make(map[string]bool)
	var place func(id string, via *Marker, depth int) LinkNode
	place = func(id string, via *Marker, depth int) LinkNode {
		seen[id] = true
		tree.Count++
		m := byID[id]
		n := LinkNode{ID: id, Name: m.Name, imageID: m.ImageID, playerImageURL: m.PlayerImageURL}
		if withVia && via != nil {
			n.Via = linkViaOf(via)
		}
		for _, e := range kids[id] {
			if seen[e.To] {
				continue
			}
			if depth+1 >= MaxLinkTreeDepth || tree.Count >= MaxLinkTreeNodes {
				tree.Truncated = true
				break
			}
			n.Children = append(n.Children, place(e.To, e.Pin, depth+1))
		}
		return n
	}
	// Maps nothing leads to come first, in the campaign's order.
	for _, m := range maps {
		if !inGraph[m.ID] || seen[m.ID] || incoming[m.ID] {
			continue
		}
		if tree.Count >= MaxLinkTreeNodes {
			tree.Truncated = true
			return tree
		}
		tree.Roots = append(tree.Roots, place(m.ID, nil, 0))
	}
	// What is left is only reachable through a cycle (World opens Isle, a pin
	// on Port opens World again). Each such group starts at the map that opens
	// the most others, which is the top of the hierarchy in practice, so a
	// "back to the world" pin does not turn the tree upside down.
	for {
		best := -1
		for i, m := range maps {
			if !inGraph[m.ID] || seen[m.ID] {
				continue
			}
			if best < 0 || len(kids[m.ID]) > len(kids[maps[best].ID]) {
				best = i
			}
		}
		if best < 0 {
			return tree
		}
		if tree.Count >= MaxLinkTreeNodes {
			tree.Truncated = true
			return tree
		}
		tree.Roots = append(tree.Roots, place(maps[best].ID, nil, 0))
	}
}

// linkViaOf reads who can follow a pin's link from its visibility and rules.
func linkViaOf(mk *Marker) *LinkVia {
	v := &LinkVia{Visibility: mk.Visibility}
	if v.Visibility == "" {
		v.Visibility = "everyone"
	}
	if r := ParseVisibilityRules(mk.VisibilityRules); r != nil {
		v.AllowedUsers = r.AllowedUsers
		v.DeniedUsers = r.DeniedUsers
	}
	return v
}

// linkTarget checks that targetID may be opened by a pin on sourceMapID: a map
// of the same campaign that is not the pin's own. A missing map and another
// campaign's map get the same answer, as requireMapInCampaign gives, so the
// write cannot be used to probe for maps elsewhere.
func (s *mapService) linkTarget(ctx context.Context, sourceMapID, targetID string) (*Map, error) {
	if targetID == sourceMapID {
		return nil, apperror.NewValidation("a pin cannot open the map it is on")
	}
	src, err := s.repo.GetMap(ctx, sourceMapID)
	if err != nil {
		return nil, fmt.Errorf("get map for link: %w", err)
	}
	if src == nil {
		return nil, apperror.NewNotFound("map not found")
	}
	tgt, err := s.repo.GetMap(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("get linked map: %w", err)
	}
	if tgt == nil || tgt.CampaignID != src.CampaignID {
		return nil, apperror.NewValidation("linked map not found")
	}
	return tgt, nil
}

// ResolveTrail implements MapService.
func (s *mapService) ResolveTrail(ctx context.Context, campaignID, mapID string, from []string) ([]TrailStep, error) {
	if len(from) == 0 {
		return nil, nil
	}
	ms, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps for trail: %w", err)
	}
	known := make(map[string]string, len(ms))
	for _, m := range ms {
		known[m.ID] = m.Name
	}
	return foldTrail(from, mapID, known), nil
}

// LinkTree implements MapService. Every edge comes from ListMarkers for this
// viewer, the same filtering the marker list applies (visibility, rules,
// shadows, fog), so a link on a pin they cannot see never reaches the tree.
func (s *mapService) LinkTree(ctx context.Context, campaignID string, role int, userID string) (*LinkTree, error) {
	ms, err := s.repo.ListMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list maps for link tree: %w", err)
	}
	inCampaign := make(map[string]bool, len(ms))
	for _, m := range ms {
		inCampaign[m.ID] = true
	}
	sources, err := s.repo.ListLinkSourceMaps(ctx, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list linking maps: %w", err)
	}
	var edges []linkEdge
	read, skipped := 0, false
	for _, src := range sources {
		if !inCampaign[src] {
			continue
		}
		// One marker read per map that has links; capped like the tree.
		if read >= MaxLinkTreeNodes {
			skipped = true
			break
		}
		read++
		markers, err := s.ListMarkers(ctx, campaignID, src, role, userID)
		if err != nil {
			return nil, err
		}
		for i := range markers {
			mk := &markers[i]
			if mk.LinkedMapID == nil || *mk.LinkedMapID == "" || !inCampaign[*mk.LinkedMapID] {
				continue
			}
			edges = append(edges, linkEdge{From: src, To: *mk.LinkedMapID, Pin: mk})
		}
	}

	// Only maps on a visible link can be nodes; only they need the viewer's
	// version of their picture.
	used := make(map[string]bool)
	for _, e := range edges {
		used[e.From] = true
		used[e.To] = true
	}
	var nodes []Map
	for _, m := range ms {
		if used[m.ID] {
			nodes = append(nodes, m)
		}
	}
	nodes, err = s.ForViewerList(ctx, nodes, role)
	if err != nil {
		return nil, err
	}
	tree := buildLinkTree(nodes, edges, permissions.CanSeeDmOnly(role))
	// Maps past the read cap add no links, so the tree must say it is partial.
	if skipped {
		tree.Truncated = true
	}
	return &tree, nil
}
