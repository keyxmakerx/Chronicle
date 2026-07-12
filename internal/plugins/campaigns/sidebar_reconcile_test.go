// sidebar_reconcile_test.go — C-NAV-V3: the legacy→items conversion and the
// idempotent EnsureSidebarItems boot reconciler that back-writes it once.
package campaigns

import (
	"context"
	"encoding/json"
	"testing"
)

func TestConvertLegacySidebarConfig(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantOK      bool
		wantItems   []SidebarItem // only checked when wantOK
		wantHiddenE []string
	}{
		{
			name:   "already on items is not converted",
			raw:    `{"items":[{"type":"category","type_id":5,"visible":true}]}`,
			wantOK: false,
		},
		{
			name:   "empty on both models is not converted",
			raw:    `{}`,
			wantOK: false,
		},
		{
			name:   "empty string is not converted",
			raw:    ``,
			wantOK: false,
		},
		{
			name:   "unparseable is not converted",
			raw:    `not json`,
			wantOK: false,
		},
		{
			name:   "entity_type_order becomes ordered category items",
			raw:    `{"entity_type_order":[3,1,2]}`,
			wantOK: true,
			wantItems: []SidebarItem{
				{Type: "category", TypeID: 3, Visible: true},
				{Type: "category", TypeID: 1, Visible: true},
				{Type: "category", TypeID: 2, Visible: true},
			},
		},
		{
			name:   "hidden types carried as invisible items",
			raw:    `{"entity_type_order":[1,2],"hidden_type_ids":[2,9]}`,
			wantOK: true,
			wantItems: []SidebarItem{
				{Type: "category", TypeID: 1, Visible: true},
				{Type: "category", TypeID: 2, Visible: false}, // hidden, in order
				{Type: "category", TypeID: 9, Visible: false}, // hidden, not in order
			},
		},
		{
			name:   "custom sections and links convert to items and hidden sets pass through",
			raw:    `{"entity_type_order":[1],"custom_sections":[{"id":"s1","label":"Lore"}],"custom_links":[{"id":"l1","label":"Wiki","url":"/wiki","icon":"fa-globe"}],"hidden_entity_ids":["e7"]}`,
			wantOK: true,
			wantItems: []SidebarItem{
				{Type: "category", TypeID: 1, Visible: true},
				{Type: "section", ID: "s1", Label: "Lore", Visible: true},
				{Type: "link", ID: "l1", Label: "Wiki", URL: "/wiki", Icon: "fa-globe", Visible: true},
			},
			wantHiddenE: []string{"e7"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := convertLegacySidebarConfig(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("converted = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if len(got.Items) != len(tt.wantItems) {
				t.Fatalf("items = %+v, want %+v", got.Items, tt.wantItems)
			}
			for i, w := range tt.wantItems {
				if got.Items[i] != w {
					t.Errorf("item[%d] = %+v, want %+v", i, got.Items[i], w)
				}
			}
			if len(tt.wantHiddenE) > 0 {
				if len(got.HiddenEntityIDs) != len(tt.wantHiddenE) || got.HiddenEntityIDs[0] != tt.wantHiddenE[0] {
					t.Errorf("hidden_entity_ids = %v, want %v", got.HiddenEntityIDs, tt.wantHiddenE)
				}
			}
		})
	}
}

// TestEnsureSidebarItems verifies the reconciler converts only legacy campaigns,
// back-writes the items model, and is a clean no-op on the second run.
func TestEnsureSidebarItems(t *testing.T) {
	// A stateful campaign set: writes are applied so a second sweep sees the
	// converted config (pins idempotency).
	store := map[string]string{
		"legacy": `{"entity_type_order":[2,1],"hidden_type_ids":[1]}`,
		"items":  `{"items":[{"type":"category","type_id":5,"visible":true}]}`,
		"empty":  `{}`,
	}
	order := []string{"legacy", "items", "empty"}

	repo := &mockCampaignRepo{
		listAllFn: func(_ context.Context, opts ListOptions) ([]Campaign, int, error) {
			// Single page: return everything on page 1, empty afterwards.
			if opts.Page > 1 {
				return nil, len(order), nil
			}
			out := make([]Campaign, 0, len(order))
			for _, id := range order {
				out = append(out, Campaign{ID: id, SidebarConfig: store[id]})
			}
			return out, len(order), nil
		},
		updateSidebarConfigFn: func(_ context.Context, campaignID, cfg string) error {
			store[campaignID] = cfg
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	// First run: exactly the one legacy campaign is converted.
	n, err := svc.EnsureSidebarItems(context.Background())
	if err != nil {
		t.Fatalf("EnsureSidebarItems: %v", err)
	}
	if n != 1 {
		t.Fatalf("converted = %d, want 1", n)
	}

	// The legacy campaign is now on items, order preserved, hidden carried.
	var got SidebarConfig
	if err := json.Unmarshal([]byte(store["legacy"]), &got); err != nil {
		t.Fatalf("unmarshal converted: %v", err)
	}
	want := []SidebarItem{
		{Type: "category", TypeID: 2, Visible: true},
		{Type: "category", TypeID: 1, Visible: false},
	}
	if len(got.Items) != len(want) {
		t.Fatalf("converted items = %+v, want %+v", got.Items, want)
	}
	for i, w := range want {
		if got.Items[i] != w {
			t.Errorf("item[%d] = %+v, want %+v", i, got.Items[i], w)
		}
	}

	// Second run: idempotent — nothing left to convert.
	n2, err := svc.EnsureSidebarItems(context.Background())
	if err != nil {
		t.Fatalf("EnsureSidebarItems (2nd): %v", err)
	}
	if n2 != 0 {
		t.Errorf("second run converted = %d, want 0 (idempotent)", n2)
	}
}
