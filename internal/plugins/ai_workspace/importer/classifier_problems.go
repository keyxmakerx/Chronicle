package importer

import (
	"context"
	"fmt"
	"strings"
)

// PageProblem is a page block the review could never apply: whatever the
// owner picks there, it stays unusable.
type PageProblem struct {
	Block  int
	Page   string
	Action string
	Reason string
}

// Did is the problem as one "What it did" line, such as "Asked to change
// Nobody Here, refused: no such page".
func (p PageProblem) Did() string {
	verb, why := "change", "its sheet fields don't fit the category"
	switch p.Action {
	case ActionDelete:
		verb = "remove"
	case ActionUpdate:
	default:
		verb = "add"
	}
	if strings.HasPrefix(p.Reason, "Cannot ") {
		why = "no such page"
	}
	return fmt.Sprintf("Asked to %s %s, refused: %s", verb, p.Page, why)
}

// Line is the problem in the AI's own terms.
func (p PageProblem) Line() string {
	return fmt.Sprintf("block %d (%s): %s", p.Block, p.Page, p.Reason)
}

// PageProblems returns the blocks the review would mark unusable with
// nothing the owner can choose to fix them: an update or delete of a page
// that doesn't exist, and sheet fields the page's category doesn't have.
// A new category or an era row is left out, since the owner can still map
// the category at review. It looks pages up by name, so callers must only
// tell the result to a key that may know a page exists.
func (c *Classifier) PageProblems(ctx context.Context, pages []ParsedPage) ([]PageProblem, error) {
	cls, err := c.ClassifyAll(ctx, pages)
	if err != nil {
		return nil, err
	}
	var out []PageProblem
	for i, p := range pages {
		k := cls[i]
		unfixable := k.Status == StatusActionMismatch ||
			(k.Status == StatusFieldProblem && !k.IsNewCategory && !p.FrontMatter.IsEra() &&
				strings.HasPrefix(k.Reason, "These sheet fields don't fit"))
		if unfixable {
			out = append(out, PageProblem{Block: p.SourceIndex + 1, Page: p.Name, Action: p.FrontMatter.Action, Reason: k.Reason})
		}
	}
	return out, nil
}
