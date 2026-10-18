package records

import (
	"context"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// FoldersAPI is the entities folder service as the folder kind needs it,
// adapted in app wiring so this package does not import the plugin. The
// service keeps the rules (same campaign, no loops, what a delete leaves).
type FoldersAPI interface {
	Categories(ctx context.Context, campaignID string) ([]FolderCategory, error)
	// List returns every folder in the campaign, all categories together.
	List(ctx context.Context, campaignID string) ([]FolderRef, error)
	// Create makes a folder; parentID "" is the top level of the category.
	Create(ctx context.Context, campaignID string, categoryID int, name, parentID string) error
	Rename(ctx context.Context, campaignID, id, name string) error
	// Move puts the folder under parentID, or at the top level when nil.
	Move(ctx context.Context, campaignID, id string, parentID *string) error
	Delete(ctx context.Context, campaignID, id string) error
}

// FolderCategory is a category a folder can live in.
type FolderCategory struct {
	ID                 int
	Name, Plural, Slug string
}

// FolderRef is a stored folder. ParentID is set only when the parent is
// another folder.
type FolderRef struct {
	ID, Name, ParentID string
	CategoryID         int
}

// FolderKind makes, renames, moves and removes the sidebar folders inside a
// category, matched by `category` and `name`. A page is filed in a folder
// with `folder:` on the page's own block.
type FolderKind struct {
	Svc FoldersAPI
}

func (FolderKind) Name() string  { return "folder" }
func (FolderKind) Label() string { return "Folder" }
func (FolderKind) Doc() string {
	return "A sidebar folder inside a category (for example \"Gods\" and \"Cosmology\" inside Lore), matched by `category` and `name`, ignoring capitals; two folders in a category never share a name. Keys: `category` (the category's name, required), `parent` (another folder of the same category to nest it in; `none` on an update moves it to the top level), `rename_to`. A folder holds no text of its own. To file a page in a folder, write `folder: Gods` on the page's block (the folder's name within the page's own category; `folder: none` takes it out). A folder made earlier in the same paste can be used by pages and by later folders, and a page names a folder by the name it will have after this paste. Removing a folder removes only the folder: the pages in it move to the top level of the category and its sub-folders move up one level; no page is deleted.\n\n```\n---\nkind: folder\ncategory: Lore\nname: Gods\n---\n```\n\n```\n---\nkind: folder\ncategory: Lore\nname: Elder Gods\nparent: Gods\n---\n```\n\n```\n---\nkind: folder\naction: update\ncategory: Lore\nname: Cosmology\nrename_to: The Heavens\n---\n```"
}

// pendingFoldersKey carries the folders earlier rows of one paste will make.
type pendingFoldersKey struct{}

// withPendingFolders lets later rows be checked against folders that rows
// before them create. Review only: at commit those rows have already run.
func withPendingFolders(ctx context.Context) context.Context {
	return context.WithValue(ctx, pendingFoldersKey{}, map[string]bool{})
}

func pendingFolders(ctx context.Context) map[string]bool {
	m, _ := ctx.Value(pendingFoldersKey{}).(map[string]bool)
	return m
}

func folderKey(categoryID int, name string) string {
	return strconv.Itoa(categoryID) + "|" + strings.ToLower(strings.TrimSpace(name))
}

// folderTarget is what a record's keys point at, resolved against the campaign.
type folderTarget struct {
	cat     FolderCategory
	folders []FolderRef // this category's folders
	hit     *FolderRef  // the named folder, nil when it doesn't exist
	parent  *FolderRef  // the `parent` folder, nil when absent or "none"
	lift    bool        // `parent: none`
	newName string      // rename_to
}

// resolve reads the keys and finds the category, folder and parent. Names
// match ignoring case; one that matches two is refused.
func (k FolderKind) resolve(ctx context.Context, campaignID string, r Record) (folderTarget, error) {
	var t folderTarget
	catName := r.Str("category")
	if catName == "" {
		return t, badRequestf("a folder needs `category:`, the name of the category it lives in")
	}
	cats, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return t, err
	}
	var found []FolderCategory
	for _, c := range cats {
		if sameName(c.Name, catName) || sameName(c.Plural, catName) || sameName(c.Slug, catName) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 0:
		return t, badRequestf("no category called %s", quote(catName))
	case 1:
		t.cat = found[0]
	default:
		return t, badRequestf("more than one category is called %s", quote(catName))
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return t, err
	}
	for _, f := range all {
		if f.CategoryID == t.cat.ID {
			t.folders = append(t.folders, f)
		}
	}
	if t.hit, err = pick(t.folders, r.Name); err != nil {
		return t, err
	}
	if p := r.Str("parent"); p != "" {
		if strings.EqualFold(p, "none") {
			t.lift = true
		} else if t.parent, err = pick(t.folders, p); err != nil {
			return t, err
		} else if t.parent == nil && !pendingFolders(ctx)[folderKey(t.cat.ID, p)] {
			return t, badRequestf("no folder called %s in %s to nest it in", quote(p), t.cat.Name)
		}
	}
	t.newName = r.Str("rename_to")
	return t, nil
}

