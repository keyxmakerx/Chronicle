package syncapi

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// gmOnlyEntitySvc is a claimed hero whose category has a GM-only field, and
// whose page adds another.
type gmOnlyEntitySvc struct{ actingEntitySvc }

func (s *gmOnlyEntitySvc) GetByID(ctx context.Context, id string) (*entities.Entity, error) {
	e, _ := s.actingEntitySvc.GetByID(ctx, id)
	e.FieldsData["gm_notes"] = "Betrays the party"
	e.FieldOverrides = &entities.FieldOverrides{Added: []entities.FieldDefinition{{Key: "doom", Type: "number", GMOnly: true}}}
	return e, nil
}

func (s *gmOnlyEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1", Fields: []entities.FieldDefinition{
		{Key: "class", Type: "text"}, {Key: "stamina", Type: "number"}, {Key: "gm_notes", Type: "text", GMOnly: true},
	}}, nil
}

// A player cannot see a GM-only field, so their sheet write through Foundry
// may not set it either: it is held, the rest is stored. A Director's write
// and a value sent back unchanged are not held.
func TestUpdateEntityFields_ActingPlayerGMOnlyHeld(t *testing.T) {
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer, "dir": campaigns.RoleOwner}
	for _, tc := range []struct {
		name       string
		body       string
		wantMerged []string
		wantHeld   []string
	}{
		{"player sets GM-only fields", `{"actingUserId":"pat","fields_data":{"gm_notes":"Loyal","doom":9,"stamina":30}}`, []string{"stamina"}, []string{"doom", "gm_notes"}},
		{"player resends the stored value", `{"actingUserId":"pat","fields_data":{"gm_notes":"Betrays the party","stamina":30}}`, []string{"gm_notes", "stamina"}, nil},
		{"Director sets it", `{"actingUserId":"dir","fields_data":{"gm_notes":"Loyal"}}`, []string{"gm_notes"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ents := &gmOnlyEntitySvc{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
			h.SetHeroPlanner(fixedPlan{&entities.HeroPlan{}})
			c, rec := gateContext(http.MethodPut, tc.body)
			if err := h.UpdateEntityFields(c); err != nil {
				t.Fatalf("UpdateEntityFields: %v", err)
			}
			var stored []string
			if len(ents.merges) == 1 {
				for k := range ents.merges[0] {
					stored = append(stored, k)
				}
			}
			if strings.Join(slices.Sorted(slices.Values(stored)), ",") != strings.Join(tc.wantMerged, ",") {
				t.Errorf("stored %v, want %v", ents.merges, tc.wantMerged)
			}
			var resp struct {
				Held []struct{ Key, Reason string }
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			var held []string
			for _, h := range resp.Held {
				held = append(held, h.Key)
				if !strings.Contains(h.Reason, "GM's to change") {
					t.Errorf("held %q reason %q", h.Key, h.Reason)
				}
			}
			if strings.Join(slices.Sorted(slices.Values(held)), ",") != strings.Join(tc.wantHeld, ",") {
				t.Errorf("held %v, want %v", held, tc.wantHeld)
			}
		})
	}
}
