package syncapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// Foundry writes a player's edit under the GM's key. Naming the member the
// edit is for makes Chronicle judge it as that member: picks that fail the
// hero-choice check are held and reported, the rest is stored.

type actingCampaignSvc struct {
	campaigns.CampaignService
	roles map[string]campaigns.Role
}

func (s actingCampaignSvc) GetMember(_ context.Context, campaignID, userID string) (*campaigns.CampaignMember, error) {
	r, ok := s.roles[userID]
	if !ok {
		return nil, nil
	}
	return &campaigns.CampaignMember{CampaignID: campaignID, UserID: userID, Role: r}, nil
}

func (s actingCampaignSvc) IsUserDmGranted(context.Context, string, string) (bool, error) {
	return false, nil
}

type actingEntitySvc struct {
	entities.EntityService
	merges []map[string]any
}

func (s *actingEntitySvc) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	owner := "pat"
	return &entities.Entity{ID: id, CampaignID: "camp-1", EntityTypeID: 1, OwnerUserID: &owner,
		FieldsData: map[string]any{"class": "Fury", "stamina": float64(20)}}, nil
}

func (s *actingEntitySvc) CheckEntityAccess(_ context.Context, _ string, role int, _ string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true, CanEdit: role >= int(campaigns.RoleScribe)}, nil
}

func (s *actingEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1", Fields: []entities.FieldDefinition{
		{Key: "class", Type: "text"}, {Key: "stamina", Type: "number"},
	}}, nil
}

func (s *actingEntitySvc) MergeFields(_ context.Context, _ string, p map[string]any) error {
	s.merges = append(s.merges, p)
	return nil
}

type fixedPlan struct{ plan *entities.HeroPlan }

func (f fixedPlan) HeroPlan(context.Context, string, string, bool) (*entities.HeroPlan, error) {
	return f.plan, nil
}

