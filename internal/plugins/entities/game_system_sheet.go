package entities

import (
	"context"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// unplacedPageTabs is the block tabs (Trackers, Cards) a game system sheet
// placed by a layout brings along: those the whole-page sheet would show
// under it, less any block the layout already places, so nothing draws twice.
func unplacedPageTabs(ctx context.Context, cc *campaigns.CampaignContext, entity *Entity, entityType *EntityType, csrfToken string) []pageTab {
	seen, addable := blockPageTabs(ctx, cc, entity, entityType, csrfToken)
	if len(seen)+len(addable) == 0 {
		return nil
	}
	var placed map[string]bool
	if entityType != nil {
		placed = layoutBlockTypes(entityType.Layout)
	}
	var out []pageTab
	for _, t := range append(seen, addable...) {
		if !placed[t.Block] {
			out = append(out, t)
		}
	}
	return out
}

// layoutBlockTypes is every block type a layout places, in its columns, its
// tile sections and inside container blocks, whose children sit in their
// config.
func layoutBlockTypes(l EntityTypeLayout) map[string]bool {
	out := map[string]bool{}
	for _, row := range l.Rows {
		for _, col := range row.Columns {
			for _, b := range col.Blocks {
				out[b.Type] = true
				collectConfigBlockTypes(b.Config, out)
			}
		}
		for _, t := range row.Tiles {
			out[t.Type] = true
			collectConfigBlockTypes(t.Config, out)
		}
	}
	return out
}

// collectConfigBlockTypes walks a container's config for child blocks: any
// object carrying a string "type" beside an "id" is one, however deep the
// container's own shape (columns, tabs, sections) nests it.
func collectConfigBlockTypes(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		if t, ok := x["type"].(string); ok && t != "" {
			if _, hasID := x["id"]; hasID {
				out[t] = true
			}
		}
		for _, c := range x {
			collectConfigBlockTypes(c, out)
		}
	case []any:
		for _, c := range x {
			collectConfigBlockTypes(c, out)
		}
	case []map[string]any:
		for _, c := range x {
			collectConfigBlockTypes(c, out)
		}
	}
}
