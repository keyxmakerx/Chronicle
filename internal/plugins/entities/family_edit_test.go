package entities

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// editFam is a campaign held in memory with the writer the app builds over the
// relations service: Link and Unlink touch both rows of a pair. Failures can be
// injected by the pair a write names.
type editFam struct {
	*famFixture
	dm map[string]bool // a link's rows by "a>b" key that are Director-only

	ops        []string        // every write, in order
	failLink   map[string]bool // "source>target": Link fails
	halfLink   map[string]bool // "source>target": Link fails and leaves a half-written pair
	failUnlink map[string]bool // "source>target": Unlink fails
	failRedo   bool            // the removed maud>dad link cannot be put back
	found      []Entity        // what the entity listing holds
	types      []EntityType
	made       []CreateEntityInput
	failCreate int // Create fails from this call on (1-based); 0 = never
	listOpts   []ListOptions
	listRole   []int
}

func newEditFam() *editFam {
	e := &editFam{famFixture: houseFixture(), dm: map[string]bool{}, failLink: map[string]bool{}, halfLink: map[string]bool{}, failUnlink: map[string]bool{}}
	character := "character"
	e.types = []EntityType{
		{ID: 1, Slug: "location", Enabled: true},
		{ID: 2, Slug: "npc", PresetCategory: &character, Enabled: true},
		{ID: 3, Slug: "item", Enabled: false},
	}
	return e
}

func (e *editFam) service() FamilyEditService {
	return NewFamilyEditService(e, e, e.famFixture, e.famFixture.service())
}

func (e *editFam) add(source string, typ string, target string, dmOnly bool) {
	e.links[source] = append(e.links[source], FamilyLink{OtherID: target, Type: typ, DMOnly: dmOnly})
}

func (e *editFam) Link(_ context.Context, _ string, l FamilyNewLink) error {
	key := l.Source + ">" + l.Target
	e.ops = append(e.ops, "link "+key+" "+l.Type)
	switch {
	case e.halfLink[key]:
		e.add(l.Source, l.Type, l.Target, l.DMOnly)
		return &FamilyHalfWritten{Err: errors.New("reverse failed")}
	case e.failLink[key]:
		return errors.New("db down")
	case e.failRedo && key == "maud>dad":
		// Only the put-back of the removed link fails.
		return errors.New("db down")
	}
	e.add(l.Source, l.Type, l.Target, l.DMOnly)
	e.add(l.Target, l.Reverse, l.Source, l.DMOnly)
	return nil
}

func (e *editFam) Unlink(_ context.Context, _, source, target string, types, reverseTypes []string) ([]FamilyNewLink, error) {
	key := source + ">" + target
	e.ops = append(e.ops, "unlink "+key)
	if e.failUnlink[key] {
		return nil, errors.New("db down")
	}
	var removed []FamilyNewLink
	keep := e.links[source][:0:0]
	for _, l := range e.links[source] {
		if l.OtherID == target && relationTypeMatch(l.Type, types) {
			removed = append(removed, FamilyNewLink{Source: source, Target: target, Type: l.Type, Reverse: reverseOf(l.Type), DMOnly: l.DMOnly})
			continue
		}
		keep = append(keep, l)
	}
	e.links[source] = keep
	rkeep := e.links[target][:0:0]
	for _, l := range e.links[target] {
		if l.OtherID == source && relationTypeMatch(l.Type, reverseTypes) {
			continue
		}
		rkeep = append(rkeep, l)
	}
	e.links[target] = rkeep
	return removed, nil
}

func relationTypeMatch(t string, list []string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(t), x) {
			return true
		}
	}
	return false
}

func reverseOf(t string) string {
	if r, ok := famReverse[strings.ToLower(strings.TrimSpace(t))]; ok {
		return r
	}
	return t
}

func (e *editFam) List(_ context.Context, _ string, _ int, role int, _ string, opts ListOptions) ([]Entity, int, error) {
	e.listOpts = append(e.listOpts, opts)
	e.listRole = append(e.listRole, role)
	return e.page(e.found, opts), len(e.found), nil
}

