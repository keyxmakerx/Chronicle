// toggle_private_idor_test.go — SEC-IDOR-1 cross-campaign guard for the
// visibility toggle. TogglePrivateInCampaign is the campaign-scoped entry point
// the npcs reveal toggle uses; it must refuse to flip is_private on an entity
// that lives in another campaign, and must never reach the write when it does.
package entities

import (
	"context"
	"net/http"
	"testing"
)

// TestTogglePrivateInCampaign_CrossCampaignRejected: an entity that belongs to
// "camp-owner" must not be toggled through a request scoped to "camp-attacker".
// The service returns NotFound (existence not leaked) and never calls
// UpdatePrivate.
func TestTogglePrivateInCampaign_CrossCampaignRejected(t *testing.T) {
	updated := false
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "camp-owner", IsPrivate: false}, nil
		},
		updatePrivateFn: func(_ context.Context, _ string, _ bool) error {
			updated = true
			return nil
		},
	}
	svc := newTestService(entityRepo, &mockEntityTypeRepo{})

	_, err := svc.TogglePrivateInCampaign(context.Background(), "ent-1", "camp-attacker")
	assertAppError(t, err, http.StatusNotFound)
	if updated {
		t.Error("cross-campaign toggle must short-circuit before the UpdatePrivate write")
	}
}

// TestTogglePrivateInCampaign_SameCampaignAllowed: the happy path — when the
// entity belongs to the caller's campaign, the flag flips and the new state is
// returned. Regression guard so the campaign check doesn't block legit reveals.
func TestTogglePrivateInCampaign_SameCampaignAllowed(t *testing.T) {
	updated := false
	var setTo bool
	entityRepo := &mockEntityRepo{
		findByIDFn: func(_ context.Context, id string) (*Entity, error) {
			return &Entity{ID: id, CampaignID: "camp-owner", IsPrivate: false}, nil
		},
		updatePrivateFn: func(_ context.Context, _ string, isPrivate bool) error {
			updated = true
			setTo = isPrivate
			return nil
		},
	}
	svc := newTestService(entityRepo, &mockEntityTypeRepo{})

	newPrivate, err := svc.TogglePrivateInCampaign(context.Background(), "ent-1", "camp-owner")
	if err != nil {
		t.Fatalf("same-campaign toggle should succeed, got: %v", err)
	}
	if !updated {
		t.Error("same-campaign toggle should have reached UpdatePrivate")
	}
	if !setTo || !newPrivate {
		t.Errorf("expected toggle to flip is_private false→true, got setTo=%v newPrivate=%v", setTo, newPrivate)
	}
}
