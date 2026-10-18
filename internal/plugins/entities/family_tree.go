package entities

import (
	"context"
	"sort"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The entity page's "Family tree" block draws a page's parents, children and
// partners from the entity relations the campaign already has (the Relations
// block writes them). Nothing about a family is stored on the page: the tree
// is read, for one viewer, each time the page is drawn.
//
// What a viewer may see is decided here and nowhere else. A relative the
// viewer cannot open is not read past, not named and not counted, and a
// relation marked DM-only is not followed, so the data a player's block
// carries holds no trace of either and a secret never shows as a gap.

// FamilyTreeBlockType is the block's type key in saved layouts: an append-only
// namespace value, never rename.
const FamilyTreeBlockType = "family_tree"

// FamilyTreeWidgetName is the data-widget the block mounts so boot.js loads its
// script on sight (ADR-063).
const FamilyTreeWidgetName = "family-tree"

const (
	// MaxFamilyGenerations is how many generations the walk follows each way
	// from the page (ancestors up, descendants down). It is read one further
	// to learn whether the family goes on, which is what the block says.
	MaxFamilyGenerations = 4
	// MaxFamilyPeople bounds how many relatives one page carries, so a house
	// of hundreds cannot turn one page view into a wall of data. Nearer
	// relatives are kept first.
	MaxFamilyPeople = 60
	// maxFamilyReads bounds the relation reads (one per person whose own
	// links are read), two for each person kept at the worst.
	maxFamilyReads = 2 * MaxFamilyPeople
)

// Relation kinds on a parent link. The wire values are used by the script.
const (
	FamilyKindBirth    = "birth"
	FamilyKindAdopted  = "adopted"
	FamilyKindGuardian = "guardian"
)

// familySide says which way a relation type points, from the page that holds
// the relation: the other page is this page's parent, child or partner.
type familySide int

const (
	familyNone familySide = iota
	familyOtherIsParent
	familyOtherIsChild
	familyOtherIsPartner
)

type familyRelation struct {
	side familySide
	kind string
}

// familyRelations are the relation types that count as family, matched on the
// trimmed, lower-cased type the relation holds from the reading page's side.
// "parent of" and "child of" are in the Relations block's suggestion list; the
// rest are typed by hand. Anything else (allied with, knows) is not family.
var familyRelations = map[string]familyRelation{
	"parent of":          {familyOtherIsChild, FamilyKindBirth},
	"child of":           {familyOtherIsParent, FamilyKindBirth},
	"adoptive parent of": {familyOtherIsChild, FamilyKindAdopted},
	"adopted child of":   {familyOtherIsParent, FamilyKindAdopted},
	"guardian of":        {familyOtherIsChild, FamilyKindGuardian},
	"ward of":            {familyOtherIsParent, FamilyKindGuardian},
	"married to":         {familyOtherIsPartner, ""},
	"spouse of":          {familyOtherIsPartner, ""},
	"partner of":         {familyOtherIsPartner, ""},
}

// classifyFamilyRelation reports how a relation type reads as family.
func classifyFamilyRelation(relationType string) (familyRelation, bool) {
	r, ok := familyRelations[strings.ToLower(strings.TrimSpace(relationType))]
	return r, ok
}

// FamilyLink is one relation row of a page, in the page's own direction.
type FamilyLink struct {
	OtherID string
	// Type is the relation type as the reading page holds it ("parent of").
	Type   string
	DMOnly bool
}

// FamilyLinks reads a page's relation rows. Implemented in the app by an
// adapter over the relations service, so this plugin never reaches into the
// relations tables.
type FamilyLinks interface {
	LinksOf(ctx context.Context, campaignID, entityID string) ([]FamilyLink, error)
}

// FamilyViewFilter reports which pages a viewer may open. The entity service
// implements it (FilterViewableEntityIDs), the one visibility rule.
type FamilyViewFilter interface {
	FilterViewableEntityIDs(ctx context.Context, campaignID string, entityIDs []string, role int, userID string) (map[string]bool, error)
}

// FamilyParent is a link from a person up to one of their parents.
type FamilyParent struct {
	ID   string
	Kind string
	// DMOnly is set when the relation itself is Director-only; only a viewer
	// who can see DM-only content ever receives such a link.
	DMOnly bool
}

// FamilyPerson is one relative.
type FamilyPerson struct {
	ID       string
	Name     string
	Type     string // the page's category, the line under the name
	Image    string // the stored picture path; empty for none
	Parents  []FamilyParent
	Partners []string
	// Read is set when this person's own relations were read, so their
	// parents, children and partners here are complete. A person at the edge
	// of the walk is not read, and the script will not centre the tree on them.
	Read bool
	// DMOnly is set when players cannot open this page. Only a viewer who can
	// see DM-only content ever receives such a person.
	DMOnly bool
}

// FamilyTree is a page's family for one viewer.
type FamilyTree struct {
	PageID string
	// People holds the page itself first, then relatives nearest first.
	People []FamilyPerson
	// CutGenerations is set when the family goes on past MaxFamilyGenerations,
	// CutPeople when MaxFamilyPeople kept nearer relatives and left others out.
	CutGenerations bool
	CutPeople      bool
}

// Relatives is how many people are drawn besides the page itself.
func (t *FamilyTree) Relatives() int {
	if t == nil || len(t.People) == 0 {
		return 0
	}
	return len(t.People) - 1
}

// Cut reports whether any bound left relatives out.
func (t *FamilyTree) Cut() bool { return t != nil && (t.CutGenerations || t.CutPeople) }

// FamilyTreeService reads a page's family for a viewer.
type FamilyTreeService interface {
	// Tree returns the page's family as role and userID may see it.
	Tree(ctx context.Context, campaignID, entityID string, role int, userID string) (*FamilyTree, error)
}

type familyTreeService struct {
	links  FamilyLinks
	cards  PageCards
	viewer FamilyViewFilter
}

// NewFamilyTreeService creates the family tree reader.
func NewFamilyTreeService(links FamilyLinks, cards PageCards, viewer FamilyViewFilter) FamilyTreeService {
	return &familyTreeService{links: links, cards: cards, viewer: viewer}
}

// familyWalk is one Tree call: the viewer, what has been admitted so far and
// what has been spent.
type familyWalk struct {
	s          *familyTreeService
	ctx        context.Context
	campaignID string
	role       int
	userID     string
	dm         bool

	order   []string
	people  map[string]*FamilyPerson
	judged  map[string]bool // id -> admitted (true) or refused (false)
	dmOnly  map[string]bool
	reads   int
	cutGen  bool
	cutPpl  bool
	linkMem map[string][]FamilyLink
}

func (w *familyWalk) person(id string) *FamilyPerson { return w.people[id] }

// judge decides, in one batch, which of ids the viewer may see and which a
// player could not, for ids not judged before. A page the viewer cannot open
// is refused for good.
func (w *familyWalk) judge(ids []string) error {
	var fresh []string
	seen := map[string]bool{}
	for _, id := range ids {
		if _, done := w.judged[id]; !done && !seen[id] {
			seen[id] = true
			fresh = append(fresh, id)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	viewable, err := w.s.viewer.FilterViewableEntityIDs(w.ctx, w.campaignID, fresh, w.role, w.userID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	var playerSees map[string]bool
	if w.dm {
		// What an ordinary player could open decides which relatives carry the
		// DM mark; a page limited to named players counts as hidden.
		playerSees, err = w.s.viewer.FilterViewableEntityIDs(w.ctx, w.campaignID, fresh, int(permissions.RolePlayer), "")
		if err != nil {
			return apperror.NewInternal(err)
		}
	}
	for _, id := range fresh {
		w.judged[id] = viewable[id]
		if viewable[id] && w.dm && !playerSees[id] {
			w.dmOnly[id] = true
		}
	}
	return nil
}

// admit adds a person unless the viewer may not see them or the people bound
// is spent; it reports whether the person is in the tree.
func (w *familyWalk) admit(id string) bool {
	if w.people[id] != nil {
		return true
	}
	if !w.judged[id] {
		return false
	}
	if len(w.order) >= MaxFamilyPeople {
		w.cutPpl = true
		return false
	}
	w.people[id] = &FamilyPerson{ID: id, DMOnly: w.dmOnly[id]}
	w.order = append(w.order, id)
	return true
}

// read returns a page's family links, once, within the read budget. ok is
// false once the budget is spent (and the people bound is then reported).
func (w *familyWalk) read(id string) (links []FamilyLink, ok bool, err error) {
	if l, hit := w.linkMem[id]; hit {
		return l, true, nil
	}
	if w.reads >= maxFamilyReads {
		w.cutPpl = true
		return nil, false, nil
	}
	w.reads++
	all, err := w.s.links.LinksOf(w.ctx, w.campaignID, id)
	if err != nil {
		return nil, false, apperror.NewInternal(err)
	}
	var fam []FamilyLink
	for _, l := range all {
		if l.OtherID == "" || l.OtherID == id {
			continue
		}
		if _, isFamily := classifyFamilyRelation(l.Type); !isFamily {
			continue
		}
		// A relation only the Director may see is not followed for anyone else.
		if l.DMOnly && !w.dm {
			continue
		}
		fam = append(fam, l)
	}
	w.linkMem[id] = fam
	return fam, true, nil
}

// addParent records that child has parent, once.
func (w *familyWalk) addParent(child, parent, kind string, dmOnly bool) {
	c := w.person(child)
	if c == nil {
		return
	}
	for _, p := range c.Parents {
		if p.ID == parent {
			return
		}
	}
	c.Parents = append(c.Parents, FamilyParent{ID: parent, Kind: kind, DMOnly: dmOnly})
}

func (w *familyWalk) addPartners(a, b string) {
	pa, pb := w.person(a), w.person(b)
	if pa == nil || pb == nil {
		return
	}
	if !containsString(pa.Partners, b) {
		pa.Partners = append(pa.Partners, b)
	}
	if !containsString(pb.Partners, a) {
		pb.Partners = append(pb.Partners, a)
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// expand reads every person of front and returns the next generation in the
// given direction (parents when up, children when down). Their partners and,
// for a read ancestor, their other children (the page's aunts and uncles) are
// admitted too, after the spine, so a nearer relative is never crowded out by
// a farther one. beyond is true when the family goes on past the bound: the
// persons found are then only counted, not admitted.
func (w *familyWalk) expand(front []string, up bool, beyond bool, seen map[string]bool) (next []string, err error) {
	type found struct {
		id, kind string
		dmOnly   bool
		from     string
	}
	var spine, side []found
	var partners [][2]string
	var ids []string
	for _, id := range front {
		links, ok, err := w.read(id)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if p := w.person(id); p != nil {
			p.Read = true
		}
		for _, l := range links {
			rel, _ := classifyFamilyRelation(l.Type)
			ids = append(ids, l.OtherID)
			switch {
			case rel.side == familyOtherIsPartner:
				partners = append(partners, [2]string{id, l.OtherID})
			case rel.side == familyOtherIsParent && up, rel.side == familyOtherIsChild && !up:
				spine = append(spine, found{l.OtherID, rel.kind, l.DMOnly, id})
			default:
				side = append(side, found{l.OtherID, rel.kind, l.DMOnly, id})
			}
		}
	}
	if err := w.judge(ids); err != nil {
		return nil, err
	}
	link := func(f found, up bool) {
		if up {
			w.addParent(f.from, f.id, f.kind, f.dmOnly)
		} else {
			w.addParent(f.id, f.from, f.kind, f.dmOnly)
		}
	}
	for _, f := range spine {
		if !w.judged[f.id] {
			continue
		}
		if beyond {
			if w.person(f.id) == nil {
				w.cutGen = true
			}
			continue
		}
		if !w.admit(f.id) {
			continue
		}
		link(f, up)
		// A page the other direction already admitted (a read ancestor's
		// other children, say) still has to be walked from in this one.
		if !seen[f.id] {
			seen[f.id] = true
			next = append(next, f.id)
		}
	}
	// Side links are the opposite direction of the spine: for a walk up they
	// are the children of a read ancestor (siblings, aunts, uncles); for a walk
	// down they are the parents of a read descendant (the other parent, who is
	// usually the partner as well).
	for _, f := range side {
		if !w.judged[f.id] || !w.admit(f.id) {
			continue
		}
		link(f, !up)
	}
	for _, p := range partners {
		if w.judged[p[1]] && w.admit(p[1]) {
			w.addPartners(p[0], p[1])
		}
	}
	return next, nil
}

// Tree implements FamilyTreeService.
func (s *familyTreeService) Tree(ctx context.Context, campaignID, entityID string, role int, userID string) (*FamilyTree, error) {
	w := &familyWalk{
		s: s, ctx: ctx, campaignID: campaignID, role: role, userID: userID,
		dm:      permissions.CanSeeDmOnly(role),
		people:  map[string]*FamilyPerson{},
		judged:  map[string]bool{},
		dmOnly:  map[string]bool{},
		linkMem: map[string][]FamilyLink{},
	}
	if err := w.judge([]string{entityID}); err != nil {
		return nil, err
	}
	if !w.judged[entityID] {
		return nil, apperror.NewNotFound("page not found")
	}
	w.admit(entityID)

	up, down := []string{entityID}, []string{entityID}
	upSeen, downSeen := map[string]bool{entityID: true}, map[string]bool{entityID: true}
	for gen := 1; gen <= MaxFamilyGenerations+1; gen++ {
		beyond := gen > MaxFamilyGenerations
		nu, err := w.expand(up, true, beyond, upSeen)
		if err != nil {
			return nil, err
		}
		nd, err := w.expand(down, false, beyond, downSeen)
		if err != nil {
			return nil, err
		}
		up, down = nu, nd
		if len(up) == 0 && len(down) == 0 {
			break
		}
	}

	// Name the people. A page that is gone or foreign is dropped, with every
	// link that pointed at it.
	cards, err := s.cards.InCampaign(ctx, campaignID, w.order)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	tree := &FamilyTree{PageID: entityID, CutGenerations: w.cutGen, CutPeople: w.cutPpl}
	keep := map[string]bool{}
	for _, id := range w.order {
		if _, ok := cards[id]; ok {
			keep[id] = true
		}
	}
	for _, id := range w.order {
		if !keep[id] {
			continue
		}
		p := *w.people[id]
		c := cards[id]
		p.Name, p.Type, p.Image = c.Name, c.TypeName, c.ImagePath
		p.Parents = keepParents(p.Parents, keep)
		p.Partners = keepIDs(p.Partners, keep)
		sort.SliceStable(p.Partners, func(i, j int) bool { return orderIndex(w.order, p.Partners[i]) < orderIndex(w.order, p.Partners[j]) })
		tree.People = append(tree.People, p)
	}
	if len(tree.People) == 0 || tree.People[0].ID != entityID {
		return nil, apperror.NewNotFound("page not found")
	}
	return tree, nil
}

func orderIndex(order []string, id string) int {
	for i, o := range order {
		if o == id {
			return i
		}
	}
	return len(order)
}

func keepParents(in []FamilyParent, keep map[string]bool) []FamilyParent {
	var out []FamilyParent
	for _, p := range in {
		if keep[p.ID] {
			out = append(out, p)
		}
	}
	return out
}

func keepIDs(in []string, keep map[string]bool) []string {
	var out []string
	for _, id := range in {
		if keep[id] {
			out = append(out, id)
		}
	}
	return out
}
