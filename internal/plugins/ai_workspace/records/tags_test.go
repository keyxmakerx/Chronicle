package records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// fakeTags is an in-memory tags service; byPage maps a page id to tag ids.
type fakeTags struct {
	list    []tags.Tag
	byPage  map[string][]int
	created []tags.Tag
	updated map[int]tags.UpdateTagInput
	deleted []int
	// sawDm records the includeDmOnly flag of every read.
	sawDm []bool
}

func (f *fakeTags) ListByCampaign(_ context.Context, _ string, dm bool) ([]tags.Tag, error) {
	f.sawDm = append(f.sawDm, dm)
	var out []tags.Tag
	for _, t := range f.list {
		if dm || !t.DmOnly {
			out = append(out, t)
		}
	}
	return out, nil
}
func (f *fakeTags) Create(_ context.Context, _ string, name, color string, dmOnly bool) (*tags.Tag, error) {
	t := tags.Tag{ID: 100 + len(f.created), Name: name, Color: color, DmOnly: dmOnly}
	f.created = append(f.created, t)
	return &t, nil
}
func (f *fakeTags) Update(_ context.Context, id int, in tags.UpdateTagInput) (*tags.Tag, error) {
	f.updated[id] = in
	return &tags.Tag{ID: id}, nil
}
func (f *fakeTags) Delete(_ context.Context, id int) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeTags) GetEntityTagsBatch(_ context.Context, ids []string, dm bool) (map[string][]tags.Tag, error) {
	f.sawDm = append(f.sawDm, dm)
	out := map[string][]tags.Tag{}
	for _, id := range ids {
		for _, tid := range f.byPage[id] {
			for _, t := range f.list {
				if t.ID == tid && (dm || !t.DmOnly) {
					out[id] = append(out[id], t)
				}
			}
		}
	}
	return out, nil
}

func tagWorld() (*fakeTags, *fakePages) {
	ft := &fakeTags{
		list: []tags.Tag{
			{ID: 1, Name: "Gods", Slug: "gods", Color: "#8b5cf6"},
			{ID: 2, Name: "Trade Hub", Slug: "trade-hub", Color: "#6b7280"},
			{ID: 3, Name: "Secret Cult", Slug: "secret-cult", Color: "#ef4444", DmOnly: true},
		},
		byPage:  map[string][]int{"aldric": {1, 3}, "anchor": {2}, "rope": {1}},
		updated: map[int]tags.UpdateTagInput{},
	}
	fp := &fakePages{all: []entities.Entity{
		page("aldric", "Ser Aldric", "Character", false),
		page("anchor", "The Rusty Anchor", "Location", false),
		page("rope", "Rope", "Item", false),
		page("map", "Treasure Map", "Item", true),
	}}
	return ft, fp
}

func tagRec(action, name string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: "tag", Action: action, Name: name, Fields: fields}
}

func TestTagKind_Plan(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"new tag", tagRec(ActionCreate, "Moons", map[string]any{"color": "#112233", "dm_only": true}), owner, "makes the tag in #112233, hidden from players", ""},
		{"new tag, no keys", tagRec(ActionCreate, "Moons", nil), owner, "makes the tag", ""},
		{"spelling finds the same tag", tagRec(ActionCreate, "trade  hub", nil), owner, "", "already exists"},
		{"rename on create", tagRec(ActionCreate, "Moons", map[string]any{"rename_to": "X"}), owner, "", "only applies to action: update"},
		{"bad colour", tagRec(ActionCreate, "Moons", map[string]any{"color": "purple"}), owner, "", "#RRGGBB"},
		{"short colour", tagRec(ActionCreate, "Moons", map[string]any{"color": "#fff"}), owner, "", "#RRGGBB"},
		{"too long", tagRec(ActionCreate, strings.Repeat("a", 101), nil), owner, "", "at most 100"},
		{"no name", tagRec(ActionCreate, "", nil), owner, "", "needs a name"},
		{"players refused", tagRec(ActionCreate, "Moons", nil), player, "", "only the owner or a co-DM"},
		{"rename", tagRec(ActionUpdate, "Gods", map[string]any{"rename_to": "Deities"}), owner, `renamed to "Deities"`, ""},
		{"rename onto another tag", tagRec(ActionUpdate, "Gods", map[string]any{"rename_to": "Trade Hub"}), owner, "", "already exists"},
		{"rename to its own spelling", tagRec(ActionUpdate, "Gods", map[string]any{"rename_to": "GODS"}), owner, `renamed to "GODS"`, ""},
		{"hide", tagRec(ActionUpdate, "Gods", map[string]any{"dm_only": true}), owner, "hidden from players", ""},
		{"show", tagRec(ActionUpdate, "Secret Cult", map[string]any{"dm_only": false}), owner, "shown to players", ""},
		{"nothing to change", tagRec(ActionUpdate, "Gods", nil), owner, "", "nothing to change"},
		{"update missing", tagRec(ActionUpdate, "Nope", map[string]any{"color": "#000000"}), owner, "", "no tag called"},
		{"remove, with count", tagRec(ActionDelete, "Gods", nil), owner, "removes the tag from 2 pages", ""},
		{"remove, one page", tagRec(ActionDelete, "Trade Hub", nil), owner, "removes the tag from 1 page", ""},
		{"remove missing", tagRec(ActionDelete, "Nope", nil), owner, "", "no tag called"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft, fp := tagWorld()
			p := TagKind{Svc: ft, Pages: fp}.Plan(context.Background(), camp, tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error %q; want it to contain %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || p.Summary != tt.summary {
				t.Fatalf("got %q / %q; want %q", p.Summary, p.Error, tt.summary)
			}
		})
	}
}

