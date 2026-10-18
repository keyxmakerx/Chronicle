package importer

import (
	"context"
	"fmt"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/widgets/tags"
)

// TagWriter is the slice of tags.TagService a page row's `tags:` needs. The
// concrete tags.TagService implements it as-is.
type TagWriter interface {
	ListByCampaign(ctx context.Context, campaignID string, includeDmOnly bool) ([]tags.Tag, error)
	Create(ctx context.Context, campaignID string, name, color string, dmOnly bool) (*tags.Tag, error)
	GetEntityTags(ctx context.Context, entityID string, includeDmOnly bool) ([]tags.Tag, error)
	SetEntityTags(ctx context.Context, entityID string, campaignID string, tagIDs []int) error
}

// SetTagWriter wires `tags:` on page rows. Optional: unwired, a page is saved
// and its row says the tags were not.
func (c *Committer) SetTagWriter(w TagWriter) { c.tags = w }

// maxTagName is tags.name's column width.
const maxTagName = 100

// tagSlug matches tags.generateSlug, so "Trade Hub" finds the tag trade-hub.
func tagSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "tag"
	}
	return s
}

// applyTags sets a page's tags from its block. A nil list (no `tags:` line)
// leaves them alone; `tags: []` clears them. A name is matched to the
// campaign's tags by slug, and a tag that doesn't exist yet is made. On an
// update the page's GM-only tags always stay, since the AI's export may not
// have shown them. Returns a plain-words reason on failure.
func (c *Committer) applyTags(ctx context.Context, campaignID, entityID string, want []string, update bool) string {
	if want == nil {
		return ""
	}
	if c.tags == nil {
		return "Saved the page, but its tags could not be set here. Add them on the page."
	}
	existing, err := c.tags.ListByCampaign(ctx, campaignID, true)
	if err != nil {
		return "Saved the page, but could not set its tags. Add them on the page."
	}
	bySlug := make(map[string]tags.Tag, len(existing))
	for _, t := range existing {
		bySlug[t.Slug] = t
	}
	var ids []int
	seen := map[int]bool{}
	add := func(id int) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, name := range want {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if len(name) > maxTagName {
			return fmt.Sprintf("Saved the page, but the tag %q is longer than %d characters, so no tags were set.", string([]rune(name)[:20])+"…", maxTagName)
		}
		t, ok := bySlug[tagSlug(name)]
		if !ok {
			made, err := c.tags.Create(ctx, campaignID, name, "", false)
			if err != nil {
				return fmt.Sprintf("Saved the page, but could not make the tag %q, so no tags were set.", name)
			}
			t = *made
			bySlug[t.Slug] = t
		}
		add(t.ID)
	}
	if update {
		current, err := c.tags.GetEntityTags(ctx, entityID, true)
		if err != nil {
			return "Saved the page, but could not set its tags. Add them on the page."
		}
		for _, t := range current {
			if t.DmOnly {
				add(t.ID)
			}
		}
	}
	if err := c.tags.SetEntityTags(ctx, entityID, campaignID, ids); err != nil {
		return "Saved the page, but could not set its tags. Add them on the page."
	}
	return ""
}
