package records

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// folderTreeDepth bounds a walk down the folder tree, so a stored loop of
// folders can never recurse forever.
const folderTreeDepth = 50

// folderNode is one folder with the folders nested in it.
type folderNode struct {
	f    FolderRef
	kids []folderNode
}

// folderForest nests one category's folders. A folder whose parent is not in
// the list sits at the top level; folders caught in a stored loop are listed
// at the top level too, once each, so none goes missing.
func folderForest(in []FolderRef) []folderNode {
	byID := make(map[string]bool, len(in))
	for _, f := range in {
		byID[f.ID] = true
	}
	kids := map[string][]FolderRef{}
	var roots []FolderRef
	for _, f := range in {
		if f.ParentID != "" && byID[f.ParentID] && f.ParentID != f.ID {
			kids[f.ParentID] = append(kids[f.ParentID], f)
		} else {
			roots = append(roots, f)
		}
	}
	placed := map[string]bool{}
	var grow func(f FolderRef, depth int) folderNode
	grow = func(f FolderRef, depth int) folderNode {
		placed[f.ID] = true
		n := folderNode{f: f}
		if depth >= folderTreeDepth {
			return n
		}
		for _, k := range kids[f.ID] {
			if !placed[k.ID] {
				n.kids = append(n.kids, grow(k, depth+1))
			}
		}
		return n
	}
	var out []folderNode
	for _, r := range roots {
		out = append(out, grow(r, 0))
	}
	for _, f := range in {
		if !placed[f.ID] {
			out = append(out, grow(f, 0))
		}
	}
	return out
}