func (e *editFam) Search(_ context.Context, _, query string, _ int, role int, _ string, opts ListOptions) ([]Entity, int, error) {
	e.listOpts = append(e.listOpts, opts)
	e.listRole = append(e.listRole, role)
	var hit []Entity
	for _, p := range e.found {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(query)) {
			hit = append(hit, p)
		}
	}
	return e.page(hit, opts), len(hit), nil
}

func (e *editFam) page(all []Entity, opts ListOptions) []Entity {
	if len(all) > opts.PerPage {
		return all[:opts.PerPage]
	}
	return all
}

func (e *editFam) GetEntityTypes(context.Context, string) ([]EntityType, error) { return e.types, nil }

func (e *editFam) Create(_ context.Context, _, _ string, in CreateEntityInput) (*Entity, error) {
	if e.failCreate > 0 && len(e.made)+1 >= e.failCreate {
		e.made = append(e.made, in)
		return nil, errors.New("db down")
	}
	e.made = append(e.made, in)
	id := fmt.Sprintf("made%d", len(e.made))
	e.names[id] = in.Name
	return &Entity{ID: id, Name: in.Name}, nil
}

// has reports whether page a holds a relation of the given type to b.
func (e *editFam) has(a, typ, b string) bool {
	for _, l := range e.links[a] {
		if l.OtherID == b && strings.EqualFold(strings.TrimSpace(l.Type), typ) {
			return true
		}
	}
	return false
}

// markDM makes every row between a and b Director-only, in both directions.
func (e *editFam) markDM(a, b string) {
	for i := range e.links[a] {
		if e.links[a][i].OtherID == b {
			e.links[a][i].DMOnly = true
		}
	}
	for i := range e.links[b] {
		if e.links[b][i].OtherID == a {
			e.links[b][i].DMOnly = true
		}
	}
}

func (e *editFam) pair(a, typ, b string) bool { return e.has(a, typ, b) && e.has(b, reverseOf(typ), a) }

func (e *editFam) snapshot() string {
	var rows []string
	for a, ls := range e.links {
		for _, l := range ls {
			rows = append(rows, fmt.Sprintf("%s|%s|%s|%t", a, strings.ToLower(strings.TrimSpace(l.Type)), l.OtherID, l.DMOnly))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

var (
	owner      = FamilyActor{UserID: "owner", MemberRole: int(permissions.RoleOwner)}
	coDirector = FamilyActor{UserID: "co", MemberRole: int(permissions.RolePlayer), DmGranted: true}
	scribe     = FamilyActor{UserID: "scribe", MemberRole: int(permissions.RoleScribe)}
	player     = FamilyActor{UserID: "player", MemberRole: int(permissions.RolePlayer)}
)

func parentEdit(op, child, parent, link string) FamilyEdit {
	return FamilyEdit{Op: op, Edge: FamilyEdgeParent, A: child, B: parent, Link: link}
}
func partnerEdit(op, a, b string) FamilyEdit {
	return FamilyEdit{Op: op, Edge: FamilyEdgePartner, A: a, B: b}
}

// Only the Director can edit, and every entry point says so itself: the route
// gate is a second lock, not the only one. A refused call changes nothing and
// creates nothing.
func TestFamilyEditIsDirectorOnly(t *testing.T) {
	tests := []struct {
		name  string
		actor FamilyActor
		ok    bool
	}{
		{"owner", owner, true},
		{"granted co-Director (a player with the grant)", coDirector, true},
		{"scribe", scribe, false},
		{"scribe with no grant, even asking for the tree's own page", FamilyActor{UserID: "s2", MemberRole: int(permissions.RoleScribe)}, false},
		{"player", player, false},
		{"not a member", FamilyActor{UserID: "x", MemberRole: int(permissions.RoleNone)}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			f.found = []Entity{{ID: "bob", Name: "Bob"}}
			before := f.snapshot()
			svc := f.service()

			_, err := svc.Apply(context.Background(), "c1", "maud", tt.actor, false, FamilyEditRequest{
				NewPages: []FamilyNewPage{{Ref: "new:1", Name: "Odo"}},
				Edits:    []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "new:1", "birth")},
			})
			_, cerr := svc.Candidates(context.Background(), "c1", "maud", tt.actor, "")
			if tt.ok {
				if err != nil || cerr != nil {
					t.Fatalf("a Director must be allowed, got apply=%v candidates=%v", err, cerr)
				}
				return
			}
			assertAppError(t, err, http.StatusForbidden)
			assertAppError(t, cerr, http.StatusForbidden)
			if len(f.ops) != 0 || len(f.made) != 0 || len(f.listOpts) != 0 || f.snapshot() != before {
				t.Errorf("a refused call must change nothing: ops=%v made=%v lists=%d", f.ops, f.made, len(f.listOpts))
			}
		})
	}
}