func TestUpdateEntityFields_ActingPlayer(t *testing.T) {
	plan := &entities.HeroPlan{Steps: []entities.HeroStep{{
		FieldKey: "class",
		Choices:  []entities.HeroChoice{{Name: "Fury"}, {Name: "Censor"}},
		Rules:    &entities.HeroStepRules{Limited: true},
	}}}
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer, "lee": campaigns.RolePlayer, "dir": campaigns.RoleOwner}
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantMerged map[string]any // nil: nothing stored
		wantHeld   []string
	}{
		{"player: off-list class held, number kept",
			`{"actingUserId":"pat","fields_data":{"class":"Wizard","stamina":30}}`,
			http.StatusOK, map[string]any{"stamina": float64(30)}, []string{"class"}},
		{"player: on-list class and number both kept",
			`{"actingUserId":"pat","fields_data":{"class":"Censor","stamina":30}}`,
			http.StatusOK, map[string]any{"class": "Censor", "stamina": float64(30)}, nil},
		{"player: only a held key stores nothing",
			`{"actingUserId":"pat","fields_data":{"class":"Wizard"}}`,
			http.StatusOK, nil, []string{"class"}},
		{"player: stored values resent are not held",
			`{"actingUserId":"pat","fields_data":{"class":"Fury","stamina":21}}`,
			http.StatusOK, map[string]any{"class": "Fury", "stamina": float64(21)}, nil},
		{"no author: unchanged, off-list class stored",
			`{"fields_data":{"class":"Wizard","stamina":30}}`,
			http.StatusOK, map[string]any{"class": "Wizard", "stamina": float64(30)}, nil},
		{"Director author: unchanged, off-list class stored",
			`{"actingUserId":"dir","fields_data":{"class":"Wizard"}}`,
			http.StatusOK, map[string]any{"class": "Wizard"}, nil},
		{"player who does not own the page is refused",
			`{"actingUserId":"lee","fields_data":{"stamina":30}}`,
			http.StatusForbidden, nil, nil},
		{"non-member author is refused",
			`{"actingUserId":"ghost","fields_data":{"stamina":30}}`,
			http.StatusForbidden, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &actingEntitySvc{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
			h.SetHeroPlanner(fixedPlan{plan})
			c, rec := gateContext(http.MethodPut, tt.body)
			err := h.UpdateEntityFields(c)
			if got := gateStatus(err); got != tt.wantStatus {
				t.Fatalf("status = %d (%v), want %d", got, err, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				if len(ents.merges) != 0 {
					t.Fatalf("a refused write stored %v", ents.merges)
				}
				return
			}
			if tt.wantMerged == nil && len(ents.merges) != 0 {
				t.Fatalf("stored %v, want nothing", ents.merges)
			}
			if tt.wantMerged != nil {
				if len(ents.merges) != 1 || len(ents.merges[0]) != len(tt.wantMerged) {
					t.Fatalf("stored %v, want %v", ents.merges, tt.wantMerged)
				}
				for k, v := range tt.wantMerged {
					if ents.merges[0][k] != v {
						t.Errorf("stored[%q] = %v, want %v", k, ents.merges[0][k], v)
					}
				}
			}
			var resp struct {
				Status string `json:"status"`
				Held   []struct{ Key, Reason string }
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Status != "ok" || len(resp.Held) != len(tt.wantHeld) {
				t.Fatalf("answer %s, want held %v", rec.Body.String(), tt.wantHeld)
			}
			for i, k := range tt.wantHeld {
				if resp.Held[i].Key != k || !strings.Contains(resp.Held[i].Reason, "not on the class list") {
					t.Errorf("held[%d] = %+v", i, resp.Held[i])
				}
			}
		})
	}
}

// levelBell records the level bells a write rang.
type levelBell struct {
	rang []string
}

func (b *levelBell) HeroChanged(context.Context, *entities.Entity, string, []entities.HeroPickChange) error {
	return nil
}

func (b *levelBell) LevelRaised(_ context.Context, e *entities.Entity, userID string, level int) error {
	b.rang = append(b.rang, fmt.Sprintf("%s:%s:%d", userID, e.ID, level))
	return nil
}

// leveledEntitySvc is a claimed hero at level 2 that has a level field.
type leveledEntitySvc struct{ actingEntitySvc }

func (s *leveledEntitySvc) GetByID(ctx context.Context, id string) (*entities.Entity, error) {
	e, _ := s.actingEntitySvc.GetByID(ctx, id)
	e.FieldsData["level"] = float64(2)
	return e, nil
}

func (s *leveledEntitySvc) GetEntityTypeByID(_ context.Context, id int) (*entities.EntityType, error) {
	return &entities.EntityType{ID: id, CampaignID: "camp-1", Fields: []entities.FieldDefinition{
		{Key: "class", Type: "text"}, {Key: "stamina", Type: "number"}, {Key: "level", Type: "number"},
	}}, nil
}

// A player who raises their own hero's level in Foundry rings the Directors'
// bell; the level-up still counts. Staff, the plain sync, and a level that
// stays or drops ring nothing.
func TestUpdateEntityFields_LevelRaisedBell(t *testing.T) {
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer, "dir": campaigns.RoleOwner, "scr": campaigns.RoleScribe}
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"the player raises their own hero", `{"actingUserId":"pat","fields_data":{"level":3}}`, []string{"pat:ent-1:3"}},
		{"the player raises it alongside other fields", `{"actingUserId":"pat","fields_data":{"level":4,"stamina":40}}`, []string{"pat:ent-1:4"}},
		{"the player resends the same level", `{"actingUserId":"pat","fields_data":{"level":2}}`, nil},
		{"the player lowers the level", `{"actingUserId":"pat","fields_data":{"level":1}}`, nil},
		{"the player changes something else", `{"actingUserId":"pat","fields_data":{"stamina":40}}`, nil},
		{"the owner raises it", `{"actingUserId":"dir","fields_data":{"level":3}}`, nil},
		{"a Scribe raises it", `{"actingUserId":"scr","fields_data":{"level":3}}`, nil},
		{"the plain sync raises it", `{"fields_data":{"level":3}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &leveledEntitySvc{}
			bell := &levelBell{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
			h.SetHeroChanges(entities.NewHeroChangeService(nil, nil, bell))
			c, _ := gateContext(http.MethodPut, tt.body)
			if err := h.UpdateEntityFields(c); err != nil {
				t.Fatalf("write refused: %v", err)
			}
			if len(ents.merges) != 1 {
				t.Fatalf("stored %v, want one write", ents.merges)
			}
			if fmt.Sprint(bell.rang) != fmt.Sprint(tt.want) {
				t.Fatalf("rang %v, want %v", bell.rang, tt.want)
			}
		})
	}
}
