package entities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/permissions"
)

// famFixture is a campaign's pages and relations, held in memory. Every
// relation is stored as the mirrored pair the relations service writes.
type famFixture struct {
	links  map[string][]FamilyLink
	names  map[string]string
	images map[string]string
	hidden map[string]bool // pages an ordinary player cannot open
	reads  int
}

func newFam() *famFixture {
	return &famFixture{links: map[string][]FamilyLink{}, names: map[string]string{}, images: map[string]string{}, hidden: map[string]bool{}}
}

var famReverse = map[string]string{
	"parent of": "child of", "child of": "parent of",
	"adoptive parent of": "adopted child of", "adopted child of": "adoptive parent of",
	"guardian of": "ward of", "ward of": "guardian of",
}

// page adds a page with a name; hidden pages are the Director's.
func (f *famFixture) page(id, name string, hidden bool) {
	f.names[id] = name
	if hidden {
		f.hidden[id] = true
	}
}

// rel adds "a <typ> b" and its reverse row on b.
func (f *famFixture) rel(a, typ, b string, dmOnly bool) {
	rev, ok := famReverse[strings.ToLower(strings.TrimSpace(typ))]
	if !ok {
		rev = typ
	}
	f.links[a] = append(f.links[a], FamilyLink{OtherID: b, Type: typ, DMOnly: dmOnly})
	f.links[b] = append(f.links[b], FamilyLink{OtherID: a, Type: rev, DMOnly: dmOnly})
}

func (f *famFixture) LinksOf(_ context.Context, _, id string) ([]FamilyLink, error) {
	f.reads++
	return f.links[id], nil
}

func (f *famFixture) InCampaign(_ context.Context, _ string, ids []string) (map[string]PageCard, error) {
	out := map[string]PageCard{}
	for _, id := range ids {
		if n, ok := f.names[id]; ok {
			out[id] = PageCard{ID: id, Name: n, ImagePath: f.images[id], TypeName: "Character"}
		}
	}
	return out, nil
}

func (f *famFixture) FilterViewableEntityIDs(_ context.Context, _ string, ids []string, role int, _ string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		if _, ok := f.names[id]; !ok {
			continue
		}
		out[id] = role >= int(permissions.RoleOwner) || !f.hidden[id]
	}
	return out, nil
}

func (f *famFixture) service() FamilyTreeService { return NewFamilyTreeService(f, f, f) }

func (t *FamilyTree) person(id string) *FamilyPerson {
	for i := range t.People {
		if t.People[i].ID == id {
			return &t.People[i]
		}
	}
	return nil
}

func (t *FamilyTree) ids() []string {
	var out []string
	for _, p := range t.People {
		out = append(out, p.ID)
	}
	return out
}

func hasParent(p *FamilyPerson, id, kind string) bool {
	if p == nil {
		return false
	}
	for _, x := range p.Parents {
		if x.ID == id && x.Kind == kind {
			return true
		}
	}
	return false
}

// A house: gran > mum, dad (partners) > maud (the page) > tam; maud adopted
// perrin and is guardian of kessa; "knows" is not family.
func houseFixture() *famFixture {
	f := newFam()
	for id, n := range map[string]string{"gran": "Edda", "mum": "Ione", "dad": "Corvin", "maud": "Maud", "tam": "Tam", "perrin": "Perrin", "kessa": "Kessa", "bob": "Bob"} {
		f.page(id, n, false)
	}
	f.rel("gran", "parent of", "mum", false)
	f.rel("mum", "parent of", "maud", false)
	f.rel("dad", "Parent Of ", "maud", false) // matched without regard to case or padding
	f.rel("mum", "married to", "dad", false)
	f.rel("maud", "parent of", "tam", false)
	f.rel("maud", "adoptive parent of", "perrin", false)
	f.rel("maud", "guardian of", "kessa", false)
	f.rel("maud", "knows", "bob", false)
	return f
}