func TestTagKind_RemoveWithoutPagesSaysEveryPage(t *testing.T) {
	ft, _ := tagWorld()
	p := TagKind{Svc: ft}.Plan(context.Background(), camp, owner, tagRec(ActionDelete, "Gods", nil))
	if p.Summary != "removes the tag from every page that carries it" {
		t.Fatal(p.Summary)
	}
}

func TestTagKind_Apply(t *testing.T) {
	ctx := context.Background()
	ft, fp := tagWorld()
	k := TagKind{Svc: ft, Pages: fp}

	if err := k.Apply(ctx, camp, owner, tagRec(ActionCreate, "Moons", map[string]any{"color": "#112233", "dm_only": true})); err != nil {
		t.Fatal(err)
	}
	if len(ft.created) != 1 || ft.created[0].Name != "Moons" || ft.created[0].Color != "#112233" || !ft.created[0].DmOnly {
		t.Fatalf("created %+v", ft.created)
	}

	// A rename alone must leave color and dm_only out, so the service keeps them.
	if err := k.Apply(ctx, camp, owner, tagRec(ActionUpdate, "Gods", map[string]any{"rename_to": "Deities"})); err != nil {
		t.Fatal(err)
	}
	if in := ft.updated[1]; in.Name != "Deities" || in.Color != nil || in.DmOnly != nil {
		t.Fatalf("rename sent %+v", in)
	}

	// Hiding alone keeps the stored name.
	if err := k.Apply(ctx, camp, owner, tagRec(ActionUpdate, "Gods", map[string]any{"dm_only": true})); err != nil {
		t.Fatal(err)
	}
	if in := ft.updated[1]; in.Name != "Gods" || in.DmOnly == nil || !*in.DmOnly || in.Color != nil {
		t.Fatalf("hide sent %+v", in)
	}

	if err := k.Apply(ctx, camp, owner, tagRec(ActionDelete, "Trade Hub", nil)); err != nil {
		t.Fatal(err)
	}
	if len(ft.deleted) != 1 || ft.deleted[0] != 2 {
		t.Fatalf("deleted %v", ft.deleted)
	}

	if err := k.Apply(ctx, camp, player, tagRec(ActionDelete, "Gods", nil)); err == nil {
		t.Fatal("a player removed a tag")
	}
	if len(ft.deleted) != 1 {
		t.Fatal("a refused removal still deleted")
	}
}

func TestTagKind_Export(t *testing.T) {
	ft, _ := tagWorld()
	k := TagKind{Svc: ft}
	s, err := k.Export(context.Background(), camp, owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: tag\nname: \"Gods\"\ncolor: \"#8b5cf6\"", "name: \"Secret Cult\"", "dm_only: true"} {
		if !strings.Contains(s, want) {
			t.Errorf("export lacks %q:\n%s", want, s)
		}
	}
	if s, _ := k.Export(context.Background(), camp, player); s != "" {
		t.Errorf("a player's export lists tags: %q", s)
	}
}

type failingTags struct{ fakeTags }

func (failingTags) Delete(context.Context, int) error { return errors.New("boom") }

