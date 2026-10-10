// token_position_visibility_test.go pins that the fast-drag token-move path
// carries is_hidden to the event publisher, mirroring the gate
// PublishTokenEvent already applies to create/update/delete. Without this, a
// hidden token's live drag position reached every connected client instead
// of GMs only.
package maps

import (
	"context"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/patch"
)

// recordingEventPublisher records the isHidden flag PublishTokenPositionEvent
// was called with, leaving every other MapEventPublisher method a no-op.
type recordingEventPublisher struct {
	NoopMapEventPublisher
	called   bool
	isHidden bool
}

func (p *recordingEventPublisher) PublishTokenPositionEvent(_, _, _ string, _, _ float64, isHidden bool) {
	p.called = true
	p.isHidden = isHidden
}

func TestUpdateTokenPosition_PublishesHiddenFlag(t *testing.T) {
	tests := []struct {
		name     string
		isHidden bool
	}{
		{"hidden token", true},
		{"visible token", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockDrawingRepo{
				getTokenFn: func(_ context.Context, id string) (*Token, error) {
					return &Token{ID: id, MapID: "map-1", IsHidden: tt.isHidden}, nil
				},
			}
			pub := &recordingEventPublisher{}
			svc := NewDrawingService(repo)
			svc.SetEventPublisher(pub)

			err := svc.UpdateTokenPosition(context.Background(), "tok-1", "map-1", true, UpdateTokenPositionInput{X: patch.Of(float64(10)), Y: patch.Of(float64(20))})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !pub.called {
				t.Fatal("expected PublishTokenPositionEvent to be called")
			}
			if pub.isHidden != tt.isHidden {
				t.Errorf("isHidden = %v, want %v", pub.isHidden, tt.isHidden)
			}
		})
	}
}
