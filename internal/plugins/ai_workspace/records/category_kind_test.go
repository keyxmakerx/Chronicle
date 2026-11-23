package records

import (
	"context"
	"strings"
	"testing"
)

// fakeCategories records which service call an approved row made.
type fakeCategories struct {
	cats    []CategoryInfo
	owner   string
	deleted []int
	merged  [][2]int
}

func (f *fakeCategories) IsOwner(_ context.Context, _, userID string) (bool, error) {
	return userID == f.owner, nil
}
func (f *fakeCategories) Categories(context.Context, string) ([]CategoryInfo, error) {
	return f.cats, nil
}
func (f *fakeCategories) DeleteCategory(_ context.Context, _ string, id int) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeCategories) MergeCategory(_ context.Context, _ string, from, to int) (int, error) {
	f.merged = append(f.merged, [2]int{from, to})
	return 0, nil
}

// categoriesFake is the campaign that asked: a DM-only Adventures with
// pages, an empty duplicate of it, and a few others.
func categoriesFake() *fakeCategories {
	return &fakeCategories{owner: "own", cats: []CategoryInfo{
		{ID: 1, Name: "Adventure", Plural: "Adventures", Slug: "adventures", DMOnly: true, Pages: 11, Kinds: nil},
		{ID: 2, Name: "Adventure", Plural: "Adventures", Slug: "adventures-2"},
		{ID: 3, Name: "Lore", Plural: "Lore", Slug: "lore", Pages: 4, Trashed: 1},
		{ID: 4, Name: "Secret", Plural: "Secrets", Slug: "secret", DMOnly: true, Pages: 2},
		{ID: 5, Name: "Location", Plural: "Places", Slug: "location", Pages: 3, Kinds: []string{"Cities"}},
		{ID: 6, Name: "City", Plural: "Cities", Slug: "city", Parent: "Places", Pages: 1},
		{ID: 7, Name: "Rumour", Plural: "Rumours", Slug: "rumour", Trashed: 2},
	}}
}

func TestCategoryKind_Plan(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name    string
		actor   Actor
		action  string
		fields  map[string]any
		summary string
		err     string
		warn    string
	}{
		{"empty category is deleted", owner, ActionDelete, map[string]any{"category": "adventures-2"},
			"Adventures (adventures-2) is deleted. It has no pages.", "", "page layout, home and fields are deleted"},
		{"empty category with a target only deletes", owner, ActionDelete, map[string]any{"category": "adventures-2", "move_pages_to": "Lore"},
			"Adventures (adventures-2) is deleted. It has no pages, so nothing moves into Lore.", "", ""},
		{"pages and no target refused", owner, ActionDelete, map[string]any{"category": "Lore"},
			"", "Lore (lore) still has 4 pages and the 1 in the Trash; name the category that takes them in `move_pages_to`", ""},
		{"pages only in the Trash still count", owner, ActionDelete, map[string]any{"category": "Rumours"},
			"", "still has 2 pages in the Trash", ""},
		{"merge", owner, ActionDelete, map[string]any{"category": "Lore", "move_pages_to": "Secrets"},
			"Lore (lore) is deleted; its 4 pages and the 1 in the Trash and its folders move into Secrets.", "",
			"values in fields Secrets doesn't have stay saved on the pages but are not shown"},
		{"merge into DM-only warns players lose them", owner, ActionDelete, map[string]any{"category": "Lore", "move_pages_to": "secret"},
			"move into Secrets", "", "players stop seeing these pages: Secrets is for the Director team only"},
		{"merge names a shared name by its slug", owner, ActionDelete, map[string]any{"category": "Secrets", "move_pages_to": "adventures"},
			"its 2 pages and its folders move into Adventures (adventures).", "", ""},
		{"a slug that is another category's name is pointed out", owner, ActionDelete, map[string]any{"category": "Secrets", "move_pages_to": "adventures"},
			"move into Adventures (adventures).", "", "\"adventures\" is the slug of Adventures; the category called Adventures is \"adventures-2\""},
		{"ambiguous name refused listing slugs", owner, ActionDelete, map[string]any{"category": "Adventure"},
			"", "two categories are called Adventure: adventures (11 pages, only the Director team sees it) and adventures-2 (no pages); name one by its slug", ""},
		{"ambiguous target refused", owner, ActionDelete, map[string]any{"category": "Secrets", "move_pages_to": "Adventures"},
			"", "two categories are called Adventures", ""},
		{"slug settles a shared name", owner, ActionDelete, map[string]any{"category": "adventures-2"},
			"Adventures (adventures-2) is deleted", "", ""},
		{"a slug that reads like the shared name settles it", owner, ActionDelete, map[string]any{"category": "adventures", "move_pages_to": "secret"},
			"Adventures (adventures) is deleted; its 11 pages and its folders move into Secrets.", "", ""},
		{"a unique slug matches in any case", owner, ActionDelete, map[string]any{"category": "ADVENTURES-2"},
			"Adventures (adventures-2) is deleted", "", ""},
		{"block name stands in for category", owner, ActionDelete, map[string]any{"name": "Rumours", "move_pages_to": "Lore"},
			"Rumours (rumour) is deleted; its 2 pages in the Trash and its folders move into Lore.", "", ""},
		{"DM-only into visible refused", owner, ActionDelete, map[string]any{"category": "adventures", "move_pages_to": "Lore"},
			"", "is for the Director team only, so its pages can only move into another category only the Director team sees", ""},
		{"into itself refused", owner, ActionDelete, map[string]any{"category": "Lore", "move_pages_to": "lore"},
			"", "can't move its pages into itself", ""},
		{"kinds refused", owner, ActionDelete, map[string]any{"category": "Places", "move_pages_to": "Lore"},
			"", "Places (location) has kinds of its own (Cities); delete or move its kinds first", ""},
		{"a kind itself can go", owner, ActionDelete, map[string]any{"category": "Cities", "move_pages_to": "Places"},
			"Cities (city) is deleted; its 1 page and its folders move into Places.", "", ""},
		{"unknown name", owner, ActionDelete, map[string]any{"category": "Gods"}, "", `no category called "Gods"`, ""},
		{"unknown target", owner, ActionDelete, map[string]any{"category": "Lore", "move_pages_to": "Gods"}, "", `no category called "Gods"`, ""},
		{"no category", owner, ActionDelete, map[string]any{}, "", "name the category to delete in `category:`", ""},
		{"a co-DM is not the owner", Actor{UserID: "codm", Role: 3}, ActionDelete, map[string]any{"category": "adventures-2"}, "", "only the campaign's owner", ""},
		{"the owner reading as a player is not the owner", Actor{UserID: "own", Role: 1}, ActionDelete, map[string]any{"category": "adventures-2"}, "", "only the campaign's owner", ""},
		{"create refused", owner, ActionCreate, map[string]any{"category": "Gods"}, "", "made and changed in Customize › Categories", ""},
		{"update refused", owner, ActionUpdate, map[string]any{"category": "Lore"}, "", "only deletes one, with `action: delete`", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := Record{Kind: "category", Action: tc.action, Fields: tc.fields}
			if n, ok := tc.fields["name"].(string); ok {
				r.Name = n
			}
			p := CategoryKind{Svc: categoriesFake()}.Plan(context.Background(), "c", tc.actor, r)
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) {
					t.Fatalf("error %q, want %q", p.Error, tc.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.summary) {
				t.Fatalf("plan %+v, want summary with %q", p, tc.summary)
			}
			if got := strings.Join(p.Warnings, "|"); !strings.Contains(got, tc.warn) {
				t.Errorf("warnings %q, want %q", got, tc.warn)
			}
		})
	}
}

