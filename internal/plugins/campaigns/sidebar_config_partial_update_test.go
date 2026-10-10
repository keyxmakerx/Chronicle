// sidebar_config_partial_update_test.go pins the partial-update contract for
// PUT /campaigns/:id/sidebar-config: a body naming one list leaves the others
// as stored, an empty list clears, and a null preserves (none of the lists can
// be NULL).
package campaigns

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestUpdateSidebarConfig_PartialBody(t *testing.T) {
	stored := `{"items":[{"type":"app","slug":"maps","visible":true}],"hidden_entity_ids":["e1"],"hidden_node_ids":["n1"]}`
	tests := []struct {
		name       string
		body       string
		wantItems  int
		wantEntity []string
		wantNode   []string
	}{
		{"empty body preserves everything", `{}`, 1, []string{"e1"}, []string{"n1"}},
		{"entities only leaves items and nodes", `{"hidden_entity_ids":["e2","e3"]}`, 1, []string{"e2", "e3"}, []string{"n1"}},
		{"empty list clears just that list", `{"hidden_node_ids":[]}`, 1, []string{"e1"}, []string{}},
		{"null preserves", `{"items":null,"hidden_entity_ids":null,"hidden_node_ids":null}`, 1, []string{"e1"}, []string{"n1"}},
		{"items replace and leave the hidden sets", `{"items":[{"type":"app","slug":"notes","visible":true},{"type":"app","slug":"maps","visible":true}]}`, 2, []string{"e1"}, []string{"n1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved string
			svc := &campaignService{repo: &mockCampaignRepo{
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id, SidebarConfig: stored}, nil
				},
				updateSidebarConfigFn: func(_ context.Context, _, cfg string) error { saved = cfg; return nil },
			}}
			var req UpdateSidebarConfigRequest
			if err := json.Unmarshal([]byte(tt.body), &req); err != nil {
				t.Fatalf("bad test body: %v", err)
			}
			if err := svc.UpdateSidebarConfig(context.Background(), "camp-1", req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var got SidebarConfig
			if err := json.Unmarshal([]byte(saved), &got); err != nil {
				t.Fatalf("saved config is not JSON: %v (%q)", err, saved)
			}
			if len(got.Items) != tt.wantItems {
				t.Errorf("items = %d, want %d", len(got.Items), tt.wantItems)
			}
			if !reflect.DeepEqual(nonNil(got.HiddenEntityIDs), tt.wantEntity) {
				t.Errorf("hidden entities = %v, want %v", got.HiddenEntityIDs, tt.wantEntity)
			}
			if !reflect.DeepEqual(nonNil(got.HiddenNodeIDs), tt.wantNode) {
				t.Errorf("hidden nodes = %v, want %v", got.HiddenNodeIDs, tt.wantNode)
			}
		})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