// Every edit writes both directions as the Relations block would, and reads
// back through the tree reader, so the two blocks agree.
func TestFamilyEditWritesReciprocalPairs(t *testing.T) {
	tests := []struct {
		name  string
		edit  FamilyEdit
		pairs [][3]string // a, type, b: must exist with its reverse on b
		tree  func(*FamilyTree) bool
	}{
		{"birth parent", parentEdit(FamilyOpAdd, "tam", "dad", "birth"), [][3]string{{"tam", "child of", "dad"}}, func(tr *FamilyTree) bool { return hasParent(tr.person("tam"), "dad", FamilyKindBirth) }},
		{"adoptive parent", parentEdit(FamilyOpAdd, "tam", "dad", "adopted"), [][3]string{{"tam", "adopted child of", "dad"}}, func(tr *FamilyTree) bool { return hasParent(tr.person("tam"), "dad", FamilyKindAdopted) }},
		{"guardian", parentEdit(FamilyOpAdd, "tam", "dad", "guardian"), [][3]string{{"tam", "ward of", "dad"}}, func(tr *FamilyTree) bool { return hasParent(tr.person("tam"), "dad", FamilyKindGuardian) }},
		{"a link kind left empty is by birth", parentEdit(FamilyOpAdd, "tam", "dad", ""), [][3]string{{"tam", "child of", "dad"}}, nil},
		{"partner", partnerEdit(FamilyOpAdd, "maud", "bob"), [][3]string{{"maud", "partner of", "bob"}}, func(tr *FamilyTree) bool { return tr.person("bob") != nil && len(tr.person("maud").Partners) == 1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{Edits: []FamilyEdit{tt.edit}})
			if err != nil || !res.Saved {
				t.Fatalf("saved=%v err=%v", res != nil && res.Saved, err)
			}
			for _, p := range tt.pairs {
				if !f.pair(p[0], p[1], p[2]) {
					t.Errorf("%s %q %s must be written with its reverse on the other page; links=%v", p[0], p[1], p[2], f.links)
				}
			}
			if tt.tree != nil && !tt.tree(res.Tree) {
				t.Errorf("the tree read back after saving does not show the change: %v", res.Tree.ids())
			}
		})
	}
}