func TestFamilyTreeReadsKindsAndPartners(t *testing.T) {
	tree, err := houseFixture().service().Tree(context.Background(), "c1", "maud", int(permissions.RoleOwner), "u1")
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.ids()[0]; got != "maud" {
		t.Fatalf("the page must come first, got %q", got)
	}
	cases := []struct {
		name string
		ok   func() bool
	}{
		{"mum is a birth parent", func() bool { return hasParent(tree.person("maud"), "mum", FamilyKindBirth) }},
		{"dad matched through case and padding", func() bool { return hasParent(tree.person("maud"), "dad", FamilyKindBirth) }},
		{"gran is mum's parent", func() bool { return hasParent(tree.person("mum"), "gran", FamilyKindBirth) }},
		{"tam is a birth child", func() bool { return hasParent(tree.person("tam"), "maud", FamilyKindBirth) }},
		{"perrin is adopted", func() bool { return hasParent(tree.person("perrin"), "maud", FamilyKindAdopted) }},
		{"kessa is a ward", func() bool { return hasParent(tree.person("kessa"), "maud", FamilyKindGuardian) }},
		{"mum and dad are partners", func() bool {
			return containsString(tree.person("mum").Partners, "dad") && containsString(tree.person("dad").Partners, "mum")
		}},
		{"a knows relation is not family", func() bool { return tree.person("bob") == nil }},
		{"the page itself was read", func() bool { return tree.person("maud").Read }},
		{"nothing was cut", func() bool { return !tree.Cut() }},
		{"nothing is marked DM-only", func() bool {
			for _, p := range tree.People {
				if p.DMOnly {
					return false
				}
			}
			return true
		}},
	}
	for _, c := range cases {
		if !c.ok() {
			t.Errorf("%s: tree = %+v", c.name, tree.People)
		}
	}
	if got := tree.Relatives(); got != 6 {
		t.Errorf("relatives = %d, want 6", got)
	}
}

// secretsFixture adds, around the house, a half-brother only the Director can
// open, a guardian relation only the Director sees, and a great-aunt reached
// only through a hidden parent.
func secretsFixture() *famFixture {
	f := houseFixture()
	f.page("half", "Corwin Mere", true)
	f.rel("dad", "parent of", "half", false)
	f.page("steward", "Old Steward", false)
	f.rel("steward", "guardian of", "maud", true)
	f.page("hiddenmum", "Veiled Mother", true)
	f.page("auntie", "Great Aunt Ysmere", false)
	f.rel("hiddenmum", "parent of", "maud", false)
	f.rel("auntie", "parent of", "hiddenmum", false)
	return f
}

func TestFamilyTreeNeverHandsAPlayerTheDirectorsHalf(t *testing.T) {
	secrets := []string{"half", "Corwin Mere", "steward", "Old Steward", "hiddenmum", "Veiled Mother", "auntie", "Great Aunt Ysmere"}
	tests := []struct {
		name       string
		role       int
		wantIn     []string
		wantAbsent []string
		dmMarked   []string
	}{
		{
			name: "player", role: int(permissions.RolePlayer),
			wantIn:     []string{"maud", "mum", "dad", "tam", "gran"},
			wantAbsent: []string{"half", "steward", "hiddenmum", "auntie"},
		},
		{
			name: "scribe is still a player here", role: int(permissions.RoleScribe),
			wantIn:     []string{"maud", "mum", "dad", "tam"},
			wantAbsent: []string{"half", "steward", "hiddenmum", "auntie"},
		},
		{
			name: "director", role: int(permissions.RoleOwner),
			wantIn:   []string{"maud", "mum", "dad", "tam", "gran", "half", "steward", "hiddenmum", "auntie"},
			dmMarked: []string{"half", "hiddenmum"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := secretsFixture().service().Tree(context.Background(), "c1", "maud", tt.role, "u1")
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tt.wantIn {
				if tree.person(id) == nil {
					t.Errorf("%s missing from %v", id, tree.ids())
				}
			}
			for _, id := range tt.wantAbsent {
				if tree.person(id) != nil {
					t.Errorf("%s must not reach this viewer", id)
				}
			}
			for _, id := range tt.dmMarked {
				if p := tree.person(id); p == nil || !p.DMOnly {
					t.Errorf("%s should carry the DM mark: %+v", id, p)
				}
			}
			// The wire the script reads is the last stop before the browser.
			raw, err := json.Marshal(familyWireOf(context.Background(), "c1", tree))
			if err != nil {
				t.Fatal(err)
			}
			if tt.role < int(permissions.RoleOwner) {
				for _, s := range secrets {
					if strings.Contains(string(raw), s) {
						t.Errorf("player JSON mentions %q: %s", s, raw)
					}
				}
				if strings.Contains(string(raw), `"dm":true`) {
					t.Errorf("player JSON carries a DM mark: %s", raw)
				}
			}
		})
	}
}

