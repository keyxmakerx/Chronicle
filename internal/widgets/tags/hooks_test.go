package tags

import (
	"context"
	"testing"
)

// recordingTagHooks records the tag hooks SetEntityTags fires.
type recordingTagHooks struct {
	added, removed []string
}

func (h *recordingTagHooks) TagAdded(_ context.Context, campaignID, entityID string, t Tag) {
	h.added = append(h.added, campaignID+"/"+entityID+"/"+t.Name)
}

func (h *recordingTagHooks) TagRemoved(_ context.Context, campaignID, entityID string, t Tag) {
	h.removed = append(h.removed, campaignID+"/"+entityID+"/"+t.Name)
}

// SetEntityTags tells extension plugins about each tag it actually adds or
// removes, and nothing about tags it leaves alone.
func TestSetEntityTags_FiresHooks(t *testing.T) {
	repo := &mockTagRepo{
		listByCampaignFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10, Name: "npc"}, {ID: 20, Name: "ally"}, {ID: 30, Name: "dead"}}, nil
		},
		getEntityTagsFn: func(_ context.Context, _ string, _ bool) ([]Tag, error) {
			return []Tag{{ID: 10, Name: "npc"}, {ID: 20, Name: "ally"}}, nil
		},
		addTagToEntityFn:      func(context.Context, string, int) error { return nil },
		removeTagFromEntityFn: func(context.Context, string, int) error { return nil },
	}
	svc := newTestService(repo)
	hooks := &recordingTagHooks{}
	svc.SetHookDispatcher(hooks)

	if err := svc.SetEntityTags(context.Background(), "e1", "c1", []int{10, 30}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hooks.added) != 1 || hooks.added[0] != "c1/e1/dead" {
		t.Errorf("added = %v, want [c1/e1/dead]", hooks.added)
	}
	if len(hooks.removed) != 1 || hooks.removed[0] != "c1/e1/ally" {
		t.Errorf("removed = %v, want [c1/e1/ally]", hooks.removed)
	}
}
