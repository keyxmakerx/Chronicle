package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// buyerEntitySvc stubs the two EntityService calls the buyer check makes.
type buyerEntitySvc struct {
	entities.EntityService
	ent *entities.Entity
}

func (s buyerEntitySvc) GetByID(_ context.Context, _ string) (*entities.Entity, error) {
	if s.ent == nil {
		return nil, apperror.NewNotFound("entity not found")
	}
	return s.ent, nil
}

func (s buyerEntitySvc) CheckEntityAccess(_ context.Context, _ string, _ int, _ string) (*entities.EffectivePermission, error) {
	return &entities.EffectivePermission{CanView: true, CanEdit: true}, nil
}

// TestArmoryBuyerAccess_CampaignScoped pins that a campaign role never lets
// a user buy as a character from another campaign.
func TestArmoryBuyerAccess_CampaignScoped(t *testing.T) {
	tests := []struct {
		name string
		ent  *entities.Entity
		want bool
	}{
		{"same campaign", &entities.Entity{ID: "pc", CampaignID: "camp-1"}, true},
		{"other campaign", &entities.Entity{ID: "pc", CampaignID: "camp-2"}, false},
		{"missing entity", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &armoryBuyerAccessAdapter{svc: buyerEntitySvc{ent: tt.ent}}
			got, err := a.CanUserActAsBuyer(context.Background(), "camp-1", "pc", "user-1", 3)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
