package app

import (
	"slices"
	"time"

	"context"
	"github.com/keyxmakerx/chronicle/internal/plugins/dmscreen"
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
			name, err := (&dmHiddenAdapter{entities: svc, lists: fixedTypeLists{npc: []int{1}}}).Reveal(context.Background(), "e1", tt.campaign)
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

// hiddenListSvc answers List per type from a fixed table and records the
// options it was given.
type hiddenListSvc struct {
	entities.EntityService
	byType map[int][]entities.Entity
	opts   []entities.ListOptions
}

func (s *hiddenListSvc) List(_ context.Context, _ string, typeID, _ int, _ string, o entities.ListOptions) ([]entities.Entity, int, error) {
	s.opts = append(s.opts, o)
	l := s.byType[typeID]
	if len(l) > o.PerPage {
		l = l[:o.PerPage]
	}
	return l, len(s.byType[typeID]), nil
}

func TestDMHiddenAdapter_HiddenCharacters(t *testing.T) {
	mk := func(id string, age int) entities.Entity {
		return entities.Entity{ID: id, Name: id, IsPrivate: true, UpdatedAt: time.Unix(int64(1000-age), 0)}
	}
	tests := []struct {
		name     string
		byType   map[int][]entities.Entity
		limit    int
		wantIDs  []string
		wantMore bool
	}{
		{"fewer than the limit shows all", map[int][]entities.Entity{1: {mk("a", 1)}, 2: {mk("b", 2)}}, 3, []string{"a", "b"}, false},
		{"exactly the limit has no more", map[int][]entities.Entity{1: {mk("a", 1), mk("b", 2)}}, 2, []string{"a", "b"}, false},
		{"one past the limit reports more", map[int][]entities.Entity{1: {mk("a", 1), mk("b", 2), mk("c", 3)}}, 2, []string{"a", "b"}, true},
		{"newest across types wins", map[int][]entities.Entity{1: {mk("old", 50)}, 2: {mk("new", 1), mk("mid", 9)}}, 2, []string{"new", "mid"}, true},
		{"an entity listed under two types counts once", map[int][]entities.Entity{1: {mk("a", 1)}, 2: {mk("a", 1)}}, 2, []string{"a"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &hiddenListSvc{byType: tt.byType}
			ad := &dmHiddenAdapter{entities: svc, lists: fixedTypeLists{npc: []int{1, 2}}}
			got, more, err := ad.HiddenCharacters(context.Background(), "c1", dmscreen.Viewer{Role: 3}, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, h := range got {
				ids = append(ids, h.ID)
			}
			if !slices.Equal(ids, tt.wantIDs) || more != tt.wantMore {
				t.Errorf("got %v more=%v, want %v more=%v", ids, more, tt.wantIDs, tt.wantMore)
			}
			for _, o := range svc.opts {
				if !o.PrivateOnly {
					t.Error("must ask the service for hidden entities only")
				}
			}
		})
	}
}
