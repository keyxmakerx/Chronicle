package campaigns

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// legacySidebarConfig mirrors the pre-unification sidebar_config JSON shape.
// It exists ONLY so the one-time EnsureSidebarItems reconciler (and the import
// restore path) can still read the four legacy fields after they were removed
// from the canonical SidebarConfig struct. Nothing else references it — the
// moment a legacy campaign is converted, its JSON carries only `items` (plus
// the non-legacy hidden sets) and this shape becomes irrelevant to it.
type legacySidebarConfig struct {
	Items           []SidebarItem      `json:"items,omitempty"`
	HiddenEntityIDs []string           `json:"hidden_entity_ids,omitempty"`
	HiddenNodeIDs   []string           `json:"hidden_node_ids,omitempty"`
	EntityTypeOrder []int              `json:"entity_type_order,omitempty"`
	HiddenTypeIDs   []int              `json:"hidden_type_ids,omitempty"`
	CustomSections  []legacyNavSection `json:"custom_sections,omitempty"`
	CustomLinks     []legacyNavLink    `json:"custom_links,omitempty"`
}

// legacyNavSection / legacyNavLink are the retired custom-nav shapes, kept
// private here purely for the one-time conversion.
type legacyNavSection struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	After string `json:"after"`
}

type legacyNavLink struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	URL     string `json:"url"`
	Icon    string `json:"icon"`
	Section string `json:"section"`
}

// convertLegacySidebarConfig translates a legacy sidebar_config JSON blob into
// the unified items model. It returns (converted, true) ONLY when the config is
// on the legacy model AND has something to convert — i.e. Items is empty but at
// least one legacy field is populated. A config already on items, or empty on
// both models (a never-customized campaign, which renders the default from the
// injector), returns (_, false) and must be left untouched — so the reconciler
// is a clean no-op on the second run.
//
// The synthesized Items reproduce the legacy customization: category items in
// EntityTypeOrder order (hidden types carried as Visible:false so the render's
// inject-missing pass can't re-add them visible), then any hidden type not in
// the order, then the custom sections and links. The standard scaffold
// (dashboard, addon links, the remaining categories, All Pages) is added by the
// render injector, exactly as it is for a campaign customized through the editor.
func convertLegacySidebarConfig(raw string) (SidebarConfig, bool) {
	if raw == "" {
		return SidebarConfig{}, false
	}
	var lc legacySidebarConfig
	if err := json.Unmarshal([]byte(raw), &lc); err != nil {
		// Unparseable config: leave it for the render path's defensive default.
		return SidebarConfig{}, false
	}
	// Already on the unified model.
	if len(lc.Items) > 0 {
		return SidebarConfig{}, false
	}
	// Nothing legacy to convert (never-customized campaign).
	if len(lc.EntityTypeOrder) == 0 && len(lc.HiddenTypeIDs) == 0 &&
		len(lc.CustomSections) == 0 && len(lc.CustomLinks) == 0 {
		return SidebarConfig{}, false
	}

	hidden := make(map[int]bool, len(lc.HiddenTypeIDs))
	for _, id := range lc.HiddenTypeIDs {
		hidden[id] = true
	}

	items := make([]SidebarItem, 0, len(lc.EntityTypeOrder)+len(lc.HiddenTypeIDs)+len(lc.CustomSections)+len(lc.CustomLinks))
	emitted := make(map[int]bool, len(lc.EntityTypeOrder))

	// Ordered categories first (preserving the legacy order + hidden state).
	for _, id := range lc.EntityTypeOrder {
		if emitted[id] {
			continue
		}
		emitted[id] = true
		items = append(items, SidebarItem{Type: "category", TypeID: id, Visible: !hidden[id]})
	}
	// Any hidden type not present in the order must still be carried as an
	// explicit hidden item, or the render inject-missing pass would re-add it
	// as visible.
	for _, id := range lc.HiddenTypeIDs {
		if emitted[id] {
			continue
		}
		emitted[id] = true
		items = append(items, SidebarItem{Type: "category", TypeID: id, Visible: false})
	}
	// Custom sections and links: the data is preserved as section/link items.
	// (They rendered in the categories zone under the legacy model; under the
	// one unified template, section/link items render in the top nav.)
	for _, s := range lc.CustomSections {
		items = append(items, SidebarItem{Type: "section", ID: s.ID, Label: s.Label, Visible: true})
	}
	for _, l := range lc.CustomLinks {
		items = append(items, SidebarItem{Type: "link", ID: l.ID, Label: l.Label, URL: l.URL, Icon: l.Icon, Visible: true})
	}

	return SidebarConfig{
		Items:           items,
		HiddenEntityIDs: lc.HiddenEntityIDs,
		HiddenNodeIDs:   lc.HiddenNodeIDs,
	}, true
}

// EnsureSidebarItems is a one-time, idempotent boot reconciler that converts
// every campaign still on the legacy sidebar model (empty Items + populated
// legacy fields) onto the unified items model, back-writing the equivalent
// Items array once. Campaigns already on items, and never-customized campaigns
// (empty on both models, which render the default from the injector), are left
// untouched — so a second run is a clean no-op. Per-campaign failures are logged
// and skipped so one bad row neither aborts the sweep nor blocks startup.
// Returns the number of campaigns converted.
//
// This is a data reconciler, NOT a migration (per conventions: one-time data
// fixes run through the owning service, never as schema migrations), so it is
// safe to run on every boot and self-heals any straggler.
func (s *campaignService) EnsureSidebarItems(ctx context.Context) (int, error) {
	const perPage = 200
	converted := 0
	seen := 0
	for page := 1; ; page++ {
		batch, total, err := s.repo.ListAll(ctx, ListOptions{Page: page, PerPage: perPage})
		if err != nil {
			return converted, fmt.Errorf("listing campaigns for sidebar reconcile: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			c := &batch[i]
			newCfg, ok := convertLegacySidebarConfig(c.SidebarConfig)
			if !ok {
				continue
			}
			configJSON, err := json.Marshal(newCfg)
			if err != nil {
				slog.Warn("sidebar reconcile: marshal failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			if err := s.repo.UpdateSidebarConfig(ctx, c.ID, string(configJSON)); err != nil {
				slog.Warn("sidebar reconcile: write failed",
					slog.String("campaign_id", c.ID), slog.Any("error", err))
				continue
			}
			converted++
		}
		seen += len(batch)
		if seen >= total {
			break
		}
	}
	return converted, nil
}
