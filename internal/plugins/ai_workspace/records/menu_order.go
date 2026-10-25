package records

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// MenuAPI is the sidebar menu's category order, as the owner sets it in the
// sidebar settings.
type MenuAPI interface {
	// IsOwner reports whether userID owns the campaign; only the owner
	// arranges the sidebar.
	IsOwner(ctx context.Context, campaignID, userID string) (bool, error)
	// MenuCategories lists the top-level categories in the order the
	// sidebar draws them.
	MenuCategories(ctx context.Context, campaignID string) ([]MenuCategory, error)
	// PreviewOrder is MenuCategories as it would read after OrderCategories.
	PreviewOrder(ctx context.Context, campaignID string, ids []int) ([]MenuCategory, error)
	// OrderCategories puts the categories ids names in that order, each
	// within the section it sits in; the rest keep their places.
	OrderCategories(ctx context.Context, campaignID string, ids []int) error
}

// MenuCategory is one category in the sidebar menu and the section it is in.
type MenuCategory struct {
	ID                 int
	Name, Plural, Slug string
	Section            string // the section's title, e.g. "Categories"
}

// MenuOrderKind sets the order of the categories in the sidebar menu. Pages
// and folders inside a category have no order to set: the sidebar always
// shows folders first, then pages, each alphabetical.
type MenuOrderKind struct {
	Svc MenuAPI
}

func (MenuOrderKind) Name() string  { return "menu-order" }
func (MenuOrderKind) Label() string { return "Sidebar menu order" }
func (MenuOrderKind) Doc() string {
	return "The order of the categories in the sidebar menu. Key: `categories`, a list of category names, the one wanted highest first. Categories left out keep their places; each listed category moves only among the categories in its own section of the menu (Pinned, Categories, or a section the owner made), since which section a category sits in is set by the owner in the sidebar settings. Only the campaign's owner can change it. Inside a category the sidebar always shows folders first, then pages, each alphabetical, so there is no order to set there: arrange pages with `folder:` on the page's block and the `folder` kind.\n\n```\n---\nkind: menu-order\ncategories:\n  - Characters\n  - Places\n  - Lore\n---\n```"
}

// menuTarget is a record's list resolved against the menu.
type menuTarget struct {
	ids           []int
	before, after []MenuCategory
}

func (k MenuOrderKind) resolve(ctx context.Context, campaignID string, r Record) (menuTarget, error) {
	var t menuTarget
	names := r.List("categories")
	if len(names) == 0 {
		return t, badRequestf("list the categories in `categories:`, the one wanted highest first")
	}
	menu, err := k.Svc.MenuCategories(ctx, campaignID)
	if err != nil {
		return t, err
	}
	t.before = menu
	listed := map[int]string{}
	for _, v := range names {
		name := strings.TrimSpace(fmt.Sprint(v))
		var hit []MenuCategory
		for _, c := range menu {
			if sameName(c.Name, name) || sameName(c.Plural, name) || sameName(c.Slug, name) {
				hit = append(hit, c)
			}
		}
		switch len(hit) {
		case 0:
			return t, badRequestf("no category called %s in the sidebar menu", quote(name))
		case 1:
		default:
			return t, badRequestf("more than one category is called %s", quote(name))
		}
		if was, dup := listed[hit[0].ID]; dup {
			return t, badRequestf("%s is listed twice (once as %s)", quote(name), quote(was))
		}
		listed[hit[0].ID] = name
		t.ids = append(t.ids, hit[0].ID)
	}
	if t.after, err = k.Svc.PreviewOrder(ctx, campaignID, t.ids); err != nil {
		return t, err
	}
	return t, nil
}

func (k MenuOrderKind) Plan(ctx context.Context, campaignID string, a Actor, r Record) Plan {
	if r.Action == ActionDelete {
		return Plan{Error: "the menu's order can't be removed; list the categories in the order wanted instead"}
	}
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil {
		return Plan{Error: planError(err)}
	} else if !owner {
		return Plan{Error: "only the campaign's owner can change the sidebar menu"}
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return Plan{Error: planError(err)}
	}
	if menuNames(t.before) == menuNames(t.after) {
		return Plan{Error: "the menu is already in this order"}
	}
	p := Plan{Summary: "the menu's categories become: " + menuNames(t.after) + " (now: " + menuNames(t.before) + ")"}
	if sections := listedSections(t); len(sections) > 1 {
		p.Warnings = append(p.Warnings, "these categories sit in different sections of the menu ("+
			strings.Join(sections, ", ")+"); each moves only within its own section")
	}
	return p
}

func (k MenuOrderKind) Apply(ctx context.Context, campaignID string, a Actor, r Record) error {
	if p := k.Plan(ctx, campaignID, a, r); p.Error != "" {
		return apperror.NewBadRequest(p.Error)
	}
	t, err := k.resolve(ctx, campaignID, r)
	if err != nil {
		return err
	}
	return k.Svc.OrderCategories(ctx, campaignID, t.ids)
}

// isOwner is the owner acting as the owner. An export for what players see
// carries the owner's id with a player's role, and the menu names the
// categories only the DM team sees, so that actor is not the owner here.
func (k MenuOrderKind) isOwner(ctx context.Context, campaignID string, a Actor) (bool, error) {
	if !a.CanAuthorDmOnly() {
		return false, nil
	}
	return k.Svc.IsOwner(ctx, campaignID, a.UserID)
}

// Export writes the menu's order as it stands, for the owner only.
func (k MenuOrderKind) Export(ctx context.Context, campaignID string, a Actor) (string, error) {
	if owner, err := k.isOwner(ctx, campaignID, a); err != nil || !owner {
		return "", err
	}
	menu, err := k.Svc.MenuCategories(ctx, campaignID)
	if err != nil || len(menu) == 0 {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\nkind: menu-order\ncategories:\n")
	for _, c := range menu {
		b.WriteString("  - " + quote(menuName(c)) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String(), nil
}

// menuName is a category as the menu shows it: its plural name.
func menuName(c MenuCategory) string {
	if c.Plural != "" {
		return c.Plural
	}
	return c.Name
}

func menuNames(cs []MenuCategory) string {
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = menuName(c)
	}
	return strings.Join(names, ", ")
}

// listedSections is the sections the listed categories sit in, in menu order.
func listedSections(t menuTarget) []string {
	listed := map[int]bool{}
	for _, id := range t.ids {
		listed[id] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, c := range t.before {
		if listed[c.ID] && !seen[c.Section] {
			seen[c.Section] = true
			out = append(out, c.Section)
		}
	}
	return out
}
