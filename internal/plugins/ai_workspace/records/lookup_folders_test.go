package records

import (
	"context"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// racesWorld is the shape behind "Fairies (in folder Races)" failing to
// delete as `category: Races`: Races is a sub-category of Lore, and the
// Races folder its pages sit in belongs to Lore. Adventures is Director-only.
func racesWorld() (*fakeFolders, *fakePages) {
	adv := entities.PresetCategoryAdventures
	f := &fakeFolders{
		cats: []FolderCategory{
			{ID: 1, Name: "Lore", Plural: "Lore", Slug: "lore"},
			{ID: 2, Name: "Races", Plural: "Races", Slug: "races", ParentID: 1},
			{ID: 3, Name: "Adventure", Plural: "Adventures", Slug: "adventure"},
			{ID: 4, Name: "Location", Plural: "Locations", Slug: "location"},
		},
		folders: []FolderRef{
			{ID: "races", Name: "Races", CategoryID: 1},
			{ID: "fey", Name: "Fey", CategoryID: 1, ParentID: "races"},
			{ID: "gods", Name: "Gods", CategoryID: 1},
			{ID: "plots", Name: "Secret Plots", CategoryID: 3},
		},
	}
	races := "races"
	fairies := page("fairies", "Fairies", "Races", false)
	fairies.EntityTypeID, fairies.ParentNodeID = 2, &races
	hidden := page("drow", "Drow", "Races", true)
	hidden.EntityTypeID, hidden.ParentNodeID = 2, &races
	p := &fakePages{
		all: []entities.Entity{fairies, hidden},
		types: []entities.EntityType{
			{ID: 1, Name: "Lore"}, {ID: 2, Name: "Races"},
			{ID: 3, Name: "Adventure", PresetCategory: &adv}, {ID: 4, Name: "Location"},
		},
	}
	return f, p
}

func TestLookupFolders(t *testing.T) {
	f, p := racesWorld()
	tests := []struct {
		name    string
		l       *Lookups
		a       Actor
		fields  map[string]any
		want    []string
		notWant []string
		err     string
	}{
		{"nested tree with empty folders and page counts", &Lookups{FolderTree: f, Pages: p}, owner, nil,
			[]string{"## Folders (4)", "### Lore (3 folders)", "- Races (2 pages)\n  - Fey (no pages)\n- Gods (no pages)", "### Adventure (1 folder)", "Secret Plots",
				"No folders: Races (a sub-category of Lore: its pages can sit in Lore's folders), Location."}, nil, ""},
		{"one category", &Lookups{FolderTree: f, Pages: p}, owner, map[string]any{"category": "lore"},
			[]string{"### Lore (3 folders)", "- Races"}, []string{"Adventure", "No folders"}, ""},
		{"a sub-category says where its pages' folders are", &Lookups{FolderTree: f, Pages: p}, owner, map[string]any{"category": "Races"},
			[]string{"Races has no folders (a sub-category of Lore: its pages can sit in Lore's folders)."}, nil, ""},
		{"a player sees no Director-only category and counts only visible pages", &Lookups{FolderTree: f, Pages: p}, player, nil,
			[]string{"- Races (1 page)"}, []string{"Adventure", "Secret Plots"}, ""},
		{"a player can't name a Director-only category", &Lookups{FolderTree: f, Pages: p}, player, map[string]any{"category": "Adventures"},
			nil, nil, "no category called"},
		{"unknown category", &Lookups{FolderTree: f, Pages: p}, owner, map[string]any{"category": "Spells"}, nil, nil, "no category called"},
		{"not wired", &Lookups{Pages: p}, owner, nil, nil, nil, "can't be looked up"},
		{"a player's answer needs the categories' visibility", &Lookups{FolderTree: f}, player, nil, nil, nil, "can't be looked up"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ans := answerOne(tt.l, tt.a, "folders", tt.fields)
			if tt.err != "" {
				if !strings.Contains(ans.Error, tt.err) {
					t.Fatalf("error = %q, want %q", ans.Error, tt.err)
				}
				return
			}
			if ans.Error != "" {
				t.Fatalf("error = %q", ans.Error)
			}
			for _, w := range tt.want {
				if !strings.Contains(ans.Text, w) {
					t.Errorf("missing %q in:\n%s", w, ans.Text)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(ans.Text, w) {
					t.Errorf("unexpected %q in:\n%s", w, ans.Text)
				}
			}
		})
	}
}

func TestFolderForest_LoopsAndOrphans(t *testing.T) {
	tests := []struct {
		name string
		in   []FolderRef
		want int // folders reached
	}{
		{"orphan sits at the top", []FolderRef{{ID: "a", ParentID: "gone"}}, 1},
		{"a loop is listed once each", []FolderRef{{ID: "a", ParentID: "b"}, {ID: "b", ParentID: "a"}}, 2},
		{"self parent", []FolderRef{{ID: "a", ParentID: "a"}}, 1},
	}
	var count func(ns []folderNode) int
	count = func(ns []folderNode) int {
		n := len(ns)
		for _, k := range ns {
			n += count(k.kids)
		}
		return n
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := count(folderForest(tt.in)); got != tt.want {
				t.Errorf("reached %d folders, want %d", got, tt.want)
			}
		})
	}
}

func TestPageIndex_FoldersOfAnotherCategory(t *testing.T) {
	f, p := racesWorld()
	tests := []struct {
		name    string
		a       Actor
		want    []string
		notWant []string
	}{
		{"a Director reads the folder's category and every folder", owner,
			[]string{"Fairies (in folder Races of Lore)", "- **Folders** (sub-folders in brackets): Lore: Races (Fey), Gods; Adventure: Secret Plots"}, nil},
		{"a player gets neither", player,
			[]string{"Fairies (in folder Races)"}, []string{"of Lore", "**Folders**", "Secret Plots"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &Lookups{FolderTree: f, Pages: p, Folders: fakeFolderNames{"races": "Races", "fey": "Fey", "gods": "Gods", "plots": "Secret Plots"}}
			idx := l.PageIndex(context.Background(), camp, tt.a)
			for _, w := range tt.want {
				if !strings.Contains(idx, w) {
					t.Errorf("missing %q in:\n%s", w, idx)
				}
			}
			for _, w := range tt.notWant {
				if strings.Contains(idx, w) {
					t.Errorf("unexpected %q in:\n%s", w, idx)
				}
			}
		})
	}
}

// The refusal for a missing folder names what the category does have, and
// where a folder of that name lives instead.
func TestFolderKind_MissingNamesTheFolders(t *testing.T) {
	f, _ := racesWorld()
	tests := []struct {
		name string
		r    Record
		want []string
	}{
		{"the issue's delete", folderRec("delete", "Races", map[string]any{"category": "Races"}),
			[]string{`no folder called "Races" in Races`, "Races has no folders", `A folder called "Races" is in Lore; give that category as` + " `category:`"}},
		{"a category with folders lists them", folderRec("update", "Titans", map[string]any{"category": "Lore", "rename_to": "X"}),
			[]string{`no folder called "Titans" in Lore; its folders are "Races", "Fey", "Gods"`}},
		{"a missing parent", folderRec("create", "Elves", map[string]any{"category": "Lore", "parent": "Secret Plots"}),
			[]string{`no folder called "Secret Plots" in Lore to nest it in; its folders are`, "is in Adventure"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := FolderKind{Svc: f}.Plan(context.Background(), camp, owner, tt.r)
			for _, w := range tt.want {
				if !strings.Contains(p.Error, w) {
					t.Errorf("error = %q, want %q", p.Error, w)
				}
			}
		})
	}
}
