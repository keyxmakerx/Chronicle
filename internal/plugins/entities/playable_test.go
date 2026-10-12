package entities

import (
	"context"
	"testing"
)

// TestSetPlayable pins the gates and side effects of marking a character
// playable: wrong-campaign pages look absent, the addon gates it, only
// claimable types can be marked, and a no-op never writes or publishes.
func TestSetPlayable(t *testing.T) {
	character := &EntityType{ID: 7, Slug: "character"}
	location := &EntityType{ID: 5, Slug: "location"}

	tests := []struct {
		name       string
		addonOn    bool
		campaign   string
		typ        *EntityType
		current    bool
		set        bool
		wantCode   int
		wantWrite  bool
		wantEvents int
	}{
		{"addon off", false, "c1", character, false, true, 403, false, 0},
		{"wrong campaign", true, "other", character, false, true, 404, false, 0},
		{"non-claimable type cannot be marked", true, "c1", location, false, true, 400, false, 0},
		{"mark", true, "c1", character, false, true, 0, true, 1},
		{"unmark", true, "c1", character, true, false, 0, true, 1},
		{"unmark on a non-claimable type is allowed", true, "c1", location, true, false, 0, true, 1},
		{"no-op when already marked", true, "c1", character, true, true, 0, false, 0},
		{"no-op when already unmarked", true, "c1", character, false, false, 0, false, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ent := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: tc.typ.ID, Playable: tc.current}
			var wrote bool
			var wroteVal bool
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil },
				setPlayableFn: func(_ context.Context, _ string, p bool) error {
					wrote, wroteVal = true, p
					return nil
				},
			}
			typeRepo := &mockEntityTypeRepo{
				findByIDFn: func(_ context.Context, _ int) (*EntityType, error) { return tc.typ, nil },
			}
			svc := newTestService(repo, typeRepo)
			svc.SetAddonChecker(&mockAddonChecker{enabled: map[string]bool{AddonPlayerCharacterClaiming: tc.addonOn}})
			pub := &capturingEntityEventPublisher{}
			svc.SetEventPublisher(pub)

			got, err := svc.SetPlayable(context.Background(), tc.campaign, "e1", tc.set)
			if tc.wantCode != 0 {
				assertAppError(t, err, tc.wantCode)
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got.Playable != tc.set {
					t.Errorf("returned Playable = %v, want %v", got.Playable, tc.set)
				}
			}
			if wrote != tc.wantWrite {
				t.Errorf("SetPlayable written = %v, want %v", wrote, tc.wantWrite)
			}
			if wrote && wroteVal != tc.set {
				t.Errorf("wrote %v, want %v", wroteVal, tc.set)
			}
			if len(pub.events) != tc.wantEvents {
				t.Fatalf("events = %d, want %d", len(pub.events), tc.wantEvents)
			}
			if tc.wantEvents == 1 && (pub.events[0].eventType != "updated" || pub.events[0].entity.Playable != tc.set) {
				t.Errorf("event = %+v", pub.events[0])
			}
		})
	}
}

// TestAssignOwnerAndClaim_MarkPlayable pins that an owner implies playable in
// the returned and published entity, and that clearing an owner does not
// silently unmark it.
func TestAssignOwnerAndClaim_MarkPlayable(t *testing.T) {
	character := &EntityType{ID: 7, Slug: "character"}
	typeRepo := &mockEntityTypeRepo{
		findByIDFn: func(_ context.Context, _ int) (*EntityType, error) { return character, nil },
	}
	tests := []struct {
		name     string
		do       func(svc EntityService) (*Entity, error)
		startOwn *string
		startPl  bool
		want     bool
	}{
		{"assign owner marks", func(s EntityService) (*Entity, error) {
			return s.AssignOwner(context.Background(), "e1", strPtr("p1"))
		}, nil, false, true},
		{"claim marks", func(s EntityService) (*Entity, error) {
			return s.ClaimEntity(context.Background(), "e1", "p1")
		}, nil, false, true},
		{"unclaim keeps the mark", func(s EntityService) (*Entity, error) {
			return s.AssignOwner(context.Background(), "e1", nil)
		}, strPtr("p1"), true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ent := &Entity{ID: "e1", CampaignID: "c1", EntityTypeID: 7, OwnerUserID: tc.startOwn, Playable: tc.startPl}
			repo := &mockEntityRepo{findByIDFn: func(_ context.Context, _ string) (*Entity, error) { return ent, nil }}
			svc := newTestService(repo, typeRepo)
			pub := &capturingEntityEventPublisher{}
			svc.SetEventPublisher(pub)
			got, err := tc.do(svc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Playable != tc.want {
				t.Errorf("Playable = %v, want %v", got.Playable, tc.want)
			}
			if len(pub.events) != 1 || pub.events[0].entity.Playable != tc.want {
				t.Errorf("published events = %+v", pub.events)
			}
		})
	}
}
