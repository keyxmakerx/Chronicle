package entities

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/sanitize"
)

// HideUnviewableMentions rewrites each given text, HTML or ProseMirror JSON,
// so a mention of a page this viewer can't open reads "a hidden page": the
// stored link carries the page's name, which the page's own visibility hides.
// Every text is checked against one batch of ids. If the check fails, every
// mention is hidden, since a name shown wrongly can't be taken back. It
// returns how many mentions it replaced. nil texts are skipped.
func (s *entityService) HideUnviewableMentions(ctx context.Context, campaignID string, role int, userID string, texts ...*string) (int, error) {
	var ids []string
	seen := map[string]bool{}
	for _, t := range texts {
		if t == nil {
			continue
		}
		for _, id := range sanitize.MentionIDs(*t) {
			if !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	viewable, err := s.entities.FilterViewableEntityIDs(ctx, campaignID, ids, role, userID)
	if err != nil {
		viewable = nil
	}
	keep := func(id string) bool { return viewable[id] }
	total := 0
	for _, t := range texts {
		if t == nil {
			continue
		}
		out, n := sanitize.HideMentions(*t, keep)
		*t = out
		total += n
	}
	return total, err
}
