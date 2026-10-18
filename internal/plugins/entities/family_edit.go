package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// The Family tree block's Edit tab lets the Director add and remove a page's
// parents, children and partners in one sitting and save them together. The
// tree stays a view of the entity relations (family_tree.go); editing is only
// a friendlier way of writing the same relation rows the Relations block
// writes, so the two always agree. Nothing is stored on the page.
//
// Who may edit is decided here, in the service, on every call; the route
// repeats the check but is not the only gate. Writes go through FamilyWriter
// (an app adapter over the relations service, which writes and deletes both
// directions of a pair), never into the relation tables.

const (
	// MaxFamilyEdits bounds one save. A sitting adds a few relatives; a body of
	// hundreds is a mistake or an attack, and each edit is several writes.
	MaxFamilyEdits = 40
	// MaxFamilyNewPages bounds the pages one save may create.
	MaxFamilyNewPages = 10
	// FamilyPickerMax is how many pages the picker ever returns. The Director
	// narrows by typing; there is no unbounded list.
	FamilyPickerMax = 20
	// familyPickerQueryMax bounds the search text.
	familyPickerQueryMax = 100
	// maxCycleReads bounds the relation reads of one "is this page already an
	// ancestor" check.
	maxCycleReads = 120
	// familyNewPagePrefix marks a page that does not exist yet in an edit: it
	// is created first and the prefix is replaced by the new id.
	familyNewPagePrefix = "new:"
)

// Edge names and operations on the wire.
const (
	FamilyEdgeParent  = "parent"
	FamilyEdgePartner = "partner"
	FamilyOpAdd       = "add"
	FamilyOpRemove    = "remove"
)

// familyLinkTypes is the relation type pair (from the child's side, then from
// the parent's) a link kind is written as. They are the same strings the
// Relations block's suggestion list holds for "parent of" / "child of", and
// the family reader recognises all of them.
var familyLinkTypes = map[string][2]string{
	FamilyKindBirth:    {"child of", "parent of"},
	FamilyKindAdopted:  {"adopted child of", "adoptive parent of"},
	FamilyKindGuardian: {"ward of", "guardian of"},
}

// familyPartnerType is written both ways, so the pair reads the same from
// either page.
const familyPartnerType = "partner of"

// FamilyActor is who is editing. MemberRole is the real membership role, not
// the promoted visibility role, because editing is an authorization question.
type FamilyActor struct {
	UserID     string
	MemberRole int
	// DmGranted is the campaign owner's grant that makes a member a
	// co-Director.
	DmGranted bool
}

// IsDirector reports whether the actor is the campaign owner or a granted
// co-Director. Scribes and players are not.
func (a FamilyActor) IsDirector() bool {
	return permissions.CanSeeDmOnly(a.MemberRole, a.DmGranted)
}

// FamilyEdit is one change to the family: link or unlink a pair.
type FamilyEdit struct {
	Op   string `json:"op"`
	Edge string `json:"edge"`
	// A and B name the pair. For a parent edge A is the child and B the
	// parent; a partner edge is the same either way round. Either may be the
	// ref of a page created in the same save.
	A string `json:"a"`
	B string `json:"b"`
	// Link is the kind of a parent link being added (birth, adopted,
	// guardian). Ignored for other edits.
	Link string `json:"link,omitempty"`
}

// FamilyNewPage asks for a page to be created and used by its ref.
type FamilyNewPage struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
}

// FamilyEditRequest is one Save: pages to create, then edits to apply.
type FamilyEditRequest struct {
	NewPages []FamilyNewPage `json:"newPages"`
	Edits    []FamilyEdit    `json:"edits"`
}

