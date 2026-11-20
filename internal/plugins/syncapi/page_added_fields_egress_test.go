package syncapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// pageAddedFixtures is a page whose category restricts nothing, with a GM-only
// field added to just this page.
func pageAddedFixtures() (*entities.Entity, *entities.EntityType) {
	et := &entities.EntityType{ID: 7, Fields: []entities.FieldDefinition{{Key: "might", Label: "Might"}}}
	ent := &entities.Entity{
		ID: "e1", CampaignID: "camp-1", EntityTypeID: 7,
		FieldsData: map[string]any{"might": 2, "secret_weakness": "silver"},
		FieldOverrides: &entities.FieldOverrides{Added: []entities.FieldDefinition{
			{Key: "secret_weakness", Label: "Secret weakness", Type: "text", GMOnly: true},
		}},
	}
	return ent, et
}

// TestEgress_StripsPageAddedGMFields pins that the REST API's single and list
// reads withhold a page-added GM-only value from anyone below the owner.
func TestEgress_StripsPageAddedGMFields(t *testing.T) {
	for _, tc := range gmViewerCases {
		for _, route := range []string{"get", "list"} {
			t.Run(route+"/"+tc.name, func(t *testing.T) {
				ent, et := pageAddedFixtures()
				h := NewAPIHandler(nil, &stubEntityServiceForGM{entity: ent, etype: et}, &stubCampaignSvcForGM{role: tc.role}, nil)
				var fd map[string]any
				if route == "get" {
					c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities/e1", "e1", tc.bearer)
					if err := h.GetEntity(c); err != nil {
						t.Fatalf("GetEntity: %v", err)
					}
					var resp map[string]any
					if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
						t.Fatalf("decode: %v", err)
					}
					fd, _ = resp["fields_data"].(map[string]any)
				} else {
					c, rec := gmContext(http.MethodGet, "/api/v1/campaigns/camp-1/entities", "", tc.bearer)
					if err := h.ListEntities(c); err != nil {
						t.Fatalf("ListEntities: %v", err)
					}
					var resp map[string]any
					if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
						t.Fatalf("decode: %v", err)
					}
					data, _ := resp["data"].([]any)
					if len(data) != 1 {
						t.Fatalf("want 1 entity, got %d", len(data))
					}
					item, _ := data[0].(map[string]any)
					fd, _ = item["fields_data"].(map[string]any)
				}
				if _, ok := fd["might"]; !ok {
					t.Errorf("open field missing: %v", fd)
				}
				if _, ok := fd["secret_weakness"]; ok != tc.wantGM {
					t.Errorf("secret_weakness present=%v, want %v", ok, tc.wantGM)
				}
			})
		}
	}
}
