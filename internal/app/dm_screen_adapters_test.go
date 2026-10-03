package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// revealEntitySvc stubs the EntityService calls Reveal makes and counts
// writes, so the test can prove Reveal never hides, never crosses campaigns
// and never touches types the panel doesn't list.
type revealEntitySvc struct {
	entities.EntityService
	ent     *entities.Entity
	toggles int
	hides   int
}

func (s *revealEntitySvc) GetByID(context.Context, string) (*entities.Entity, error) {
	return s.ent, nil
}

func (s *revealEntitySvc) GetEntityTypes(context.Context, string) ([]entities.EntityType, error) {
	return []entities.EntityType{{ID: 1, Slug: "character", Enabled: true}, {ID: 2, Slug: "location", Enabled: true}}, nil
}

func (s *revealEntitySvc) SetPrivateInCampaign(_ context.Context, _, _ string, private bool) error {
	if private {
		s.hides++
	}
	s.toggles++
	return nil
}

func TestDMHiddenAdapter_RevealOnlyReveals(t *testing.T) {
	tests := []struct {
		name        string
		ent         entities.Entity
		campaign    string
		wantErr     bool
		wantToggles int
	}{
		{"hidden in this campaign is revealed", entities.Entity{ID: "e1", CampaignID: "c1", Name: "Vosk", EntityTypeID: 1, IsPrivate: true}, "c1", false, 1},
		{"already visible is left alone", entities.Entity{ID: "e1", CampaignID: "c1", Name: "Vosk", EntityTypeID: 1}, "c1", false, 0},
		{"another campaign's entity is refused", entities.Entity{ID: "e1", CampaignID: "c2", Name: "Vosk", IsPrivate: true}, "c1", true, 0},
		{"a hidden location is refused", entities.Entity{ID: "e1", CampaignID: "c1", Name: "Vosk", EntityTypeID: 2, IsPrivate: true}, "c1", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &revealEntitySvc{ent: &tt.ent}
			name, err := (&dmHiddenAdapter{entities: svc}).Reveal(context.Background(), "e1", tt.campaign)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if svc.hides != 0 {
				t.Fatalf("Reveal hid the entity")
			}
			if svc.toggles != tt.wantToggles {
				t.Fatalf("toggles = %d, want %d", svc.toggles, tt.wantToggles)
			}
			if !tt.wantErr && name != "Vosk" {
				t.Fatalf("name = %q", name)
			}
		})
	}
}

func TestNightWhen(t *testing.T) {
	if got := nightWhen("2026-10-09", "19:30"); got != "Fri 9 Oct, 19:30" {
		t.Fatalf("got %q", got)
	}
	if got := nightWhen("not-a-date", ""); got != "not-a-date" {
		t.Fatalf("got %q", got)
	}
}