func TestFamilyEditRemovesBothDirections(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*editFam)
		edit   FamilyEdit
		gone   [][3]string
		kept   [][3]string
		nOther int // rows of the other kinds that must stay
	}{
		{"a birth parent", nil, parentEdit(FamilyOpRemove, "maud", "dad", ""), [][3]string{{"maud", "child of", "dad"}, {"dad", "parent of", "maud"}}, [][3]string{{"maud", "child of", "mum"}}, 0},
		{"a child, named from the other side", nil, parentEdit(FamilyOpRemove, "tam", "maud", ""), [][3]string{{"maud", "parent of", "tam"}, {"tam", "child of", "maud"}}, nil, 0},
		{"an adoptive link", nil, parentEdit(FamilyOpRemove, "perrin", "maud", ""), [][3]string{{"maud", "adoptive parent of", "perrin"}, {"perrin", "adopted child of", "maud"}}, nil, 0},
		{"a guardian link", nil, parentEdit(FamilyOpRemove, "kessa", "maud", ""), [][3]string{{"maud", "guardian of", "kessa"}}, nil, 0},
		{"a partner", nil, partnerEdit(FamilyOpRemove, "mum", "dad"), [][3]string{{"mum", "married to", "dad"}, {"dad", "married to", "mum"}}, [][3]string{{"maud", "child of", "mum"}}, 0},
		{
			"a reverse row left alone by an earlier half-written pair goes too, so the other page stops showing it",
			func(f *editFam) { f.add("bob", "child of", "tam", false) },
			parentEdit(FamilyOpRemove, "bob", "tam", ""),
			[][3]string{{"bob", "child of", "tam"}}, nil, 0,
		},
		{"a link already gone is no error", nil, parentEdit(FamilyOpRemove, "tam", "dad", ""), nil, nil, 0},
		{"a relation that is not family stays", nil, partnerEdit(FamilyOpRemove, "maud", "bob"), nil, [][3]string{{"maud", "knows", "bob"}}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			if tt.setup != nil {
				tt.setup(f)
			}
			res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{Edits: []FamilyEdit{tt.edit}})
			if err != nil || !res.Saved {
				t.Fatalf("saved=%v err=%v", res != nil && res.Saved, err)
			}
			for _, g := range tt.gone {
				if f.has(g[0], g[1], g[2]) {
					t.Errorf("%s %q %s should be gone", g[0], g[1], g[2])
				}
			}
			for _, k := range tt.kept {
				if !f.has(k[0], k[1], k[2]) {
					t.Errorf("%s %q %s should stay", k[0], k[1], k[2])
				}
			}
		})
	}
}

// A link replaced by another kind is taken off before the new one goes on, and
// an add for a link that is already there writes nothing.
func TestFamilyEditOrderAndRepeats(t *testing.T) {
	f := newEditFam()
	res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{Edits: []FamilyEdit{
		parentEdit(FamilyOpAdd, "maud", "dad", "adopted"),
		parentEdit(FamilyOpRemove, "maud", "dad", ""),
		parentEdit(FamilyOpAdd, "maud", "mum", "birth"), // already linked
	}})
	if err != nil || !res.Saved {
		t.Fatalf("saved=%v err=%v", res != nil && res.Saved, err)
	}
	want := []string{"unlink maud>dad", "link maud>dad adopted child of"}
	if strings.Join(f.ops, ",") != strings.Join(want, ",") {
		t.Errorf("ops = %v, want %v (remove first, repeat skipped)", f.ops, want)
	}
	if !f.pair("maud", "adopted child of", "dad") || f.has("maud", "child of", "dad") {
		t.Errorf("the link should now be the adoptive one: %v", f.links["maud"])
	}
}

