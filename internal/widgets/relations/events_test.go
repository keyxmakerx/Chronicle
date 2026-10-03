package relations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// capturePublisher records every relation event as "type source->target".
type capturePublisher struct{ got []string }

func (c *capturePublisher) PublishRelationEvent(eventType string, rel *Relation) {
	c.got = append(c.got, fmt.Sprintf("%s %s->%s", eventType, rel.SourceEntityID, rel.TargetEntityID))
}

func withPublisher(repo *mockRelationRepo) (RelationService, *capturePublisher) {
	svc := NewRelationService(repo)
	pub := &capturePublisher{}
	svc.SetEventPublisher(pub)
	return svc, pub
}

// Every relation row written is announced once, after the write, so a
// listener on either entity (a Foundry character's inventory) sees it.
func TestRelationEvents(t *testing.T) {
	ctx := context.Background()
	reverse := &Relation{ID: 2, CampaignID: "camp-1", SourceEntityID: "item", TargetEntityID: "hero", RelationType: "In Inventory Of"}
	forward := &Relation{ID: 1, CampaignID: "camp-1", SourceEntityID: "hero", TargetEntityID: "item", RelationType: "Has Item", ReverseRelationType: "In Inventory Of"}

	tests := []struct {
		name string
		repo *mockRelationRepo
		run  func(RelationService) error
		want []string
	}{
		{
			name: "create announces both rows",
			repo: &mockRelationRepo{},
			run: func(s RelationService) error {
				_, err := s.Create(ctx, "camp-1", "hero", "item", "Has Item", "In Inventory Of", "u1", nil)
				return err
			},
			want: []string{"created item->hero", "created hero->item"},
		},
		{
			name: "create with an existing reverse announces only the forward row",
			repo: &mockRelationRepo{createFn: func(_ context.Context, rel *Relation) error {
				if rel.SourceEntityID == "item" {
					return apperror.NewConflict("duplicate relation")
				}
				rel.ID = 1
				return nil
			}},
			run: func(s RelationService) error {
				_, err := s.Create(ctx, "camp-1", "hero", "item", "Has Item", "In Inventory Of", "u1", nil)
				return err
			},
			want: []string{"created hero->item"},
		},
		{
			name: "a failed forward write announces nothing",
			repo: &mockRelationRepo{createFn: func(context.Context, *Relation) error { return errors.New("db down") }},
			run: func(s RelationService) error {
				_, err := s.Create(ctx, "camp-1", "hero", "item", "Has Item", "In Inventory Of", "u1", nil)
				if err == nil {
					return errors.New("expected an error")
				}
				return nil
			},
			want: nil,
		},
		{
			name: "delete announces both rows",
			repo: &mockRelationRepo{
				findByIDFn:    func(context.Context, int) (*Relation, error) { return forward, nil },
				findReverseFn: func(context.Context, string, string, string) (*Relation, error) { return reverse, nil },
			},
			run:  func(s RelationService) error { return s.Delete(ctx, 1) },
			want: []string{"deleted item->hero", "deleted hero->item"},
		},
		{
			name: "a failed delete announces nothing",
			repo: &mockRelationRepo{
				findByIDFn: func(context.Context, int) (*Relation, error) { return forward, nil },
				deleteFn:   func(context.Context, int) error { return errors.New("db down") },
			},
			run: func(s RelationService) error {
				if s.Delete(ctx, 1) == nil {
					return errors.New("expected an error")
				}
				return nil
			},
			want: nil,
		},
		{
			name: "metadata update announces the row",
			repo: &mockRelationRepo{findByIDFn: func(context.Context, int) (*Relation, error) { return forward, nil }},
			run:  func(s RelationService) error { return s.UpdateMetadata(ctx, 1, json.RawMessage(`{"quantity":2}`)) },
			want: []string{"metadata_updated hero->item"},
		},
		{
			name: "a conditional update that wrote announces the row",
			repo: &mockRelationRepo{findByIDFn: func(context.Context, int) (*Relation, error) { return forward, nil }},
			run: func(s RelationService) error {
				_, err := s.UpdateMetadataIf(ctx, 1, json.RawMessage(`{}`), json.RawMessage(`{"quantity":2}`))
				return err
			},
			want: []string{"metadata_updated hero->item"},
		},
		{
			name: "a conditional update that lost the race announces nothing",
			repo: &mockRelationRepo{updateIfFn: func() (bool, error) { return false, nil }},
			run: func(s RelationService) error {
				_, err := s.UpdateMetadataIf(ctx, 1, json.RawMessage(`{}`), json.RawMessage(`{"quantity":2}`))
				return err
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, pub := withPublisher(tt.repo)
			if err := tt.run(svc); err != nil {
				t.Fatal(err)
			}
			if fmt.Sprint(pub.got) != fmt.Sprint(tt.want) {
				t.Errorf("events = %v, want %v", pub.got, tt.want)
			}
		})
	}
}

// Without a publisher (tests, tools) writes stay silent and do not re-read.
func TestRelationEvents_NoPublisher(t *testing.T) {
	reads := 0
	repo := &mockRelationRepo{findByIDFn: func(context.Context, int) (*Relation, error) { reads++; return &Relation{}, nil }}
	if err := NewRelationService(repo).UpdateMetadata(context.Background(), 1, json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Errorf("re-read %d times without a publisher", reads)
	}
}
