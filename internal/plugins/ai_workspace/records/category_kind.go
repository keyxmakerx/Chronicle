package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// CategoryAPI is the campaign's categories as the Customizer's Categories
// tab manages them, narrowed to removing one.
type CategoryAPI interface {
	// IsOwner reports whether userID owns the campaign; only the owner
	// removes a category.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// Categories lists every category of the campaign with its page counts.
	Categories(ctx context.Context, campaignID string) ([]CategoryInfo, error)
	// DeleteCategory deletes a category that has no pages, none in the Trash.
	DeleteCategory(ctx context.Context, campaignID string, id int) error
	// MergeCategory moves every page of fromID (Trash included), its folders
	// and saved filters into toID, then deletes fromID; it returns the
	// number of pages moved.
	MergeCategory(ctx context.Context, campaignID string, fromID, toID int) (int, error)
}

// CategoryInfo is one category with what deleting it would touch.
type CategoryInfo struct {
	ID                 int
	Name, Plural, Slug string
	Parent             string   // the name of the category it is a kind of; "" at the top
	Kinds              []string // the names of its own kinds
	DMOnly             bool     // only the Director team sees it and its pages
	Pages, Trashed     int
}

// CategoryKind lets an AI ask to delete a category, or to merge it into
// another by moving its pages there first. Making and changing categories
// stays in the Customizer, where their fields and layout are drawn.
type CategoryKind struct {
	Svc CategoryAPI
}

func (CategoryKind) Name() string  { return "category" }
func (CategoryKind) Label() string { return "Category" }
func (CategoryKind) Brief() string {
	return "delete a category, or merge it into another by moving its pages there first (owner only)"
}
func (CategoryKind) Doc() string {
	return "Deletes a category, only with `action: delete`; categories are made and changed in Customize › Categories. " +
		"Keys: `category`, the category to delete, by its name, plural name or slug; `move_pages_to`, the category that takes its pages, named the same way. " +
		"A category that still has pages, counting those in the Trash, is deleted only with `move_pages_to`: its pages, its sidebar folders and saved filters move there first, keeping their words, tags and links. " +
		"Its own page layout, home and fields are deleted with it; values in fields the other category lacks stay saved on the pages but are not shown. " +
		"A category only the Director team sees can move its pages only into another such category. A category with kinds of its own is refused until those are deleted or moved. " +
		"When two categories share a name, name each by its slug, written exactly as the list of categories below gives it. Only the campaign's owner can delete a category, and it cannot be undone.\n\n" +
		"```\n---\nkind: category\naction: delete\ncategory: adventures-2\nmove_pages_to: adventures\n---\n```"
}

// Title names the review row after the category it deletes.
func (CategoryKind) Title(r Record) string {
	if c := strings.TrimSpace(r.Str("category")); c != "" {
		return c
	}
	if n := strings.TrimSpace(r.Name); n != "" {
		return n
	}
	return "Category"
}

// isOwner is the owner acting as the owner, as for MenuOrderKind: an export
// for what players see carries the owner's id with a player's role.
func (k CategoryKind) isOwner(ctx context.Context, campaignID string, a Actor) (bool, error) {
	if !a.CanAuthorDmOnly() {
		return false, nil
	}
	return k.Svc.IsOwner(ctx, campaignID, a.UserID)
}

// categoryTarget is a record resolved against the campaign's categories.
type categoryTarget struct {
	from CategoryInfo
	to   *CategoryInfo // nil without move_pages_to
	all  []CategoryInfo
}

// categoryWord is how the row names the category to delete: `category:`,
// else the block's name.
func categoryWord(r Record) string {
	if name := strings.TrimSpace(r.Str("category")); name != "" {
		return name
	}
	return strings.TrimSpace(r.Name)
}

// resolve finds the record's category and, when written, the one taking its
// pages.
func (k CategoryKind) resolve(ctx context.Context, campaignID string, r Record) (categoryTarget, error) {
	var t categoryTarget
	name := categoryWord(r)
	if name == "" {
		return t, badRequestf("name the category to delete in `category:`")
	}
	all, err := k.Svc.Categories(ctx, campaignID)
	if err != nil {
		return t, err
	}
	t.all = all
	if t.from, err = findCategory(all, name); err != nil {
		return t, err
	}
	if to := strings.TrimSpace(r.Str("move_pages_to")); to != "" {
		c, err := findCategory(all, to)
		if err != nil {
			return t, err
		}
		t.to = &c
	}
	return t, nil
}

// findCategory matches a category by name, plural name or slug, ignoring
// case. A slug written exactly is unique, so it settles a name two
// categories share, even when the slug reads like that name ("adventures"
// for the first of two called Adventures); written any other way, a shared
// name is refused rather than guessed, since the row deletes.
func findCategory(all []CategoryInfo, name string) (CategoryInfo, error) {
	var bySlug, hit []CategoryInfo
	for _, c := range all {
		if c.Slug == name {
			bySlug = append(bySlug, c)
		}
		if sameName(c.Name, name) || sameName(c.Plural, name) || sameName(c.Slug, name) {
			hit = append(hit, c)
		}
	}
	if len(bySlug) == 1 {
		return bySlug[0], nil
	}
	switch len(hit) {
	case 0:
		return CategoryInfo{}, badRequestf("no category called %s", quote(name))
	case 1:
		return hit[0], nil
	}
	parts := make([]string, len(hit))
	for i, c := range hit {
		parts[i] = c.Slug + " (" + categoryFacts(c) + ")"
	}
	list := strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	return CategoryInfo{}, badRequestf("%s categories are called %s: %s; name one by its slug", countWord(len(hit)), name, list)
}