// Nothing is written for a request that is wrong on its face.
func TestFamilyEditValidatesBeforeWriting(t *testing.T) {
	many := func(n int) []FamilyEdit {
		out := make([]FamilyEdit, n)
		for i := range out {
			out[i] = partnerEdit(FamilyOpAdd, "maud", "bob")
		}
		return out
	}
	pages := func(n int) []FamilyNewPage {
		out := make([]FamilyNewPage, n)
		for i := range out {
			out[i] = FamilyNewPage{Ref: fmt.Sprintf("new:%d", i), Name: "P"}
		}
		return out
	}
	tests := []struct {
		name string
		req  FamilyEditRequest
	}{
		{"empty", FamilyEditRequest{}},
		{"a page related to itself", FamilyEditRequest{Edits: []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "maud", "birth")}}},
		{"unknown operation", FamilyEditRequest{Edits: []FamilyEdit{{Op: "toggle", Edge: FamilyEdgePartner, A: "maud", B: "bob"}}}},
		{"unknown edge", FamilyEditRequest{Edits: []FamilyEdit{{Op: FamilyOpAdd, Edge: "sibling", A: "maud", B: "bob"}}}},
		{"unknown link kind", FamilyEditRequest{Edits: []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "bob", "sworn")}}},
		{"a missing page", FamilyEditRequest{Edits: []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "", "birth")}}},
		{"a page that is not in the campaign", FamilyEditRequest{Edits: []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "elsewhere", "birth")}}},
		{"more edits than one save may carry", FamilyEditRequest{Edits: many(MaxFamilyEdits + 1)}},
		{"more new pages than one save may create", FamilyEditRequest{NewPages: pages(MaxFamilyNewPages + 1), Edits: many(1)}},
		{"a ref nobody defined", FamilyEditRequest{Edits: []FamilyEdit{partnerEdit(FamilyOpAdd, "maud", "new:9")}}},
		{"a ref without the prefix", FamilyEditRequest{NewPages: []FamilyNewPage{{Ref: "bob", Name: "Odo"}}, Edits: many(1)}},
		{"the same ref twice", FamilyEditRequest{NewPages: []FamilyNewPage{{Ref: "new:1", Name: "A"}, {Ref: "new:1", Name: "B"}}, Edits: many(1)}},
		{"a nameless page", FamilyEditRequest{NewPages: []FamilyNewPage{{Ref: "new:1", Name: "   "}}, Edits: []FamilyEdit{partnerEdit(FamilyOpAdd, "maud", "new:1")}}},
		{"a name too long", FamilyEditRequest{NewPages: []FamilyNewPage{{Ref: "new:1", Name: strings.Repeat("x", 201)}}, Edits: []FamilyEdit{partnerEdit(FamilyOpAdd, "maud", "new:1")}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			before := f.snapshot()
			_, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, tt.req)
			assertAppError(t, err, http.StatusBadRequest)
			if len(f.ops) != 0 || len(f.made) != 0 || f.snapshot() != before {
				t.Errorf("a bad request must write and create nothing: ops=%v made=%v", f.ops, f.made)
			}
		})
	}

	t.Run("the page being edited must be one the Director can open here", func(t *testing.T) {
		f := newEditFam()
		_, err := f.service().Apply(context.Background(), "c1", "ghost", owner, false, FamilyEditRequest{Edits: many(1)})
		assertAppError(t, err, http.StatusNotFound)
	})
}

// "Create a new page" goes through the entity create path with the campaign's
// default visibility, in the family's usual category.
func TestFamilyEditCreatesPagesLikeANormalNewPage(t *testing.T) {
	tests := []struct {
		name    string
		private bool
		types   []EntityType
		wantTyp int
		wantErr int
	}{
		{"the character category when there is one, public by default", false, nil, 2, 0},
		{"private when the campaign defaults to private", true, nil, 2, 0},
		{"the first enabled top-level category otherwise", false, []EntityType{{ID: 7, Slug: "item", Enabled: false}, {ID: 8, Slug: "place", Enabled: true}, {ID: 9, Slug: "npc", Enabled: true}}, 8, 0},
		{"no usable category is a clear error and no page", false, []EntityType{{ID: 7, Enabled: false}}, 0, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			if tt.types != nil {
				f.types = tt.types
			}
			req := FamilyEditRequest{
				NewPages: []FamilyNewPage{{Ref: "new:1", Name: "  Odo  "}},
				Edits:    []FamilyEdit{parentEdit(FamilyOpAdd, "maud", "new:1", "guardian"), partnerEdit(FamilyOpAdd, "new:1", "bob")},
			}
			res, err := f.service().Apply(context.Background(), "c1", "maud", owner, tt.private, req)
			if tt.wantErr != 0 {
				assertAppError(t, err, tt.wantErr)
				if len(f.made) != 0 || len(f.ops) != 0 {
					t.Errorf("nothing may be written: made=%v ops=%v", f.made, f.ops)
				}
				return
			}
			if err != nil || !res.Saved {
				t.Fatalf("saved=%v err=%v", res != nil && res.Saved, err)
			}
			if len(f.made) != 1 || f.made[0].Name != "Odo" || f.made[0].EntityTypeID != tt.wantTyp || f.made[0].IsPrivate != tt.private {
				t.Errorf("created %+v, want one page Odo in type %d private=%v", f.made, tt.wantTyp, tt.private)
			}
			if len(res.Created) != 1 || res.Created[0].Ref != "new:1" || res.Created[0].ID != "made1" {
				t.Errorf("the caller must learn what the ref became, got %+v", res.Created)
			}
			if !f.pair("maud", "ward of", "made1") || !f.pair("made1", "partner of", "bob") {
				t.Errorf("links must name the real id, not the ref: %v", f.links)
			}
		})
	}
}