// pick is the one folder with the name, nil for none, an error for several.
func pick(folders []FolderRef, name string) (*FolderRef, error) {
	var hit *FolderRef
	for i := range folders {
		if !sameName(folders[i].Name, name) {
			continue
		}
		if hit != nil {
			return nil, badRequestf("more than one folder is called %s in this category", quote(name))
		}
		hit = &folders[i]
	}
	return hit, nil
}

func (k FolderKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if !a.CanAuthorDmOnly() {
		return Plan{Error: "only the owner or a co-DM can change folders"}
	}
	if r.Name == "" {
		return Plan{Error: "a folder needs a name"}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	where := " in " + t.cat.Name
	switch r.Action {
	case ActionCreate:
		if t.hit != nil || pendingFolders(ctx)[folderKey(t.cat.ID, r.Name)] {
			return Plan{Error: "a folder called " + quote(r.Name) + " already exists in " + t.cat.Name + "; use action: update"}
		}
		if t.lift || t.newName != "" {
			return Plan{Error: "a new folder has nothing to rename or lift; leave out `rename_to` and `parent: none`"}
		}
		if p := r.Str("parent"); p != "" {
			where = " under " + quote(p) + where
		}
		return Plan{Summary: "makes the folder " + quote(r.Name) + where}
	case ActionUpdate:
		if t.hit == nil {
			return Plan{Error: "no folder called " + quote(r.Name) + " in " + t.cat.Name}
		}
		var parts []string
		if t.newName != "" {
			if o, err := pick(t.folders, t.newName); err != nil || (o != nil && o.ID != t.hit.ID) {
				return Plan{Error: "a folder called " + quote(t.newName) + " already exists in " + t.cat.Name}
			}
			parts = append(parts, "renames it to "+quote(t.newName))
		}
		switch {
		case t.lift:
			parts = append(parts, "moves it to the top level of "+t.cat.Name)
		case t.parent != nil:
			if t.parent.ID == t.hit.ID {
				return Plan{Error: "a folder cannot go inside itself"}
			}
			parts = append(parts, "moves it into "+quote(t.parent.Name))
		case r.Str("parent") != "":
			parts = append(parts, "moves it into "+quote(r.Str("parent")))
		}
		if len(parts) == 0 {
			return Plan{Error: "nothing to change: give `rename_to:` or `parent:`"}
		}
		return Plan{Summary: strings.Join(parts, " and ")}
	default:
		if t.hit == nil {
			return Plan{Error: "no folder called " + quote(r.Name) + " in " + t.cat.Name}
		}
		return Plan{Summary: "removes only the folder " + quote(r.Name) + ": its pages move to the top level of " +
			t.cat.Name + " and its sub-folders move up one level; no page is deleted"}
	}
}

func (k FolderKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	switch r.Action {
	case ActionCreate:
		parent := ""
		if t.parent != nil {
			parent = t.parent.ID
		}
		return k.Svc.Create(ctx, campaignID, t.cat.ID, r.Name, parent)
	case ActionDelete:
		return k.Svc.Delete(ctx, campaignID, t.hit.ID)
	}
	if t.lift || t.parent != nil {
		var p *string
		if t.parent != nil {
			p = &t.parent.ID
		}
		if err := k.Svc.Move(ctx, campaignID, t.hit.ID, p); err != nil {
			return err
		}
	}
	if t.newName != "" {
		return k.Svc.Rename(ctx, campaignID, t.hit.ID, t.newName)
	}
	return nil
}

// Export lists every folder, parents before the folders inside them, in this
// kind's own format so an AI can rename, move or remove one by name.
func (k FolderKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if !a.CanAuthorDmOnly() {
		return "", nil
	}
	cats, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return "", err
	}
	all, err := k.Svc.List(ctx, campaignID)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range cats {
		var in []FolderRef
		byID := map[string]FolderRef{}
		for _, f := range all {
			if f.CategoryID == c.ID {
				in = append(in, f)
				byID[f.ID] = f
			}
		}
		// A folder is written after its parent. The depth bound stops a
		// stored loop from repeating forever.
		written := map[string]bool{}
		var write func(f FolderRef, depth int)
		write = func(f FolderRef, depth int) {
			if written[f.ID] {
				return
			}
			if p, ok := byID[f.ParentID]; ok && depth < 50 {
				write(p, depth+1)
			}
			written[f.ID] = true
			b.WriteString("---\nkind: folder\ncategory: " + quote(c.Name) + "\nname: " + quote(f.Name) + "\n")
			if p, ok := byID[f.ParentID]; ok {
				b.WriteString("parent: " + quote(p.Name) + "\n")
			}
			b.WriteString("---\n\n")
		}
		for _, f := range in {
			write(f, 0)
		}
	}
	return b.String(), nil
}
