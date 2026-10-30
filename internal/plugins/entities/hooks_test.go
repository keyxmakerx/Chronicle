package entities

import (
	"context"
	"testing"
)

// recordingEntityHooks records each hook call with the context it carried.
type recordingEntityHooks struct {
	calls []string
	ctxs  []context.Context
}

func (h *recordingEntityHooks) EntityChanged(ctx context.Context, eventType, campaignID string, e *Entity) {
	h.calls = append(h.calls, eventType+":"+campaignID+":"+e.ID)
	h.ctxs = append(h.ctxs, ctx)
}

type hookCtxKey struct{}

// Every page write that reaches the live-sync bus also reaches extension
// plugins, with the caller's context so the write's origin travels along.
func TestEntityHooks_FollowPublishedEvents(t *testing.T) {
	tests := []struct {
		name  string
		write func(ctx context.Context, svc EntityService) error
		want  string
	}{
		{"update fields", func(ctx context.Context, svc EntityService) error {
			return svc.UpdateFields(ctx, "ent-1", map[string]any{"hp": 3})
		}, "updated:camp-1:ent-1"},
		{"delete", func(ctx context.Context, svc EntityService) error {
			return svc.Delete(ctx, "ent-1")
		}, "deleted:camp-1:ent-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, _ string) (*Entity, error) {
					return &Entity{ID: "ent-1", CampaignID: "camp-1", Name: "Tyne"}, nil
				},
			}
			pub := &capturingEntityEventPublisher{}
			hooks := &recordingEntityHooks{}
			svc := NewEntityService(repo, &mockEntityTypeRepo{}, &mockPermissionRepo{})
			svc.SetEventPublisher(pub)
			svc.SetHookDispatcher(hooks)

			ctx := context.WithValue(context.Background(), hookCtxKey{}, "origin")
			if err := tt.write(ctx, svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(hooks.calls) != len(pub.events) {
				t.Fatalf("hooks %v, published %d events; want the same count", hooks.calls, len(pub.events))
			}
			if len(hooks.calls) != 1 || hooks.calls[0] != tt.want {
				t.Fatalf("hooks = %v, want [%s]", hooks.calls, tt.want)
			}
			if hooks.ctxs[0].Value(hookCtxKey{}) != "origin" {
				t.Error("hook did not receive the caller's context")
			}
		})
	}
}