// A page that would become its own ancestor is refused, and the rest of the
// save is taken back with it.
func TestFamilyEditRefusesALoop(t *testing.T) {
	f := newEditFam()
	before := f.snapshot()
	res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{Edits: []FamilyEdit{
		partnerEdit(FamilyOpAdd, "maud", "bob"),
		parentEdit(FamilyOpAdd, "gran", "tam", "birth"), // tam is gran's descendant: tam > maud > mum > gran
	}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved || !res.RolledBack || len(res.Failed) != 1 || res.Failed[0].Index != 1 || !strings.Contains(res.Failed[0].Reason, "ancestor") {
		t.Errorf("result = %+v", res)
	}
	if f.snapshot() != before {
		t.Error("the earlier link of the same save must be taken back")
	}
}

// A write that fails partway reports which link did not save and leaves the
// campaign as it was; one that cannot be taken back is named, never silent.
func TestFamilyEditPartialFailure(t *testing.T) {
	edits := []FamilyEdit{
		parentEdit(FamilyOpRemove, "maud", "dad", ""),
		partnerEdit(FamilyOpAdd, "maud", "bob"),
		parentEdit(FamilyOpAdd, "tam", "dad", "birth"),
	}
	tests := []struct {
		name       string
		setup      func(*editFam)
		wantSaved  bool
		wantRolled bool
		wantFail   []int
		wantHalf   bool
		wantStuck  []int
		restored   bool
	}{
		{
			name:       "the last write fails: all taken back, the removed link restored with its Director-only mark",
			setup:      func(f *editFam) { f.failLink["tam>dad"] = true; f.markDM("maud", "dad") },
			wantRolled: true, wantFail: []int{2}, restored: true,
		},
		{
			name:       "a middle write fails: the one before is taken back, the one after never runs",
			setup:      func(f *editFam) { f.failLink["maud>bob"] = true },
			wantRolled: true, wantFail: []int{1}, restored: true,
		},
		{
			name:       "a pair left half-written is flagged and still taken back where it can be",
			setup:      func(f *editFam) { f.halfLink["maud>bob"] = true },
			wantRolled: true, wantFail: []int{1}, wantHalf: true,
		},
		{
			name:       "a takeback that fails is reported as still saved",
			setup:      func(f *editFam) { f.failLink["tam>dad"] = true; f.failRedo = true },
			wantRolled: false, wantFail: []int{2}, wantStuck: []int{0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			tt.setup(f)
			before := f.snapshot()
			res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{Edits: edits})
			if err != nil {
				t.Fatal(err)
			}
			if res.Saved != tt.wantSaved || res.RolledBack != tt.wantRolled {
				t.Fatalf("saved=%v rolledBack=%v, want %v/%v", res.Saved, res.RolledBack, tt.wantSaved, tt.wantRolled)
			}
			var failed []int
			for _, fl := range res.Failed {
				failed = append(failed, fl.Index)
				if fl.HalfWritten != tt.wantHalf {
					t.Errorf("halfWritten = %v, want %v", fl.HalfWritten, tt.wantHalf)
				}
			}
			if fmt.Sprint(failed) != fmt.Sprint(tt.wantFail) {
				t.Errorf("failed = %v, want %v", failed, tt.wantFail)
			}
			if tt.wantStuck != nil {
				var stuck []int
				for _, w := range res.Written {
					if !contains(failed, w) {
						stuck = append(stuck, w)
					}
				}
				if fmt.Sprint(stuck) != fmt.Sprint(tt.wantStuck) {
					t.Errorf("written-and-not-undone = %v, want %v", stuck, tt.wantStuck)
				}
			}
			if tt.restored {
				if f.snapshot() != before {
					t.Errorf("the campaign must be as it was before the save\nbefore:\n%s\nafter:\n%s", before, f.snapshot())
				}
				if res.Tree == nil || !hasParent(res.Tree.person("maud"), "dad", FamilyKindBirth) {
					t.Error("the tree returned must show the truth (dad is still a parent)")
				}
			}
		})
	}
}