func TestFamilyTreeDMOnlyRelationIsMarkedForTheDirectorAndDroppedForPlayers(t *testing.T) {
	f := newFam()
	f.page("a", "Ansel", false)
	f.page("b", "Bryn", false)
	f.rel("a", "parent of", "b", true)
	dm, err := f.service().Tree(context.Background(), "c1", "b", int(permissions.RoleOwner), "u")
	if err != nil {
		t.Fatal(err)
	}
	if p := dm.person("b"); p == nil || len(p.Parents) != 1 || !p.Parents[0].DMOnly {
		t.Errorf("the Director should get the link marked DM-only: %+v", dm.People)
	}
	pl, err := f.service().Tree(context.Background(), "c1", "b", int(permissions.RolePlayer), "u")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Relatives() != 0 || len(pl.person("b").Parents) != 0 {
		t.Errorf("a player should get no trace of the link: %+v", pl.People)
	}
}

func TestFamilyTreeBounds(t *testing.T) {
	const gens = 7
	chainUp := func() *famFixture {
		f := newFam()
		f.page("p0", "P0", false)
		for i := 1; i <= gens; i++ {
			f.page(fmt.Sprintf("p%d", i), fmt.Sprintf("P%d", i), false)
			f.rel(fmt.Sprintf("p%d", i), "parent of", fmt.Sprintf("p%d", i-1), false)
		}
		return f
	}
	chainDown := func() *famFixture {
		f := newFam()
		f.page("p0", "P0", false)
		for i := 1; i <= gens; i++ {
			f.page(fmt.Sprintf("p%d", i), fmt.Sprintf("P%d", i), false)
			f.rel(fmt.Sprintf("p%d", i-1), "parent of", fmt.Sprintf("p%d", i), false)
		}
		return f
	}
	wide := func() *famFixture {
		f := newFam()
		f.page("p0", "P0", false)
		for i := 1; i <= MaxFamilyPeople+25; i++ {
			id := fmt.Sprintf("k%d", i)
			f.page(id, id, false)
			f.rel("p0", "parent of", id, false)
		}
		return f
	}
	exact := func() *famFixture {
		f := newFam()
		f.page("p0", "P0", false)
		for i := 1; i <= MaxFamilyGenerations; i++ {
			f.page(fmt.Sprintf("p%d", i), "P", false)
			f.rel(fmt.Sprintf("p%d", i), "parent of", fmt.Sprintf("p%d", i-1), false)
		}
		return f
	}
	tests := []struct {
		name         string
		fix          *famFixture
		wantPeople   int
		wantCutGen   bool
		wantCutPeopl bool
		maxReads     int
	}{
		{"a long line up stops at the generation bound", chainUp(), 1 + MaxFamilyGenerations, true, false, 2 * gens},
		{"a long line down stops at the generation bound", chainDown(), 1 + MaxFamilyGenerations, true, false, 2 * gens},
		{"a family exactly at the bound says nothing was cut", exact(), 1 + MaxFamilyGenerations, false, false, 2 * gens},
		{"a crowd keeps the people bound", wide(), MaxFamilyPeople, false, true, maxFamilyReads},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := tt.fix.service().Tree(context.Background(), "c1", "p0", int(permissions.RolePlayer), "u")
			if err != nil {
				t.Fatal(err)
			}
			if len(tree.People) != tt.wantPeople {
				t.Errorf("people = %d, want %d", len(tree.People), tt.wantPeople)
			}
			if tree.CutGenerations != tt.wantCutGen || tree.CutPeople != tt.wantCutPeopl {
				t.Errorf("cut = generations %v people %v, want %v %v", tree.CutGenerations, tree.CutPeople, tt.wantCutGen, tt.wantCutPeopl)
			}
			if tt.fix.reads > tt.maxReads {
				t.Errorf("relation reads = %d, over the bound %d", tt.fix.reads, tt.maxReads)
			}
			if tt.wantCutGen != familyHasCutNote(tree, "generations") {
				t.Errorf("the block's words must match the cut: %q", familyCutNote(tree))
			}
		})
	}
}

func familyHasCutNote(t *FamilyTree, word string) bool {
	return strings.Contains(familyCutNote(t), word)
}

