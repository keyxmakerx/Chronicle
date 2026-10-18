package app

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

type scopeEntityByID map[string]string // entity id -> campaign id

func (m scopeEntityByID) GetByID(_ context.Context, id string) (*entities.Entity, error) {
	camp, ok := m[id]
	if !ok {
		return nil, apperror.NewNotFound("entity not found")
	}
	return &entities.Entity{ID: id, CampaignID: camp}, nil
}

// TestEntityInCampaign pins the binding every extension host function that
// takes an entity id goes through.
func TestEntityInCampaign(t *testing.T) {
	svc := scopeEntityByID{"own-ent": "camp-1", "foreign-ent": "camp-2"}
	tests := []struct {
		name     string
		campaign string
		eid      string
		wantOK   bool
	}{
		{"entity in the call's campaign", "camp-1", "own-ent", true},
		{"another campaign's entity", "camp-1", "foreign-ent", false},
		{"unknown entity", "camp-1", "missing-ent", false},
		{"no campaign context", "", "own-ent", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ent, err := entityInCampaign(context.Background(), svc, tc.campaign, tc.eid)
			if tc.wantOK {
				if err != nil || ent == nil || ent.ID != tc.eid {
					t.Fatalf("ent=%v err=%v, want %s", ent, err, tc.eid)
				}
				return
			}
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != http.StatusNotFound {
				t.Fatalf("err = %v, want 404", err)
			}
		})
	}
}