func contains(l []int, x int) bool {
	for _, y := range l {
		if y == x {
			return true
		}
	}
	return false
}

// Pages already created stay (nothing deletes a page) and are reported, so the
// retry reuses them.
func TestFamilyEditPageCreationFailure(t *testing.T) {
	f := newEditFam()
	f.failCreate = 2
	before := f.snapshot()
	res, err := f.service().Apply(context.Background(), "c1", "maud", owner, false, FamilyEditRequest{
		NewPages: []FamilyNewPage{{Ref: "new:1", Name: "Odo"}, {Ref: "new:2", Name: "Pim"}},
		Edits:    []FamilyEdit{partnerEdit(FamilyOpAdd, "new:1", "new:2")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Saved || len(res.Created) != 1 || res.Created[0].Ref != "new:1" || len(res.Failed) != 1 || res.Failed[0].Index >= 0 {
		t.Errorf("result = %+v", res)
	}
	if len(f.ops) != 0 || f.snapshot() != before {
		t.Errorf("no link may be written when a page could not be made: %v", f.ops)
	}
}

// The picker is one bounded, searchable page of pages the Director can open,
// never the whole campaign.
func TestFamilyCandidatesAreBounded(t *testing.T) {
	big := make([]Entity, 150)
	for i := range big {
		big[i] = Entity{ID: fmt.Sprintf("p%03d", i), Name: fmt.Sprintf("Page %03d", i), TypeName: "Character"}
	}
	tests := []struct {
		name     string
		query    string
		found    []Entity
		skipKin  bool
		wantN    int
		wantMore bool
		wantErr  int
		wantList bool // the listing was asked at all
	}{
		{"no search: the first page only, and it says there is more", "", big, false, FamilyPickerMax, true, 0, true},
		{"a search narrows it", "page 14", big, false, 10, false, 0, true},
		{"a search that still matches many is bounded too", "page", big, false, FamilyPickerMax, true, 0, true},
		{"one letter is not a search", "p", big, false, 0, false, 0, false},
		{"search text is bounded", strings.Repeat("a", familyPickerQueryMax+1), big, false, 0, false, http.StatusBadRequest, false},
		{"no match is an empty list", "zzz", big, false, 0, false, 0, true},
		{"the page itself and its kin are not offered again", "", []Entity{{ID: "maud", Name: "Maud"}, {ID: "mum", Name: "Ione"}, {ID: "bob", Name: "Bob"}}, true, 1, false, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			f.found = tt.found
			f.hidden["p003"] = true
			got, err := f.service().Candidates(context.Background(), "c1", "maud", owner, tt.query)
			if tt.wantErr != 0 {
				assertAppError(t, err, tt.wantErr)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Pages) != tt.wantN || got.More != tt.wantMore {
				t.Errorf("got %d pages more=%v, want %d more=%v", len(got.Pages), got.More, tt.wantN, tt.wantMore)
			}
			if tt.wantList != (len(f.listOpts) > 0) {
				t.Errorf("listing asked = %v, want %v", len(f.listOpts) > 0, tt.wantList)
			}
			for i, o := range f.listOpts {
				if o.PerPage > 100 || o.PerPage < FamilyPickerMax {
					t.Errorf("the listing asked for %d rows; it must stay between %d and 100", o.PerPage, FamilyPickerMax)
				}
				if f.listRole[i] != int(permissions.RoleOwner) {
					t.Errorf("listed as role %d: the picker shows what the Director can see", f.listRole[i])
				}
			}
			for _, p := range got.Pages {
				if p.ID == "maud" || (tt.skipKin && (p.ID == "mum")) {
					t.Errorf("%s must not be offered", p.ID)
				}
			}
		})
	}

	t.Run("a page players cannot open is marked for the Director", func(t *testing.T) {
		f := newEditFam()
		f.found = []Entity{{ID: "p001", Name: "Open"}, {ID: "p003", Name: "Secret"}}
		f.names["p001"], f.names["p003"] = "Open", "Secret"
		f.hidden["p003"] = true
		got, err := f.service().Candidates(context.Background(), "c1", "maud", coDirector, "")
		if err != nil || len(got.Pages) != 2 {
			t.Fatalf("pages=%v err=%v", got, err)
		}
		for _, p := range got.Pages {
			if p.DMOnly != (p.ID == "p003") {
				t.Errorf("%s DMOnly=%v", p.ID, p.DMOnly)
			}
		}
	})
}

// The route gate lets in the owner and a granted co-Director and no one else;
// calling the handler without it is still refused by the service.
func TestFamilyEditRouteGate(t *testing.T) {
	cases := []struct {
		name string
		cc   *campaigns.CampaignContext
		ok   bool
	}{
		{"owner", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true}, true},
		{"granted co-Director", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsDmGranted: true, IsMember: true}, true},
		{"scribe", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleScribe, IsMember: true}, false},
		{"player", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RolePlayer, IsMember: true}, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			f := newEditFam()
			h := NewHandler(nil)
			h.SetFamilyEdit(f.service())
			c, rec := scopeContext(http.MethodPost, "/campaigns/c1/entities/maud/family", `{"edits":[{"op":"add","edge":"partner","a":"maud","b":"bob"}]}`, "maud")
			c.Set("campaign_context", tt.cc)
			auth.SetSession(c, &auth.Session{UserID: "u"})

			gated := RequireFamilyDirector()(h.FamilyEditAPI)
			err := gated(c)
			if tt.ok {
				if err != nil || rec.Code != http.StatusOK || !f.pair("maud", "partner of", "bob") {
					t.Fatalf("err=%v code=%d pair=%v", err, rec.Code, f.pair("maud", "partner of", "bob"))
				}
				return
			}
			assertAppError(t, err, http.StatusForbidden)
			if len(f.ops) != 0 {
				t.Errorf("a refused request wrote %v", f.ops)
			}

			// Without the route gate the service refuses on its own.
			c2, _ := scopeContext(http.MethodPost, "/campaigns/c1/entities/maud/family", `{"edits":[{"op":"add","edge":"partner","a":"maud","b":"bob"}]}`, "maud")
			c2.Set("campaign_context", tt.cc)
			auth.SetSession(c2, &auth.Session{UserID: "u"})
			err = h.FamilyEditAPI(c2)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusForbidden || len(f.ops) != 0 {
				t.Errorf("service must refuse on its own: err=%v ops=%v", err, f.ops)
			}
		})
	}
}

// A body that is not what the endpoint takes is a 400, not a panic or a write.
func TestFamilyEditHandlerRejectsBadBodies(t *testing.T) {
	for _, body := range []string{``, `not json`, `{"edits":"x"}`, `{"edits":[]}`, `{"edits":[{"op":"add","edge":"partner","a":"maud","b":"maud"}]}`, strings.Repeat("x", maxFamilyEditBody+10)} {
		f := newEditFam()
		h := NewHandler(nil)
		h.SetFamilyEdit(f.service())
		c, _ := scopeContext(http.MethodPost, "/x", body, "maud")
		c.Set("campaign_context", &campaigns.CampaignContext{Campaign: &campaigns.Campaign{ID: "c1"}, MemberRole: campaigns.RoleOwner, IsMember: true})
		assertAppError(t, h.FamilyEditAPI(c), http.StatusBadRequest)
		if len(f.ops) != 0 {
			t.Errorf("body %.20q wrote %v", body, f.ops)
		}
	}
}