func TestTagKind_ApplyReturnsServiceError(t *testing.T) {
	ft, _ := tagWorld()
	k := TagKind{Svc: &failingTags{*ft}}
	if err := k.Apply(context.Background(), camp, owner, tagRec(ActionDelete, "Gods", nil)); err == nil {
		t.Fatal("service error was swallowed")
	}
}

func TestLookups_Tags(t *testing.T) {
	tests := []struct {
		name    string
		r       Record
		a       Actor
		want    []string
		notWant []string
		err     string
	}{
		{"director sees all with counts", lookup("tags", nil), owner,
			[]string{"Gods, #8b5cf6, 2 pages", "Trade Hub, #6b7280, 1 page", "Secret Cult, #ef4444, Directors only, 1 page"}, nil, ""},
		{"player never sees a hidden tag", lookup("tags", nil), player,
			[]string{"Gods, #8b5cf6, 2 pages"}, []string{"Secret Cult", "Directors only"}, ""},
		{"pages show their tags", lookup("pages", nil), owner,
			[]string{"Ser Aldric [Gods, Secret Cult]", "The Rusty Anchor [Trade Hub]"}, nil, ""},
		{"a player's pages hide the hidden tag", lookup("pages", nil), player,
			[]string{"Ser Aldric [Gods]"}, []string{"Secret Cult"}, ""},
		{"pages by tag name", lookup("pages", map[string]any{"tag": "trade hub"}), owner,
			[]string{"The Rusty Anchor"}, []string{"Ser Aldric", "Rope"}, ""},
		{"pages by tag slug", lookup("pages", map[string]any{"tag": "gods"}), player,
			[]string{"Ser Aldric", "Rope"}, []string{"Rusty"}, ""},
		{"tag and type", lookup("pages", map[string]any{"tag": "Gods", "type": "Item"}), owner,
			[]string{"Rope"}, []string{"Ser Aldric"}, ""},
		{"a hidden tag filter finds nothing for a player", lookup("pages", map[string]any{"tag": "Secret Cult"}), player,
			nil, nil, "no pages tagged"},
		{"a hidden tag filter works for a director", lookup("pages", map[string]any{"tag": "Secret Cult"}), owner,
			[]string{"Ser Aldric"}, nil, ""},
		{"unknown tag", lookup("pages", map[string]any{"tag": "Nope"}), owner, nil, nil, "no pages tagged"},
		{"tag and type, none", lookup("pages", map[string]any{"tag": "Gods", "type": "Location"}), owner, nil, nil, "no pages of type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft, fp := tagWorld()
			l := &Lookups{Pages: fp, Tags: ft}
			got := l.Answer(context.Background(), camp, tt.a, []Record{tt.r})[0]
			if tt.err != "" {
				if !strings.Contains(got.Error, tt.err) {
					t.Fatalf("error %q; want %q", got.Error, tt.err)
				}
				return
			}
			if got.Error != "" {
				t.Fatal(got.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(got.Text, w) {
					t.Errorf("answer lacks %q:\n%s", w, got.Text)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(got.Text, w) {
					t.Errorf("answer has %q:\n%s", w, got.Text)
				}
			}
		})
	}
}

func TestLookups_TagsPrivacyFlag(t *testing.T) {
	ft, fp := tagWorld()
	l := &Lookups{Pages: fp, Tags: ft}
	l.Answer(context.Background(), camp, player, []Record{lookup("tags", nil), lookup("pages", map[string]any{"tag": "gods"}), lookup("page", map[string]any{"name": "Ser Aldric"})})
	for _, dm := range ft.sawDm {
		if dm {
			t.Fatal("a player-privacy lookup asked for Directors-only tags")
		}
	}
	if len(ft.sawDm) == 0 {
		t.Fatal("no tag reads happened")
	}
}

func TestLookups_TagsNotWired(t *testing.T) {
	_, fp := tagWorld()
	l := &Lookups{Pages: fp}
	if got := l.Answer(context.Background(), camp, owner, []Record{lookup("tags", nil)})[0]; !strings.Contains(got.Error, "can't be looked up") {
		t.Errorf("tags: %q", got.Error)
	}
	if got := l.Answer(context.Background(), camp, owner, []Record{lookup("pages", map[string]any{"tag": "x"})})[0]; !strings.Contains(got.Error, "can't be looked up") {
		t.Errorf("pages by tag: %q", got.Error)
	}
	if got := l.Answer(context.Background(), camp, owner, []Record{lookup("pages", nil)})[0]; got.Error != "" {
		t.Errorf("plain pages broke without tags: %q", got.Error)
	}
}