func TestFamilyTreeUnreadEdgeAndMissingPages(t *testing.T) {
	f := newFam()
	f.page("p0", "P0", false)
	f.rel("p0", "parent of", "gone", false) // no page behind it any more
	tree, err := f.service().Tree(context.Background(), "c1", "p0", int(permissions.RoleOwner), "u")
	if err != nil {
		t.Fatal(err)
	}
	if tree.Relatives() != 0 {
		t.Errorf("a link to a missing page must not draw anyone: %+v", tree.People)
	}

	// The people at the last generation are read (to know there is more) but
	// ones past the cut are not, and say so.
	f2 := newFam()
	f2.page("p0", "P0", false)
	for i := 1; i <= 3; i++ {
		f2.page(fmt.Sprintf("a%d", i), "A", false)
		f2.page(fmt.Sprintf("s%d", i), "S", false)
	}
	f2.rel("a1", "parent of", "p0", false)
	f2.rel("a1", "parent of", "s1", false) // a sibling is found, not read
	tr2, err := f2.service().Tree(context.Background(), "c1", "p0", int(permissions.RolePlayer), "u")
	if err != nil {
		t.Fatal(err)
	}
	if s := tr2.person("s1"); s == nil || s.Read {
		t.Errorf("a sibling is drawn but their own relations were not read: %+v", s)
	}
	if a := tr2.person("a1"); a == nil || !a.Read {
		t.Errorf("an ancestor's relations were read: %+v", a)
	}
}

func TestFamilyTreePageNotOpenToViewer(t *testing.T) {
	f := newFam()
	f.page("secret", "Secret", true)
	_, err := f.service().Tree(context.Background(), "c1", "secret", int(permissions.RolePlayer), "u")
	var ae *apperror.AppError
	if err == nil || !errors.As(err, &ae) || ae.Code != 404 {
		t.Errorf("want a not-found for a page the viewer cannot open, got %v", err)
	}
}

func TestFamilyRelationTable(t *testing.T) {
	tests := []struct {
		in   string
		side familySide
		kind string
		ok   bool
	}{
		{"parent of", familyOtherIsChild, FamilyKindBirth, true},
		{" Child Of", familyOtherIsParent, FamilyKindBirth, true},
		{"adoptive parent of", familyOtherIsChild, FamilyKindAdopted, true},
		{"adopted child of", familyOtherIsParent, FamilyKindAdopted, true},
		{"guardian of", familyOtherIsChild, FamilyKindGuardian, true},
		{"ward of", familyOtherIsParent, FamilyKindGuardian, true},
		{"married to", familyOtherIsPartner, "", true},
		{"spouse of", familyOtherIsPartner, "", true},
		{"partner of", familyOtherIsPartner, "", true},
		{"allied with", familyNone, "", false},
		{"", familyNone, "", false},
	}
	for _, tt := range tests {
		r, ok := classifyFamilyRelation(tt.in)
		if ok != tt.ok || (ok && (r.side != tt.side || r.kind != tt.kind)) {
			t.Errorf("%q -> %+v %v", tt.in, r, ok)
		}
	}
}

func TestFamilyCountAndCutWords(t *testing.T) {
	mk := func(n int, gen, ppl bool) *FamilyTree {
		t := &FamilyTree{CutGenerations: gen, CutPeople: ppl}
		for i := 0; i <= n; i++ {
			t.People = append(t.People, FamilyPerson{ID: fmt.Sprint(i)})
		}
		return t
	}
	tests := []struct {
		tree  *FamilyTree
		count string
		note  string
	}{
		{mk(1, false, false), "1 relative", ""},
		{mk(12, false, false), "12 relatives", ""},
		{mk(60, false, true), "At least 60 relatives", "Showing the nearest 60 relatives; the rest are on their own pages."},
		{mk(8, true, false), "At least 8 relatives", "Showing 4 generations each way; the family goes on past that."},
		{mk(60, true, true), "At least 60 relatives", "Showing the nearest 60 relatives and 4 generations each way; the family goes on past that."},
	}
	for _, tt := range tests {
		if got := familyCountLabel(tt.tree); got != tt.count {
			t.Errorf("count = %q, want %q", got, tt.count)
		}
		if got := familyCutNote(tt.tree); got != tt.note {
			t.Errorf("note = %q, want %q", got, tt.note)
		}
	}
}