// categoryFacts is a category's pages and who sees it, in a few words.
func categoryFacts(c CategoryInfo) string {
	s := pageCount(c.Pages)
	if c.Trashed > 0 {
		s += fmt.Sprintf(", %d in the Trash", c.Trashed)
	}
	if c.DMOnly {
		s += ", only the Director team sees it"
	}
	return s
}

func pageCount(n int) string {
	switch n {
	case 0:
		return "no pages"
	case 1:
		return "1 page"
	}
	return fmt.Sprintf("%d pages", n)
}

func countWord(n int) string {
	if n >= 2 && n <= 5 {
		return []string{"two", "three", "four", "five"}[n-2]
	}
	return fmt.Sprint(n)
}

// menuLabel is a category as the sidebar names it: its plural name.
func (c CategoryInfo) menuLabel() string {
	if c.Plural != "" {
		return c.Plural
	}
	return c.Name
}

// movingPages says which of a category's pages a merge moves.
func movingPages(c CategoryInfo) string {
	switch {
	case c.Pages > 0 && c.Trashed > 0:
		return pageCount(c.Pages) + " and the " + fmt.Sprint(c.Trashed) + " in the Trash"
	case c.Pages > 0:
		return pageCount(c.Pages)
	}
	return pageCount(c.Trashed) + " in the Trash"
}

// nameIn is a category's name, with its slug when another category in all
// reads the same, so a row never leaves the owner guessing which one.
func nameIn(c CategoryInfo, all []CategoryInfo) string {
	for _, o := range all {
		if o.ID != c.ID && sameName(o.menuLabel(), c.menuLabel()) {
			return c.menuLabel() + " (" + c.Slug + ")"
		}
	}
	return c.menuLabel()
}

func (k CategoryKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Action != ActionDelete {
		return Plan{Error: "categories are made and changed in Customize › Categories; a `category` block only deletes one, with `action: delete`"}
	}
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	} else if !owner {
		return Plan{Error: "only the campaign's owner can delete a category"}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	from := t.from
	self := from.menuLabel() + " (" + from.Slug + ")"
	if len(from.Kinds) > 0 {
		return Plan{Error: self + " has kinds of its own (" + strings.Join(from.Kinds, ", ") + "); delete or move its kinds first"}
	}
	if t.to != nil && t.to.ID == from.ID {
		return Plan{Error: "a category can't move its pages into itself"}
	}
	const undone = " This cannot be undone."
	if from.Pages+from.Trashed == 0 {
		p := Plan{Summary: self + " is deleted. It has no pages.", Warnings: []string{"its own page layout, home and fields are deleted with it." + undone}}
		if t.to != nil {
			p.Summary = self + " is deleted. It has no pages, so nothing moves into " + nameIn(*t.to, t.all) + "."
		}
		return withSlugNotes(p, t, r)
	}
	if t.to == nil {
		return Plan{Error: fmt.Sprintf("%s still has %s; name the category that takes them in `move_pages_to`", self, movingPages(from))}
	}
	to := *t.to
	toName := nameIn(to, t.all)
	if from.DMOnly && !to.DMOnly {
		return Plan{Error: self + " is for the Director team only, so its pages can only move into another category only the Director team sees, and players see " + toName}
	}
	p := Plan{Summary: self + " is deleted; its " + movingPages(from) + " and its folders move into " + toName + "."}
	p.Warnings = append(p.Warnings, "its own page layout, home and fields are deleted with it; values in fields "+toName+" doesn't have stay saved on the pages but are not shown."+undone)
	if to.DMOnly && !from.DMOnly {
		p.Warnings = append(p.Warnings, "players stop seeing these pages: "+toName+" is for the Director team only")
	}
	return withSlugNotes(p, t, r)
}

// withSlugNotes warns when a word the row used is the slug of the category
// it picked but the name of another one, so a renamed category is never
// deleted or filled under a name that now belongs to something else.
func withSlugNotes(p Plan, t categoryTarget, r Record) Plan {
	note := func(word string, picked *CategoryInfo) {
		if picked == nil || word == "" {
			return
		}
		for _, c := range t.all {
			if c.ID != picked.ID && (sameName(c.Name, word) || sameName(c.Plural, word)) {
				p.Warnings = append(p.Warnings, quote(word)+" is the slug of "+picked.menuLabel()+"; the category called "+c.menuLabel()+" is "+quote(c.Slug))
				return
			}
		}
	}
	note(categoryWord(r), &t.from)
	note(strings.TrimSpace(r.Str("move_pages_to")), t.to)
	return p
}

func (k CategoryKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	// An empty category is only deleted, whatever move_pages_to says: there
	// is nothing to move, and the merge's rules are about its pages.
	if t.to == nil || t.from.Pages+t.from.Trashed == 0 {
		return k.Svc.DeleteCategory(ctx, campaignID, t.from.ID)
	}
	_, err = k.Svc.MergeCategory(ctx, campaignID, t.from.ID, t.to.ID)
	return err
}

// Export lists the categories so the AI can name the one to delete, for the
// owner only, since it names the ones only the Director team sees.
func (k CategoryKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil || !owner {
		return "", err
	}
	all, err := k.Svc.Categories(ctx, campaignID)
	if err != nil || len(all) == 0 {
		return "", err
	}
	var b strings.Builder
	b.WriteString("Every category, by name and slug; a `category` block names one by either.\n\n")
	for _, c := range all {
		b.WriteString("- " + c.menuLabel() + " (slug: `" + c.Slug + "`)")
		if c.Parent != "" {
			b.WriteString(", a kind of " + c.Parent)
		}
		b.WriteString(": " + categoryFacts(c) + "\n")
	}
	b.WriteString("\n")
	return b.String(), nil
}