// FamilyCreatedPage tells the caller what a ref became.
type FamilyCreatedPage struct {
	Ref  string `json:"ref"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FamilyEditFailure is an edit that did not save, by its index in the request.
type FamilyEditFailure struct {
	Index  int    `json:"index"`
	Reason string `json:"reason"`
	// HalfWritten is set when one direction of the pair was written and could
	// not be taken back, so the Relations block must finish the job. It is
	// never silent.
	HalfWritten bool `json:"halfWritten,omitempty"`
}

// FamilyEditResult is what a Save did. Saved means every edit is in place.
// Otherwise Failed says which edits did not save and Written which edits are
// still in place (all earlier ones are taken back, so Written is empty unless a
// takeback itself failed).
type FamilyEditResult struct {
	Saved      bool                `json:"saved"`
	Created    []FamilyCreatedPage `json:"created"`
	Failed     []FamilyEditFailure `json:"failed"`
	Written    []int               `json:"written"`
	RolledBack bool                `json:"rolledBack"`
	// Tree is the family as it now stands for the Director, so the page can
	// show the truth whatever happened.
	Tree *FamilyTree `json:"-"`
}

// FamilyCandidate is a page offered by the picker.
type FamilyCandidate struct {
	ID    string
	Name  string
	Type  string
	Image string
	// DMOnly is set when an ordinary player could not open the page.
	DMOnly bool
}

// FamilyCandidates is the picker's one bounded page of results.
type FamilyCandidates struct {
	Pages []FamilyCandidate
	// More is set when other pages match; the Director types to narrow.
	More bool
}

// FamilyNewLink is a pair to write, both directions.
type FamilyNewLink struct {
	Source, Target string
	Type, Reverse  string
	DMOnly         bool
	Metadata       json.RawMessage
	CreatedBy      string
}

// FamilyWriter writes family relations. Implemented in the app over the
// relations service, which writes and removes both rows of a pair; this plugin
// never reaches into the relation tables.
type FamilyWriter interface {
	FamilyLinks
	// Link writes the pair. When a write fails partway it takes back the row it
	// wrote; if it cannot, the error is a *FamilyHalfWritten.
	Link(ctx context.Context, campaignID string, l FamilyNewLink) error
	// Unlink removes every row of source->target whose type is in types, with
	// its reverse row, and any row of target->source whose type is in
	// reverseTypes that was left without one. It returns what it removed (the
	// forward rows) so a failed save can put them back.
	Unlink(ctx context.Context, campaignID, source, target string, types, reverseTypes []string) ([]FamilyNewLink, error)
}

// FamilyHalfWritten is returned when one direction of a pair is in place and
// the other could not be written or removed, and putting things right failed.
type FamilyHalfWritten struct{ Err error }

func (e *FamilyHalfWritten) Error() string {
	return fmt.Sprintf("family link left half-written: %v", e.Err)
}
func (e *FamilyHalfWritten) Unwrap() error { return e.Err }

// FamilyPages is the slice of the entity service the editor uses: the same
// listing and search the sidebar uses (so the picker shows what the Director
// can open) and the same create path a normal new page takes.
type FamilyPages interface {
	List(ctx context.Context, campaignID string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)
	Search(ctx context.Context, campaignID, query string, typeID int, role int, userID string, opts ListOptions) ([]Entity, int, error)
	Create(ctx context.Context, campaignID, userID string, input CreateEntityInput) (*Entity, error)
	GetEntityTypes(ctx context.Context, campaignID string) ([]EntityType, error)
}

// FamilyEditService is the Director's write side of the Family tree block.
type FamilyEditService interface {
	// Candidates returns a bounded, searchable list of pages the Director may
	// add as family of pageID.
	Candidates(ctx context.Context, campaignID, pageID string, actor FamilyActor, query string) (*FamilyCandidates, error)
	// Apply creates the requested pages, then applies the edits as one save.
	// newPagePrivate is the campaign's default for a new page (the value a
	// normal "new page" takes), resolved by the caller from the campaign.
	Apply(ctx context.Context, campaignID, pageID string, actor FamilyActor, newPagePrivate bool, req FamilyEditRequest) (*FamilyEditResult, error)
}

type familyEditService struct {
	writer FamilyWriter
	pages  FamilyPages
	viewer FamilyViewFilter
	tree   FamilyTreeService
}

// NewFamilyEditService creates the editor.
func NewFamilyEditService(writer FamilyWriter, pages FamilyPages, viewer FamilyViewFilter, tree FamilyTreeService) FamilyEditService {
	return &familyEditService{writer: writer, pages: pages, viewer: viewer, tree: tree}
}

var directorRole = int(permissions.RoleOwner)

// require gates every call on the actor being a Director and the page being
// one they can open in this campaign.
func (s *familyEditService) require(ctx context.Context, campaignID, pageID string, actor FamilyActor) error {
	if !actor.IsDirector() {
		return apperror.NewForbidden("only the Director can edit the family")
	}
	ok, err := s.viewer.FilterViewableEntityIDs(ctx, campaignID, []string{pageID}, directorRole, actor.UserID)
	if err != nil {
		return apperror.NewInternal(err)
	}
	if !ok[pageID] {
		return apperror.NewNotFound("page not found")
	}
	return nil
}

// familyKinTypes returns every relation type that reads as the given side from
// the reading page: parent edge from the child's side is what the child holds.
func familyTypesOfSide(side familySide) []string {
	var out []string
	for t, r := range familyRelations {
		if r.side == side {
			out = append(out, t)
		}
	}
	return out
}

// Candidates implements FamilyEditService.
func (s *familyEditService) Candidates(ctx context.Context, campaignID, pageID string, actor FamilyActor, query string) (*FamilyCandidates, error) {
	if err := s.require(ctx, campaignID, pageID, actor); err != nil {
		return nil, err
	}
	q := strings.TrimSpace(query)
	if utf8.RuneCountInString(q) > familyPickerQueryMax {
		return nil, apperror.NewBadRequest("search text is too long")
	}
	out := &FamilyCandidates{Pages: []FamilyCandidate{}}
	if utf8.RuneCountInString(q) == 1 {
		// One letter is not a search (the same floor the sidebar search has).
		return out, nil
	}

	// A page already family of this one is not offered again; asking for that
	// many extra rows keeps the list full after they are dropped.
	skip := map[string]bool{pageID: true}
	links, err := s.writer.LinksOf(ctx, campaignID, pageID)
	if err != nil {
		return nil, apperror.NewInternal(err)
	}
	for _, l := range links {
		if _, fam := classifyFamilyRelation(l.Type); fam {
			skip[l.OtherID] = true
		}
	}
	opts := DefaultListOptions()
	opts.PerPage = min(100, FamilyPickerMax+len(skip))

	var found []Entity
	var total int
	if q == "" {
		found, total, err = s.pages.List(ctx, campaignID, 0, directorRole, actor.UserID, opts)
	} else {
		found, total, err = s.pages.Search(ctx, campaignID, q, 0, directorRole, actor.UserID, opts)
	}
	if err != nil {
		return nil, err
	}
	var kept []Entity
	for _, e := range found {
		if !skip[e.ID] {
			kept = append(kept, e)
		}
	}
	out.More = len(kept) > FamilyPickerMax || total > len(found)
	if len(kept) > FamilyPickerMax {
		kept = kept[:FamilyPickerMax]
	}
	ids := make([]string, 0, len(kept))
	for _, e := range kept {
		ids = append(ids, e.ID)
	}
	var seen map[string]bool
	if len(ids) > 0 {
		seen, err = s.viewer.FilterViewableEntityIDs(ctx, campaignID, ids, int(permissions.RolePlayer), "")
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
	}
	for _, e := range kept {
		c := FamilyCandidate{ID: e.ID, Name: e.Name, Type: e.TypeName, DMOnly: !seen[e.ID]}
		if e.ImagePath != nil {
			c.Image = *e.ImagePath
		}
		out.Pages = append(out.Pages, c)
	}
	return out, nil
}

// validate checks a request without writing anything, and returns the real
// page ids it names (not refs).
func (s *familyEditService) validate(req *FamilyEditRequest) (realIDs []string, err error) {
	if len(req.Edits) == 0 && len(req.NewPages) == 0 {
		return nil, apperror.NewBadRequest("nothing to save")
	}
	if len(req.Edits) > MaxFamilyEdits {
		return nil, apperror.NewBadRequest(fmt.Sprintf("at most %d changes can be saved at once", MaxFamilyEdits))
	}
	if len(req.NewPages) > MaxFamilyNewPages {
		return nil, apperror.NewBadRequest(fmt.Sprintf("at most %d new pages can be created at once", MaxFamilyNewPages))
	}
	refs := map[string]bool{}
	for i := range req.NewPages {
		p := &req.NewPages[i]
		p.Name = strings.TrimSpace(p.Name)
		if !strings.HasPrefix(p.Ref, familyNewPagePrefix) || len(p.Ref) > 40 || refs[p.Ref] {
			return nil, apperror.NewBadRequest("a new page has an invalid reference")
		}
		if p.Name == "" {
			return nil, apperror.NewBadRequest("a new page needs a name")
		}
		if utf8.RuneCountInString(p.Name) > apperror.MaxNameLength {
			return nil, apperror.NewBadRequest("a new page name is too long")
		}
		refs[p.Ref] = true
	}
	seenID := map[string]bool{}
	note := func(id string) error {
		if id == "" {
			return apperror.NewBadRequest("a change is missing a page")
		}
		if strings.HasPrefix(id, familyNewPagePrefix) {
			if !refs[id] {
				return apperror.NewBadRequest("a change names a page that is not being created")
			}
			return nil
		}
		if !seenID[id] {
			seenID[id] = true
			realIDs = append(realIDs, id)
		}
		return nil
	}
	for i := range req.Edits {
		e := &req.Edits[i]
		if e.Op != FamilyOpAdd && e.Op != FamilyOpRemove {
			return nil, apperror.NewBadRequest("a change has an unknown operation")
		}
		if e.Edge != FamilyEdgeParent && e.Edge != FamilyEdgePartner {
			return nil, apperror.NewBadRequest("a change has an unknown kind of link")
		}
		if e.A == e.B {
			return nil, apperror.NewBadRequest("a page cannot be its own relative")
		}
		if e.Op == FamilyOpAdd && e.Edge == FamilyEdgeParent {
			if e.Link == "" {
				e.Link = FamilyKindBirth
			}
			if _, ok := familyLinkTypes[e.Link]; !ok {
				return nil, apperror.NewBadRequest("a change has an unknown kind of parent link")
			}
		}
		if err := note(e.A); err != nil {
			return nil, err
		}
		if err := note(e.B); err != nil {
			return nil, err
		}
	}
	return realIDs, nil
}

// pageType picks the category a family-tree page is created in: the campaign's
// character category when it has one, else the first enabled top-level one, the
// way a quick-created page falls back.
func pageType(types []EntityType) (int, bool) {
	var first *EntityType
	for i := range types {
		t := &types[i]
		if !t.Enabled || t.ParentTypeID != nil {
			continue
		}
		if t.PresetCategory != nil && *t.PresetCategory == "character" || t.Slug == "character" {
			return t.ID, true
		}
		if first == nil {
			first = t
		}
	}
	if first == nil {
		return 0, false
	}
	return first.ID, true
}

// applied is a write that can be taken back.
type applied struct {
	index   int
	linked  *FamilyNewLink // an added pair: undo by unlinking
	unlinks []FamilyNewLink
}

// Apply implements FamilyEditService.
func (s *familyEditService) Apply(ctx context.Context, campaignID, pageID string, actor FamilyActor, newPagePrivate bool, req FamilyEditRequest) (*FamilyEditResult, error) {
	if err := s.require(ctx, campaignID, pageID, actor); err != nil {
		return nil, err
	}
	realIDs, err := s.validate(&req)
	if err != nil {
		return nil, err
	}
	if len(realIDs) > 0 {
		ok, err := s.viewer.FilterViewableEntityIDs(ctx, campaignID, realIDs, directorRole, actor.UserID)
		if err != nil {
			return nil, apperror.NewInternal(err)
		}
		for _, id := range realIDs {
			if !ok[id] {
				return nil, apperror.NewBadRequest("a page in this change is not available")
			}
		}
	}

	res := &FamilyEditResult{Created: []FamilyCreatedPage{}, Failed: []FamilyEditFailure{}, Written: []int{}}
	finish := func() (*FamilyEditResult, error) {
		tree, err := s.tree.Tree(ctx, campaignID, pageID, directorRole, actor.UserID)
		if err != nil {
			return nil, err
		}
		res.Tree = tree
		return res, nil
	}

	// New pages first, through the same create path (and the campaign's default
	// visibility) a normal new page takes. A page is never deleted afterwards:
	// if the links fail, the page stays and the caller reuses it on the retry.
	refTo := map[string]string{}
	if len(req.NewPages) > 0 {
		types, err := s.pages.GetEntityTypes(ctx, campaignID)
		if err != nil {
			return nil, err
		}
		typeID, ok := pageType(types)
		if !ok {
			return nil, apperror.NewBadRequest("this campaign has no page category to create a page in")
		}
		for i, p := range req.NewPages {
			e, err := s.pages.Create(ctx, campaignID, actor.UserID, CreateEntityInput{Name: p.Name, EntityTypeID: typeID, IsPrivate: newPagePrivate})
			if err != nil {
				slog.Error("family edit: cannot create a page", slog.String("campaign_id", campaignID), slog.Any("error", err))
				res.Failed = append(res.Failed, FamilyEditFailure{Index: -1 - i, Reason: "the page could not be created"})
				return finish()
			}
			refTo[p.Ref] = e.ID
			res.Created = append(res.Created, FamilyCreatedPage{Ref: p.Ref, ID: e.ID, Name: e.Name})
		}
	}
	resolve := func(id string) string {
		if r, ok := refTo[id]; ok {
			return r
		}
		return id
	}

	// Removals first so a link replaced by another kind (born to -> adopted)
	// is taken off before the new one goes on.
	var order []int
	for i, e := range req.Edits {
		if e.Op == FamilyOpRemove {
			order = append(order, i)
		}
	}
	for i, e := range req.Edits {
		if e.Op == FamilyOpAdd {
			order = append(order, i)
		}
	}

	var done []applied
	var failed *FamilyEditFailure
	for _, i := range order {
		e := req.Edits[i]
		a, b := resolve(e.A), resolve(e.B)
		var step applied
		var err error
		if e.Op == FamilyOpRemove {
			step, err = s.unlink(ctx, campaignID, e.Edge, a, b)
		} else {
			step, err = s.link(ctx, campaignID, actor, e, a, b)
		}
		step.index = i
		if step.linked != nil || len(step.unlinks) > 0 {
			done = append(done, step)
		}
		if err != nil {
			var half *FamilyHalfWritten
			reason := "the link could not be written"
			if e.Op == FamilyOpRemove {
				reason = "the link could not be removed"
			}
			if ae, ok := err.(*apperror.AppError); ok && ae.Code == 400 {
				reason = ae.Message
			}
			f := FamilyEditFailure{Index: i, Reason: reason}
			if asHalf(err, &half) {
				f.HalfWritten = true
			}
			failed = &f
			slog.Error("family edit: a write failed", slog.String("campaign_id", campaignID), slog.Int("edit", i), slog.Any("error", err))
			break
		}
	}
	if failed == nil {
		res.Saved = true
		return finish()
	}
	res.Failed = append(res.Failed, *failed)
	if failed.HalfWritten {
		res.Written = append(res.Written, failed.Index)
	}

	// Take back what this save already wrote, newest first, so the Director
	// sees all of it saved or none of it. A takeback that fails is reported.
	res.RolledBack = true
	for k := len(done) - 1; k >= 0; k-- {
		d := done[k]
		if err := s.undo(ctx, campaignID, d); err != nil {
			slog.Error("family edit: cannot take back a write", slog.String("campaign_id", campaignID), slog.Int("edit", d.index), slog.Any("error", err))
			res.RolledBack = false
			if d.index != failed.Index {
				res.Written = append(res.Written, d.index)
			}
		}
	}
	return finish()
}

func asHalf(err error, target **FamilyHalfWritten) bool {
	for err != nil {
		if h, ok := err.(*FamilyHalfWritten); ok {
			*target = h
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// edgeTypes returns the relation types an edge between a and b is held as:
// from a's side and from b's side.
func edgeTypes(edge string) (fromA, fromB []string) {
	if edge == FamilyEdgePartner {
		t := familyTypesOfSide(familyOtherIsPartner)
		return t, t
	}
	// a is the child: it holds "child of"; b holds "parent of".
	return familyTypesOfSide(familyOtherIsParent), familyTypesOfSide(familyOtherIsChild)
}

func (s *familyEditService) unlink(ctx context.Context, campaignID, edge, a, b string) (applied, error) {
	fromA, fromB := edgeTypes(edge)
	removed, err := s.writer.Unlink(ctx, campaignID, a, b, fromA, fromB)
	return applied{unlinks: removed}, err
}

func (s *familyEditService) link(ctx context.Context, campaignID string, actor FamilyActor, e FamilyEdit, a, b string) (applied, error) {
	// Already linked (the Relations block got there first): nothing to do.
	links, err := s.writer.LinksOf(ctx, campaignID, a)
	if err != nil {
		return applied{}, err
	}
	fromA, _ := edgeTypes(e.Edge)
	for _, l := range links {
		if l.OtherID != b {
			continue
		}
		for _, t := range fromA {
			if strings.EqualFold(strings.TrimSpace(l.Type), t) {
				return applied{}, nil
			}
		}
	}
	nl := FamilyNewLink{Source: a, Target: b, CreatedBy: actor.UserID}
	if e.Edge == FamilyEdgePartner {
		nl.Type, nl.Reverse = familyPartnerType, familyPartnerType
	} else {
		pair := familyLinkTypes[e.Link]
		nl.Type, nl.Reverse = pair[0], pair[1]
		// A page cannot become its own ancestor: reject a parent that is
		// already a descendant of the child.
		cyc, err := s.isDescendant(ctx, campaignID, b, a)
		if err != nil {
			return applied{}, err
		}
		if cyc {
			return applied{}, apperror.NewBadRequest("that would make a page its own ancestor")
		}
	}
	if err := s.writer.Link(ctx, campaignID, nl); err != nil {
		return applied{}, err
	}
	return applied{linked: &nl}, nil
}

// isDescendant reports whether page is a descendant of ancestor by walking
// page's parents upward, bounded by reads.
func (s *familyEditService) isDescendant(ctx context.Context, campaignID, page, ancestor string) (bool, error) {
	seen := map[string]bool{page: true}
	front := []string{page}
	for reads := 0; len(front) > 0; {
		var next []string
		for _, id := range front {
			if reads >= maxCycleReads {
				// Past the bound the check is not trusted; refuse rather than
				// risk a loop the tree reader would have to walk forever.
				return true, nil
			}
			reads++
			links, err := s.writer.LinksOf(ctx, campaignID, id)
			if err != nil {
				return false, err
			}
			for _, l := range links {
				r, fam := classifyFamilyRelation(l.Type)
				if !fam || r.side != familyOtherIsParent {
					continue
				}
				if l.OtherID == ancestor {
					return true, nil
				}
				if !seen[l.OtherID] {
					seen[l.OtherID] = true
					next = append(next, l.OtherID)
				}
			}
		}
		front = next
	}
	return false, nil
}

// undo takes one applied write back.
func (s *familyEditService) undo(ctx context.Context, campaignID string, d applied) error {
	if d.linked != nil {
		l := d.linked
		_, err := s.writer.Unlink(ctx, campaignID, l.Source, l.Target, []string{l.Type}, []string{l.Reverse})
		return err
	}
	for _, l := range d.unlinks {
		if err := s.writer.Link(ctx, campaignID, l); err != nil {
			return err
		}
	}
	return nil
}
