package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// fakeLooks keeps one look per page id; names maps ids to page names.
type fakeLooks struct {
	looks map[string]ItemLookValue
	names map[string]string
	saved map[string]ItemLookValue
	// hidden pages are left out of a player's list.
	hidden map[string]bool
}

func (f *fakeLooks) ref(id string) ItemLookRef {
	v := f.looks[id]
	r := ItemLookRef{PageID: id, Page: f.names[id], Value: v}
	if v.MadeFromID != "" {
		r.MadeFrom = f.names[v.MadeFromID]
	}
	return r
}
func (f *fakeLooks) Get(_ context.Context, _, id string, _ Actor) (*ItemLookRef, error) {
	if _, ok := f.looks[id]; !ok {
		return nil, nil
	}
	r := f.ref(id)
	return &r, nil
}
func (f *fakeLooks) List(_ context.Context, _ string, a Actor) ([]ItemLookRef, bool, error) {
	var out []ItemLookRef
	for _, id := range []string{"adder", "sword", "secret"} {
		if _, ok := f.looks[id]; ok && (a.Role >= 3 || !f.hidden[id]) {
			out = append(out, f.ref(id))
		}
	}
	return out, false, nil
}
func (f *fakeLooks) Check(_ context.Context, _, _ string, v ItemLookValue) error {
	if strings.Contains(v.Words, "!") {
		return apperror.NewBadRequest("invalid words")
	}
	return nil
}
func (f *fakeLooks) Save(_ context.Context, _, id string, _ Actor, v ItemLookValue) error {
	f.saved[id] = v
	return nil
}

type fakeOwners map[string]bool

func (f fakeOwners) IsOwner(_ context.Context, _, userID string) (bool, error) { return f[userID], nil }

func lookKind() (ItemLookKind, *fakeLooks) {
	f := &fakeLooks{
		looks: map[string]ItemLookValue{
			"adder":  {Look: "glass"},
			"sword":  {Look: "ember", Tint: "#ffd66b", Words: "6 fire", Spots: map[string]ItemLookSpot{"impact": {Look: "storm"}}, MadeFromID: "adder"},
			"secret": {Look: "rot"},
		},
		names:  map[string]string{"adder": "Glass adder", "sword": "Flametongue", "ring": "Ring", "secret": "Secret blade"},
		saved:  map[string]ItemLookValue{},
		hidden: map[string]bool{"secret": true},
	}
	pages := fakeEntities{}
	for id, name := range f.names {
		e := page(id, name, "Item", false)
		pages[e.Slug] = &e
	}
	return ItemLookKind{Svc: f, Entities: pages, Owners: fakeOwners{"owner": true}}, f
}

func lookRec(action, pg string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["page"] = pg
	return Record{Kind: "item-look", Action: action, Fields: fields}
}

func TestItemLookKind_Plan(t *testing.T) {
	made := lookRec(ActionCreate, "New Blade", map[string]any{"look": "frost"})
	made.Made = map[string]bool{"new-blade": true}
	codm := Actor{UserID: "codm", Role: 3}
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"a new look", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "color": "gold", "words": "4 cold"}), owner, `Frost shard in gold, words "4 cold"`, ""},
		{"the long name and a hex colour", lookRec(ActionCreate, "Ring", map[string]any{"look": "Dawn light", "color": "#8fd8ff"}), owner, "Dawn light in sky", ""},
		{"made from a page with no look of its own", lookRec(ActionCreate, "Ring", map[string]any{"made_from": "Glass adder"}), owner, "made from Glass adder", ""},
		{"spots", lookRec(ActionCreate, "Ring", map[string]any{"look": "ember", "spots": map[string]any{"impact": "storm", "ground": map[string]any{"look": "ember", "color": "orange"}}}), owner, "Ember strike, 2 spots", ""},
		{"an update keeps what isn't written", lookRec(ActionUpdate, "Flametongue", map[string]any{"sound": "none"}), owner, "now Ember strike in gold, sound none, words \"6 fire\", 1 spot, made from Glass adder", ""},
		{"none takes a spot away", lookRec(ActionUpdate, "Flametongue", map[string]any{"spots": map[string]any{"impact": "none"}}), owner, "now Ember strike in gold, words \"6 fire\", made from Glass adder", ""},
		{"remove", lookRec(ActionDelete, "Flametongue", nil), owner, "takes the look off the page", ""},
		{"a page made in this paste", made, owner, "gives the new page a look", ""},
		{"create over a look", lookRec(ActionCreate, "Flametongue", map[string]any{"look": "frost"}), owner, "", "already has a look; use action: update"},
		{"update with no look", lookRec(ActionUpdate, "Ring", map[string]any{"look": "frost"}), owner, "", "has no look yet"},
		{"remove with no look", lookRec(ActionDelete, "Ring", nil), owner, "", "no look to take away"},
		{"nothing to show", lookRec(ActionCreate, "Ring", map[string]any{"words": "hi"}), owner, "", "it needs `look`"},
		{"unknown look", lookRec(ActionCreate, "Ring", map[string]any{"look": "fireball"}), owner, "", "is not a look"},
		{"unknown colour", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "color": "#123456"}), owner, "", "is not one of"},
		{"unknown sound", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "sound": "scream"}), owner, "", "is not one of"},
		{"unknown spot", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "spots": map[string]any{"hand": "storm"}}), owner, "", "is not a spot"},
		{"words too long", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "words": strings.Repeat("a", 25)}), owner, "", "at most 24"},
		{"made from a missing page", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "made_from": "Nowhere"}), owner, "", "made_from: no page called"},
		{"made from itself", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "made_from": "Ring"}), owner, "", "made from itself"},
		{"the service refuses", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost", "words": "boom!"}), owner, "", "invalid words"},
		{"no such page", lookRec(ActionCreate, "Nowhere", map[string]any{"look": "frost"}), owner, "", "no page called"},
		{"no page named", lookRec(ActionCreate, "", map[string]any{"look": "frost"}), owner, "", "needs `page`"},
		{"a co-DM can't", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost"}), codm, "", "only the campaign's owner"},
		{"players can't", lookRec(ActionCreate, "Ring", map[string]any{"look": "frost"}), player, "", "only the campaign's owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k, _ := lookKind()
			p := k.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("plan = %+v, want summary %q", p, tt.summary)
			}
		})
	}
}

