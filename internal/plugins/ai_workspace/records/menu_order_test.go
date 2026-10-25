package records

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// fakeMenu orders like the sidebar: listed categories take the listed ones'
// places within their section, in the listed order.
type fakeMenu struct {
	menu    []MenuCategory
	owner   string
	ordered [][]int
}

func (f *fakeMenu) IsOwner(_ context.Context, _, userID string) (bool, error) {
	return userID == f.owner, nil
}
func (f *fakeMenu) MenuCategories(context.Context, string) ([]MenuCategory, error) {
	return f.menu, nil
}
func (f *fakeMenu) PreviewOrder(_ context.Context, _ string, ids []int) ([]MenuCategory, error) {
	rank := map[int]int{}
	for i, id := range ids {
		rank[id] = i
	}
	out := append([]MenuCategory(nil), f.menu...)
	bySection := map[string][]int{}
	for i, c := range f.menu {
		if _, ok := rank[c.ID]; ok {
			bySection[c.Section] = append(bySection[c.Section], i)
		}
	}
	for _, slots := range bySection {
		var moved []MenuCategory
		for _, s := range slots {
			moved = append(moved, f.menu[s])
		}
		for a := 0; a < len(moved); a++ {
			for b := a + 1; b < len(moved); b++ {
				if rank[moved[b].ID] < rank[moved[a].ID] {
					moved[a], moved[b] = moved[b], moved[a]
				}
			}
		}
		for j, s := range slots {
			out[s] = moved[j]
		}
	}
	return out, nil
}
func (f *fakeMenu) OrderCategories(_ context.Context, _ string, ids []int) error {
	f.ordered = append(f.ordered, ids)
	return nil
}

func menuFake() *fakeMenu {
	return &fakeMenu{owner: "own", menu: []MenuCategory{
		{ID: 1, Name: "Character", Plural: "Characters", Slug: "character", Section: "Categories"},
		{ID: 2, Name: "Location", Plural: "Places", Slug: "location", Section: "Categories"},
		{ID: 3, Name: "Lore", Plural: "Lore", Slug: "lore", Section: "Categories"},
		{ID: 4, Name: "Adventure", Plural: "Adventures", Slug: "adventure", Section: "Pinned"},
	}}
}

func TestMenuOrderKind_Plan(t *testing.T) {
	owner := Actor{UserID: "own", Role: 3}
	tests := []struct {
		name    string
		actor   Actor
		action  string
		list    any
		summary string
		err     string
		warn    string
	}{
		{"reorders", owner, ActionUpdate, []any{"Lore", "characters"}, "the menu's categories become: Lore, Places, Characters, Adventures (now: Characters, Places, Lore, Adventures)", "", ""},
		{"create means the same", owner, ActionCreate, []any{"Places", "Characters"}, "become: Places, Characters, Lore", "", ""},
		{"matched by singular or slug", owner, ActionUpdate, []any{"lore", "Location"}, "become: Characters, Lore, Places", "", ""},
		{"across sections warns", owner, ActionUpdate, []any{"Adventures", "Lore", "Characters"}, "become: Lore, Places, Characters, Adventures", "", "different sections of the menu (Categories, Pinned)"},
		{"already in order", owner, ActionUpdate, []any{"Characters", "Places"}, "", "already in this order", ""},
		{"unknown name", owner, ActionUpdate, []any{"Gods"}, "", `no category called "Gods"`, ""},
		{"listed twice", owner, ActionUpdate, []any{"Lore", "lore"}, "", `"lore" is listed twice`, ""},
		{"no list", owner, ActionUpdate, nil, "", "list the categories in `categories:`", ""},
		{"a co-DM is not the owner", Actor{UserID: "codm", Role: 3}, ActionUpdate, []any{"Lore"}, "", "only the campaign's owner", ""},
		{"no removing", owner, ActionDelete, []any{"Lore"}, "", "can't be removed", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := menuFake()
			r := Record{Kind: "menu-order", Action: tc.action, Fields: map[string]any{}}
			if tc.list != nil {
				r.Fields["categories"] = tc.list
			}
			p := MenuOrderKind{Svc: f}.Plan(context.Background(), "c", tc.actor, r)
			if tc.err != "" {
				if !strings.Contains(p.Error, tc.err) {
					t.Fatalf("error %q, want %q", p.Error, tc.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tc.summary) {
				t.Fatalf("plan %+v, want summary with %q", p, tc.summary)
			}
			if got := strings.Join(p.Warnings, "|"); tc.warn == "" && got != "" || !strings.Contains(got, tc.warn) {
				t.Errorf("warnings %q, want %q", got, tc.warn)
			}
		})
	}
}

func TestMenuOrderKind_ApplyAndExport(t *testing.T) {
	f := menuFake()
	k := MenuOrderKind{Svc: f}
	owner := Actor{UserID: "own", Role: 3}
	r := Record{Kind: "menu-order", Action: ActionUpdate, Fields: map[string]any{"categories": []any{"Lore", "Characters"}}}
	if err := k.Apply(context.Background(), "c", owner, r); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f.ordered, [][]int{{3, 1}}) {
		t.Errorf("ordered %v", f.ordered)
	}
	if err := k.Apply(context.Background(), "c", Actor{UserID: "codm", Role: 3}, r); err == nil || len(f.ordered) != 1 {
		t.Errorf("a co-DM's apply went through: %v", err)
	}

	got, err := k.Export(context.Background(), "c", owner)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\nkind: menu-order\ncategories:\n  - \"Characters\"\n  - \"Places\"\n  - \"Lore\"\n  - \"Adventures\"\n---\n\n"
	if got != want {
		t.Errorf("export\n%s\nwant\n%s", got, want)
	}
	if got, _ := k.Export(context.Background(), "c", Actor{UserID: "codm", Role: 3}); got != "" {
		t.Errorf("a co-DM's export: %q", got)
	}
	// What players see: the owner's id with a player's role.
	if got, _ := k.Export(context.Background(), "c", Actor{UserID: "own", Role: 1}); got != "" {
		t.Errorf("a players-only export lists the menu: %q", got)
	}
	if p := k.Plan(context.Background(), "c", Actor{UserID: "own", Role: 1}, r); !strings.Contains(p.Error, "only the campaign's owner") {
		t.Errorf("a player-role plan: %+v", p)
	}
}

func TestLookups_MenuOrder(t *testing.T) {
	l := &Lookups{Menu: menuFake()}
	ctx := context.Background()
	if got := l.MenuOrder(ctx, "c", Actor{UserID: "own", Role: 3}); got != "Characters, Places, Lore, Adventures" {
		t.Errorf("owner: %q", got)
	}
	for _, a := range []Actor{{UserID: "own", Role: 1}, {UserID: "codm", Role: 3}} {
		if got := l.MenuOrder(ctx, "c", a); got != "" {
			t.Errorf("%+v sees the menu: %q", a, got)
		}
	}
	if got := (&Lookups{}).MenuOrder(ctx, "c", Actor{UserID: "own", Role: 3}); got != "" {
		t.Errorf("no menu wired: %q", got)
	}
}
