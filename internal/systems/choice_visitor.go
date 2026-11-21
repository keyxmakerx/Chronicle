package systems

import "context"

// choice_visitor.go is the pick list as a look-around visitor reads it: the
// list a brand-new player with no rules or grants of their own would be
// offered, and nothing a public reader of the campaign cannot already read.
//
// The reader is nobody in particular. The context is rewritten so no later
// wrapper can widen it (not a Director, no account, not unlimited), and
// the list is built only from what the public reads: the package's files
// that are not Director-only (the same test SystemDataAPI makes) and the
// campaign's entries as a signed-out reader's catalog has them (accepted,
// visible to everyone). Rules apply as they do to a player with no account,
// so rows about everyone narrow the list and rows about a named player never
// reach it.

type choiceVisitorKey struct{}

// WithChoiceVisitor reads the pick list as a look-around visitor does.
func WithChoiceVisitor(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, choiceVisitorKey{}, true)
	ctx = context.WithValue(ctx, unlimitedKey{}, false)
	ctx = WithChoiceViewer(ctx, false)
	ctx = WithChoiceViewerUser(ctx, "")
	return WithChoiceFor(ctx, "")
}

func choiceVisitor(ctx context.Context) bool {
	v, _ := ctx.Value(choiceVisitorKey{}).(bool)
	return v
}

// visitorMayRead keeps a choice a public reader could read: never one kept to
// a character, never a draft, never a player's own. The overlay already
// leaves these out for a reader with no account; this is the second lock.
func visitorMayRead(c Choice) bool {
	return c.ForName == "" && c.Status == "" && !c.Mine && c.Visibility != EntryVisibilityDirectors && c.Visibility != EntryVisibilityCharacter
}

func visitorChoices(in []Choice) []Choice {
	out := make([]Choice, 0, len(in))
	for _, c := range in {
		if visitorMayRead(c) {
			out = append(out, c)
		}
	}
	return out
}
