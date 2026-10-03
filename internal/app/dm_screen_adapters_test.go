package app

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/entities"
)

// revealEntitySvc stubs the two EntityService calls Reveal makes and counts
// toggles, so the test can prove Reveal never hides or crosses campaigns.
type revealEntitySvc struct {
	entities.EntityService
	ent     *entities.Entity
	toggles int
}

func (s *revealEntitySvc) GetByID(context.Context, string) (*entities.Entity, error) {
	return s.ent, nil
}

func (s *revealEntitySvc) TogglePrivateInCampaign(context.Context, string, string) (bool, error) {
	s.toggles++
	return false, nil
}

func TestDMHiddenAdapter_RevealOnlyReveals(t *testing.T) {
	tests := []struct {
		name        string
		ent         entities.Entity
		campaign    string
		wantErr     bool
		wantToggles int
	}{
		{"hidden in this campaign is revealed", entities.Entity{ID: "e1", CampaignID: "c1", Name: "Vosk", IsPrivate: true}, "c1", false, 1},
		{"already visible is left alone", entities.Entity{ID: "e1", CampaignID: "c1", Name: "Vosk"}, "c1", false, 0},
		{"another campaign's entity is refused", entities.Entity{ID: "e1", CampaignID: "c2", Name: "Vosk", IsPrivate: true}, "c1", true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &revealEntitySvc{ent: &tt.ent}
			name, err := (&dmHiddenAdapter{entities: svc}).Reveal(context.Background(), "e1", tt.campaign)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
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
