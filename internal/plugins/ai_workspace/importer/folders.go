package importer

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// FolderAccess is the part of the entities folder service that `folder:` on
// a page needs; entities.FolderService satisfies it.
type FolderAccess interface {
	// FindFolder returns nil when the category has no folder of that name and
	// an error when the name is ambiguous.
	FindFolder(ctx context.Context, campaignID string, entityTypeID int, name string) (*entities.SidebarNode, error)
	SetEntityFolder(ctx context.Context, campaignID, entityID string, folderID *string) error
}

// SetFolders wires `folder:` on page rows. Optional: unwired, a row that
// names a folder is refused rather than saved in the wrong place.
func (c *Committer) SetFolders(f FolderAccess) { c.folders = f }

// folderMove is where a saved page should end up: nothing to do, a folder, or
// out of its folder.
type folderMove struct {
	active bool
	id     *string
}

// resolveFolder finds the page's folder before anything is written, so a
// missing folder refuses the row instead of leaving a half-filed page. A page
// that gains a parent page leaves its folder, since it cannot sit in both.
func (c *Committer) resolveFolder(ctx context.Context, campaignID string, typeID int, page ParsedPage, creating, gainsParent bool) (folderMove, string) {
	fm := page.FrontMatter
	if name, ok := fm.FolderName(); ok {
		if c.folders == nil {
			return folderMove{}, "Folders can't be set here, so this page was left alone."
		}
		quoted := strconv.Quote(name)
		node, err := c.folders.FindFolder(ctx, campaignID, typeID, name)
		if err != nil {
			var ae *apperror.AppError
			if errors.As(err, &ae) && ae.Code == 400 && ae.Message != "" {
				return folderMove{}, "The folder " + quoted + ": " + ae.Message + ". This page was left alone."
			}
			return folderMove{}, "Could not look up the folder " + quoted + ". Try again in a moment."
		}
		if node == nil {
			return folderMove{}, "No folder called " + quoted + " exists in this page's category, so this page was left alone. Make the folder first, or fix the name."
		}
		return folderMove{active: true, id: &node.ID}, ""
	}
	if !creating && c.folders != nil && (fm.LeavesFolder() || gainsParent) {
		return folderMove{active: true}, ""
	}
	return folderMove{}, ""
}

// applyFolder files the saved page. The page itself is already saved, so a
// failure here is reported on a row that still links it.
func (c *Committer) applyFolder(ctx context.Context, campaignID, entityID string, m folderMove) string {
	if !m.active {
		return ""
	}
	if err := c.folders.SetEntityFolder(ctx, campaignID, entityID, m.id); err != nil {
		return "Saved the page, but could not put it in its folder. Move it from the sidebar."
	}
	return ""
}

// checkFolders turns a `folder:` that can't be used into a row problem. A
// folder that a folder block earlier in this paste makes counts as present;
// the committer looks again when the page is saved.
func (c *Classifier) checkFolders(ctx context.Context, pages []ParsedPage, cls []Classification) {
	for i, p := range pages {
		name, ok := p.FrontMatter.FolderName()
		if !ok || p.Status == StatusParseError || cls[i].Status == StatusParseError ||
			cls[i].Status == StatusActionMismatch || p.FrontMatter.Action == ActionDelete || p.FrontMatter.IsEra() {
			continue
		}
		if problem := c.folderProblem(ctx, p, cls[i], name); problem != "" {
			cls[i].Status = StatusParentProblem
			cls[i].Reason = problem
		}
	}
}

func (c *Classifier) folderProblem(ctx context.Context, p ParsedPage, cls Classification, name string) string {
	quoted := strconv.Quote(name)
	if c.folders == nil {
		return "Folders can't be set on this server. Remove the folder line."
	}
	var et *entities.EntityType
	switch {
	case p.FrontMatter.Action == ActionUpdate && cls.ConflictEntity != nil:
		et = c.typeByID(cls.ConflictEntity.EntityTypeID)
	case cls.IsNewCategory:
		return "The category " + strconv.Quote(cls.ProposedTypeSlug) + " is new, so it has no folders yet. Use an existing category, or leave the folder out."
	case cls.ExistingType != nil:
		et = cls.ExistingType
	}
	if et == nil {
		return "" // category is picked at review; the committer checks again
	}
	node, err := c.folders.FindFolder(ctx, c.campaignID, et.ID, name)
	if err != nil {
		var ae *apperror.AppError
		if errors.As(err, &ae) && ae.Code == 400 && ae.Message != "" {
			return "The folder " + quoted + ": " + ae.Message + "."
		}
		return ""
	}
	if node != nil {
		return ""
	}
	// A folder made in this paste counts only for the category it is made in;
	// the folder block may spell the category by name, plural or slug.
	for _, cat := range []string{et.Name, et.NamePlural, et.Slug} {
		if cat != "" && c.pendingFolders[PendingFolderKey(cat, name)] {
			return ""
		}
	}
	msg := "No folder called " + quoted + " exists in the " + et.Name + " category or is made in this paste."
	if others := c.foldersElsewhere(ctx, name, et.ID); len(others) > 0 {
		msg += " There is one in " + joinOr(others) + ": under Change the details, pick that category, then answer Yes."
	}
	return msg
}

// foldersElsewhere names the other categories that have a folder of this name,
// or make one in this paste. Folders belong to one category, so an AI that
// names the right folder under the wrong category is a common slip; naming
// where the folder is lets the owner fix the row instead of guessing.
func (c *Classifier) foldersElsewhere(ctx context.Context, name string, skipTypeID int) []string {
	var out []string
	for i := range c.cachedTypes {
		et := &c.cachedTypes[i]
		if et.ID == skipTypeID {
			continue
		}
		found := false
		for _, cat := range []string{et.Name, et.NamePlural, et.Slug} {
			if cat != "" && c.pendingFolders[PendingFolderKey(cat, name)] {
				found = true
			}
		}
		if !found {
			// An ambiguous name is still a folder there; any other error
			// only loses the hint.
			node, err := c.folders.FindFolder(ctx, c.campaignID, et.ID, name)
			var ae *apperror.AppError
			found = node != nil || (errors.As(err, &ae) && ae.Code == 400)
		}
		if found {
			out = append(out, et.Name)
		}
	}
	return out
}

// joinOr lists names as "A", "A or B", or "A, B or C".
func joinOr(names []string) string {
	if len(names) <= 1 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// PendingFolderKey keys a folder this paste makes by category and name, both
// lower-cased, so two categories may each have a folder of one name.
func PendingFolderKey(category, name string) string {
	return strings.ToLower(strings.TrimSpace(category)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

// WithFolders lets the classifier check `folder:` names. pending holds the
// PendingFolderKey of each folder this paste makes, which exists by commit time.
func (c *Classifier) WithFolders(f FolderAccess, pending map[string]bool) *Classifier {
	c.folders, c.pendingFolders = f, pending
	return c
}