func TestCategoryKind_Apply(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name    string
		actor   Actor
		fields  map[string]any
		deleted []int
		merged  [][2]int
		wantErr bool
	}{
		{"empty category is deleted", owner, map[string]any{"category": "adventures-2"}, []int{2}, nil, false},
		{"empty category with a target is still only deleted", owner, map[string]any{"category": "adventures-2", "move_pages_to": "adventures"}, []int{2}, nil, false},
		{"pages are merged", owner, map[string]any{"category": "Lore", "move_pages_to": "Secrets"}, nil, [][2]int{{3, 4}}, false},
		{"a refused plan writes nothing", owner, map[string]any{"category": "adventures", "move_pages_to": "Lore"}, nil, nil, true},
		{"a co-DM writes nothing", Actor{UserID: "codm", Role: 3}, map[string]any{"category": "adventures-2"}, nil, nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := categoriesFake()
			err := CategoryKind{Svc: f}.Apply(context.Background(), "c", tc.actor, Record{Kind: "category", Action: ActionDelete, Fields: tc.fields})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err %v, want error %v", err, tc.wantErr)
			}
			if len(f.deleted) != len(tc.deleted) || len(tc.deleted) > 0 && f.deleted[0] != tc.deleted[0] {
				t.Errorf("deleted %v, want %v", f.deleted, tc.deleted)
			}
			if len(f.merged) != len(tc.merged) || len(tc.merged) > 0 && f.merged[0] != tc.merged[0] {
				t.Errorf("merged %v, want %v", f.merged, tc.merged)
			}
		})
	}
}

func TestCategoryKind_Export(t *testing.T) {
	k := CategoryKind{Svc: categoriesFake()}
	got, err := k.Export(context.Background(), "c", Actor{UserID: "own", Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"- Adventures (slug: `adventures`): 11 pages, only the Director team sees it\n",
		"- Adventures (slug: `adventures-2`): no pages\n",
		"- Lore (slug: `lore`): 4 pages, 1 in the Trash\n",
		"- Cities (slug: `city`), a kind of Places: 1 page\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export lacks %q:\n%s", want, got)
		}
	}
	for _, a := range []Actor{{UserID: "codm", Role: 3}, {UserID: "own", Role: 1}} {
		if got, _ := k.Export(context.Background(), "c", a); got != "" {
			t.Errorf("%+v sees the categories: %q", a, got)
		}
	}
}
