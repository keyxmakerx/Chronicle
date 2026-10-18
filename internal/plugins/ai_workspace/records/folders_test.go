package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type fakeFolders struct {
	cats    []FolderCategory
	folders []FolderRef
	calls   []string
}

func (f *fakeFolders) Categories(context.Context, string) ([]FolderCategory, error) {
	return f.cats, nil
}
func (f *fakeFolders) List(context.Context, string) ([]FolderRef, error) { return f.folders, nil }
func (f *fakeFolders) Create(_ context.Context, _ string, cat int, name, parent string) error {
	f.calls = append(f.calls, "create:"+name+":"+parent)
	f.folders = append(f.folders, FolderRef{ID: "new-" + name, Name: name, CategoryID: cat, ParentID: parent})
	return nil
}
func (f *fakeFolders) Rename(_ context.Context, _, id, name string) error {
	f.calls = append(f.calls, "rename:"+id+":"+name)
	return nil
}
func (f *fakeFolders) Move(_ context.Context, _, id string, p *string) error {
	to := "top"
	if p != nil {
		to = *p
	}
	f.calls = append(f.calls, "move:"+id+":"+to)
	return nil
}
func (f *fakeFolders) Delete(_ context.Context, _, id string) error {
	f.calls = append(f.calls, "delete:"+id)
	return nil
}

func folderFake() *fakeFolders {
	return &fakeFolders{
		cats: []FolderCategory{
			{ID: 1, Name: "Lore", Plural: "Lore", Slug: "lore"},
			{ID: 2, Name: "Location", Plural: "Locations", Slug: "location"},
			{ID: 3, Name: "Faction", Plural: "Groups", Slug: "faction"},
			{ID: 4, Name: "Group", Plural: "Groups", Slug: "group"},
		},
		folders: []FolderRef{
			{ID: "gods", Name: "Gods", CategoryID: 1},
			{ID: "elder", Name: "Elder Gods", CategoryID: 1, ParentID: "gods"},
			{ID: "cosmos", Name: "Cosmology", CategoryID: 1},
			{ID: "twin1", Name: "Twin", CategoryID: 1},
			{ID: "twin2", Name: "twin", CategoryID: 1},
			{ID: "towns", Name: "Gods", CategoryID: 2},
		},
	}
}

func folderRec(action, name string, fields map[string]any) Record {
	if fields == nil {
		fields = map[string]any{}
	}
	return Record{Kind: "folder", Action: action, Name: name, Fields: fields}
}

func TestFolderKind_Plan(t *testing.T) {
	owner := Actor{UserID: "u", Role: 3}
	player := Actor{UserID: "p", Role: 1}
	tests := []struct {
		name    string
		r       Record
		a       Actor
		summary string
		err     string
	}{
		{"create", folderRec("create", "Titans", map[string]any{"category": "Lore"}), owner, `makes the folder "Titans" in Lore`, ""},
		{"create under a parent", folderRec("create", "Titans", map[string]any{"category": "lore", "parent": "gods"}), owner, `under "gods" in Lore`, ""},
		{"same name in another category is fine", folderRec("create", "Gods", map[string]any{"category": "Faction"}), owner, "makes the folder", ""},
		{"create needs the category", folderRec("create", "Titans", nil), owner, "", "needs `category:`"},
		{"create over an existing name", folderRec("create", "GODS", map[string]any{"category": "Lore"}), owner, "", "already exists in Lore"},
		{"unknown category", folderRec("create", "X", map[string]any{"category": "Spells"}), owner, "", "no category called"},
		{"ambiguous category", folderRec("create", "X", map[string]any{"category": "Groups"}), owner, "", "more than one category"},
		{"unknown parent", folderRec("create", "X", map[string]any{"category": "Lore", "parent": "Nope"}), owner, "", "no folder called"},
		{"parent from another category", folderRec("create", "X", map[string]any{"category": "Lore", "parent": "Towns"}), owner, "", "no folder called"},
		{"rename", folderRec("update", "Cosmology", map[string]any{"category": "Lore", "rename_to": "The Heavens"}), owner, `renames it to "The Heavens"`, ""},
		{"rename onto another folder", folderRec("update", "Cosmology", map[string]any{"category": "Lore", "rename_to": "gods"}), owner, "", "already exists"},
		{"move", folderRec("update", "Cosmology", map[string]any{"category": "Lore", "parent": "Gods"}), owner, `moves it into "Gods"`, ""},
		{"lift", folderRec("update", "Elder Gods", map[string]any{"category": "Lore", "parent": "none"}), owner, "top level of Lore", ""},
		{"into itself", folderRec("update", "Gods", map[string]any{"category": "Lore", "parent": "Gods"}), owner, "", "inside itself"},
		{"update with nothing", folderRec("update", "Gods", map[string]any{"category": "Lore"}), owner, "", "nothing to change"},
		{"update missing", folderRec("update", "Nope", map[string]any{"category": "Lore", "rename_to": "X"}), owner, "", "no folder called"},
		{"ambiguous folder", folderRec("delete", "Twin", map[string]any{"category": "Lore"}), owner, "", "more than one folder"},
		{"delete says what happens to the contents", folderRec("delete", "Gods", map[string]any{"category": "Lore"}), owner, "pages move to the top level of Lore and its sub-folders move up one level; no page is deleted", ""},
		{"players cannot", folderRec("create", "X", map[string]any{"category": "Lore"}), player, "", "only the owner or a co-DM"},
		{"needs a name", folderRec("create", "", map[string]any{"category": "Lore"}), owner, "", "needs a name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := FolderKind{Svc: folderFake()}.Plan(context.Background(), "c1", tt.a, tt.r)
			if tt.err != "" {
				if !strings.Contains(p.Error, tt.err) {
					t.Fatalf("error = %q, want %q", p.Error, tt.err)
				}
				return
			}
			if p.Error != "" || !strings.Contains(p.Summary, tt.summary) {
				t.Fatalf("plan = %+v, want summary containing %q", p, tt.summary)
			}
		})
	}
}