func TestItemLookKind_Apply(t *testing.T) {
	k, f := lookKind()
	ctx := context.Background()
	if err := k.Apply(ctx, camp, owner, lookRec(ActionUpdate, "Flametongue", map[string]any{"color": "none", "made_from": "none"})); err != nil {
		t.Fatal(err)
	}
	got := f.saved["sword"]
	if got.Look != "ember" || got.Tint != "" || got.MadeFromID != "" || got.Words != "6 fire" || got.Spots["impact"].Look != "storm" {
		t.Fatalf("update saved %+v", got)
	}
	if err := k.Apply(ctx, camp, owner, lookRec(ActionCreate, "Ring", map[string]any{"made_from": "Glass adder", "words": "glints"})); err != nil {
		t.Fatal(err)
	}
	if got := f.saved["ring"]; got.Look != "" || got.MadeFromID != "adder" || got.Words != "glints" {
		t.Fatalf("create saved %+v", got)
	}
	if err := k.Apply(ctx, camp, owner, lookRec(ActionDelete, "Glass adder", nil)); err != nil {
		t.Fatal(err)
	}
	if got, ok := f.saved["adder"]; !ok || got.Look != "" || got.Spots != nil || got.MadeFromID != "" {
		t.Fatalf("delete saved %+v", got)
	}
	if err := k.Apply(ctx, camp, player, lookRec(ActionCreate, "Ring", map[string]any{"look": "frost"})); err == nil {
		t.Fatal("a player's look was applied")
	}
}

// The export is valid item-look blocks, so a sent-back block round-trips.
func TestItemLookKind_Export(t *testing.T) {
	k, _ := lookKind()
	got, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nkind: item-look\npage: \"Flametongue\"\nlook: ember\ncolor: gold\nwords: \"6 fire\"\nspots:\n  impact: storm\nmade_from: \"Glass adder\"\n---\n"
	if !strings.Contains(got, want) {
		t.Fatalf("export:\n%s\nwant it to contain:\n%s", got, want)
	}
	if got, _ := k.Export(context.Background(), camp, Actor{UserID: "codm", Role: 3}); got != "" {
		t.Fatalf("a co-DM got an export: %q", got)
	}
}

func TestLookup_ItemLooks(t *testing.T) {
	_, f := lookKind()
	l := &Lookups{Looks: f}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("crafting", map[string]any{"part": "looks"})})
	text := ans[0].Text
	for _, want := range []string{"## Item looks", "page: \"Flametongue\"", "made_from: \"Glass adder\"", "page: \"Secret blade\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("owner's answer misses %q:\n%s", want, text)
		}
	}
	if ans[0].Chip != "Item looks · 3 looks" {
		t.Fatalf("chip = %q", ans[0].Chip)
	}

	ans = l.Answer(context.Background(), camp, player, []Record{lookup("looks", nil)})
	if strings.Contains(ans[0].Text, "Secret blade") {
		t.Fatalf("a player saw a hidden page's look:\n%s", ans[0].Text)
	}
}

func TestLookup_PageShowsItsLook(t *testing.T) {
	_, f := lookKind()
	sword := page("sword", "Flametongue", "Item", false)
	l := &Lookups{Looks: f, Pages: &fakePages{all: []entities.Entity{sword}}}
	ans := l.Answer(context.Background(), camp, owner, []Record{lookup("page", map[string]any{"name": "Flametongue"})})
	if ans[0].Error != "" || !strings.Contains(ans[0].Text, "### Look") || !strings.Contains(ans[0].Text, "look: ember") {
		t.Fatalf("page answer: %+v", ans[0])
	}
}