// folderCats is the categories the Actor may know exist, in the folder
// service's order. A Director gets every one; anyone else only those the
// sidebar shows them, whose folder names the sidebar shows them too.
func (run *lookupRun) folderCats() ([]FolderCategory, error) {
	cats, err := run.l.FolderTree.Categories(run.ctx, run.campaignID)
	if err != nil || run.director() {
		return cats, err
	}
	if run.l.Pages == nil {
		return nil, badRequestf("folders can't be looked up on this server")
	}
	types, err := run.l.Pages.GetEntityTypes(run.ctx, run.campaignID)
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	for _, t := range entities.TypesForViewer(types, run.a.Role) {
		seen[t.ID] = true
	}
	out := cats[:0:0]
	for _, c := range cats {
		if seen[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

// foldersLookup lists each category's sidebar folders as a nested tree, empty
// ones included, with how many visible pages each holds. Folder blocks name
// a folder by category and name, and empty folders never show in the page
// index, so this is where an AI finds what it may rename, move or remove.
func (run *lookupRun) foldersLookup(category string) (LookupAnswer, error) {
	chip := "Folders"
	if category != "" {
		chip = "Folders · " + category
	}
	if run.l.FolderTree == nil {
		return LookupAnswer{Chip: chip}, badRequestf("folders can't be looked up on this server")
	}
	cats, err := run.folderCats()
	if err != nil {
		return LookupAnswer{Chip: chip}, err
	}
	all, err := run.l.FolderTree.List(run.ctx, run.campaignID)
	if err != nil {
		return LookupAnswer{Chip: chip}, err
	}
	catByID := make(map[int]FolderCategory, len(cats))
	for _, c := range cats {
		catByID[c.ID] = c
	}
	if category != "" {
		var found []FolderCategory
		for _, c := range cats {
			if sameName(c.Name, category) || sameName(c.Plural, category) || sameName(c.Slug, category) {
				found = append(found, c)
			}
		}
		switch len(found) {
		case 0:
			return LookupAnswer{Chip: chip}, badRequestf("no category called %s", quote(category))
		case 1:
			cats = found
		default:
			return LookupAnswer{Chip: chip}, badRequestf("more than one category is called %s", quote(category))
		}
	}
	// Pages are counted as the Actor sees them, so a folder holding only
	// hidden pages reads as empty to a player, as it does in the sidebar.
	count := map[string]int{}
	if run.l.Pages != nil {
		if pages, err := run.visiblePages(); err == nil {
			for _, p := range pages {
				if p.ParentNodeID != nil {
					count[*p.ParentNodeID]++
				}
			}
		}
	}
	byCat := map[int][]FolderRef{}
	for _, f := range all {
		byCat[f.CategoryID] = append(byCat[f.CategoryID], f)
	}
	sub := func(c FolderCategory) string {
		if p, ok := catByID[c.ParentID]; ok && c.ParentID != 0 {
			return " (a sub-category of " + p.Name + ": its pages can sit in " + p.Name + "'s folders)"
		}
		return ""
	}
	var b strings.Builder
	var empty []string
	total := 0
	for _, c := range cats {
		in := byCat[c.ID]
		if len(in) == 0 {
			empty = append(empty, c.Name+sub(c))
			continue
		}
		total += len(in)
		fmt.Fprintf(&b, "### %s (%s)%s\n\n", c.Name, plural(len(in), "folder", "folders"), sub(c))
		var write func(ns []folderNode, depth int)
		write = func(ns []folderNode, depth int) {
			for _, n := range ns {
				fmt.Fprintf(&b, "%s- %s (%s)\n", strings.Repeat("  ", depth), n.f.Name, folderPages(count[n.f.ID]))
				write(n.kids, depth+1)
			}
		}
		write(folderForest(in), 0)
		b.WriteString("\n")
	}
	head := fmt.Sprintf("## Folders (%d)\n\n", total)
	if category != "" {
		chip = cats[0].Name + " folders · " + strconv.Itoa(total)
	} else {
		chip = "Folders · " + strconv.Itoa(total)
	}
	switch {
	case category != "" && total == 0:
		b.WriteString(cats[0].Name + " has no folders" + sub(cats[0]) + ".\n")
	case len(empty) > 0:
		b.WriteString("No folders: " + strings.Join(empty, ", ") + ".\n")
	}
	return LookupAnswer{Chip: chip, Text: head + b.String()}, nil
}

// folderPages says how many pages a folder holds.
func folderPages(n int) string {
	if n == 0 {
		return "no pages"
	}
	return plural(n, "page", "pages")
}

// folderOutline is the page index's line of every category's folders,
// nested in brackets, for a Director: empty folders never show beside a
// page, and a folder block needs the folder's own category. "" when there
// are none or folders can't be read.
func (run *lookupRun) folderOutline() string {
	if run.l.FolderTree == nil || !run.director() {
		return ""
	}
	cats, err := run.l.FolderTree.Categories(run.ctx, run.campaignID)
	if err != nil {
		return ""
	}
	all, err := run.l.FolderTree.List(run.ctx, run.campaignID)
	if err != nil {
		return ""
	}
	byCat := map[int][]FolderRef{}
	for _, f := range all {
		byCat[f.CategoryID] = append(byCat[f.CategoryID], f)
	}
	var join func(ns []folderNode) string
	join = func(ns []folderNode) string {
		parts := make([]string, 0, len(ns))
		for _, n := range ns {
			s := n.f.Name
			if len(n.kids) > 0 {
				s += " (" + join(n.kids) + ")"
			}
			parts = append(parts, s)
		}
		return strings.Join(parts, ", ")
	}
	var groups []string
	for _, c := range cats {
		if in := byCat[c.ID]; len(in) > 0 {
			groups = append(groups, c.Name+": "+join(folderForest(in)))
		}
	}
	if len(groups) == 0 {
		return ""
	}
	return capText("- **Folders** (sub-folders in brackets): "+strings.Join(groups, "; ")+"\n", 2*1024)
}

// folderPlace is where a folder lives, for a page's "in folder" label.
type folderPlace struct {
	name, category string
	categoryID     int
}

// folderPlaces maps folder ids to their names and, for a Director, their
// categories; empty when folders are not wired.
func (run *lookupRun) folderPlaces() map[string]folderPlace {
	out := map[string]folderPlace{}
	if run.l.FolderTree != nil && run.director() {
		cats, cerr := run.l.FolderTree.Categories(run.ctx, run.campaignID)
		all, ferr := run.l.FolderTree.List(run.ctx, run.campaignID)
		if cerr == nil && ferr == nil {
			catName := make(map[int]string, len(cats))
			for _, c := range cats {
				catName[c.ID] = c.Name
			}
			for _, f := range all {
				out[f.ID] = folderPlace{name: f.Name, category: catName[f.CategoryID], categoryID: f.CategoryID}
			}
			return out
		}
	}
	for id, name := range run.folderNames() {
		out[id] = folderPlace{name: name}
	}
	return out
}