func TestFolderKind_Apply(t *testing.T) {
	owner := Actor{UserID: "u", Role: 3}
	tests := []struct {
		name string
		r    Record
		want []string
	}{
		{"create top level", folderRec("create", "Titans", map[string]any{"category": "Lore"}), []string{"create:Titans:"}},
		{"create nested", folderRec("create", "Titans", map[string]any{"category": "Lore", "parent": "gods"}), []string{"create:Titans:gods"}},
		{"rename", folderRec("update", "Cosmology", map[string]any{"category": "Lore", "rename_to": "Heavens"}), []string{"rename:cosmos:Heavens"}},
		{"move then rename", folderRec("update", "Cosmology", map[string]any{"category": "Lore", "parent": "Gods", "rename_to": "Heavens"}), []string{"move:cosmos:gods", "rename:cosmos:Heavens"}},
		{"lift", folderRec("update", "Elder Gods", map[string]any{"category": "Lore", "parent": "none"}), []string{"move:elder:top"}},
		{"delete", folderRec("delete", "Cosmology", map[string]any{"category": "Lore"}), []string{"delete:cosmos"}},
		{"delete picks the folder in the named category", folderRec("delete", "Gods", map[string]any{"category": "Location"}), []string{"delete:towns"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := folderFake()
			if err := (FolderKind{Svc: f}).Apply(context.Background(), "c1", owner, tt.r); err != nil {
				t.Fatal(err)
			}
			if strings.Join(f.calls, ",") != strings.Join(tt.want, ",") {
				t.Errorf("calls = %v, want %v", f.calls, tt.want)
			}
		})
	}
	t.Run("a refused plan writes nothing", func(t *testing.T) {
		f := folderFake()
		err := FolderKind{Svc: f}.Apply(context.Background(), "c1", Actor{Role: 1}, folderRec("delete", "Gods", map[string]any{"category": "Lore"}))
		if err == nil || len(f.calls) != 0 {
			t.Fatalf("err = %v, calls = %v", err, f.calls)
		}
	})
}

// A folder made earlier in a paste can hold a folder made later in it,
// though neither exists when the review plans them.
func TestRegistry_PlanAll_FoldersMadeInThePaste(t *testing.T) {
	reg := NewRegistry(FolderKind{Svc: folderFake()})
	recs := []Record{
		folderRec("create", "Titans", map[string]any{"category": "Lore"}),
		folderRec("create", "Primordials", map[string]any{"category": "Lore", "parent": "titans"}),
		folderRec("create", "Titans", map[string]any{"category": "Lore"}),
	}
	_, plans := reg.PlanAll(context.Background(), "c1", Actor{UserID: "u", Role: 3}, recs)
	if plans[0].Error != "" || plans[1].Error != "" {
		t.Fatalf("plans = %+v", plans)
	}
	if !strings.Contains(plans[2].Error, "already exists") {
		t.Errorf("a second Titans must be refused, got %+v", plans[2])
	}
}

func TestFolderKind_Export(t *testing.T) {
	got, err := FolderKind{Svc: folderFake()}.Export(context.Background(), "c1", Actor{UserID: "u", Role: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"kind: folder\ncategory: \"Lore\"\nname: \"Gods\"\n---",
		"name: \"Elder Gods\"\nparent: \"Gods\"\n",
		"category: \"Location\"\nname: \"Gods\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("export missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, `name: "Gods"`) > strings.Index(got, `name: "Elder Gods"`) {
		t.Error("a parent must be written before its sub-folder")
	}
	if out, _ := (FolderKind{Svc: folderFake()}).Export(context.Background(), "c1", Actor{Role: 1}); out != "" {
		t.Error("players get no folder export")
	}
}

type fakeFolderNames map[string]string

func (f fakeFolderNames) FolderNames(context.Context, string) (map[string]string, error) {
	return f, nil
}

func TestLookups_PageIndexShowsFolder(t *testing.T) {
	gods, anchor := "gods", "anchor"
	mk := func(id, name string, parent, node *string) entities.Entity {
		e := page(id, name, "Lore", false)
		e.ParentID, e.ParentNodeID = parent, node
		return e
	}
	pages := &fakePages{all: []entities.Entity{
		mk("zeus", "Zeus", nil, &gods),
		mk("hera", "Hera", nil, nil),
		mk("anchor", "Anchor", nil, nil),
		mk("rope", "Rope", &anchor, nil),
	}}
	tests := []struct {
		name    string
		folders aiexport.FolderNamer
		want    string
		notWant string
	}{
		{"filed page names its folder", fakeFolderNames{"gods": "Gods"}, "Zeus (in folder Gods)", ""},
		{"unfiled page is plain", fakeFolderNames{"gods": "Gods"}, "Hera", "Hera (in"},
		{"a parent page still reads as before", fakeFolderNames{"gods": "Gods"}, "Rope (in Anchor)", ""},
		{"folders not wired", nil, "Zeus", "in folder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := (&Lookups{Pages: pages, Folders: tt.folders}).PageIndex(context.Background(), camp, owner)
			if !strings.Contains(idx, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, idx)
			}
			if tt.notWant != "" && strings.Contains(idx, tt.notWant) {
				t.Errorf("unexpected %q in:\n%s", tt.notWant, idx)
			}
		})
	}
}
