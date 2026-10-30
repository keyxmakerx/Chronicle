package connector

import (
	"context"
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/aiexport"
)

// Losing a campaign ends the key for good: if ownership comes back, or the
// campaign is restored, the old key stays dead, where a check on each call
// alone would let it work again.
func TestEndKeys_OwnershipReturningDoesNotRevive(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		end  func(svc *Service) error
	}{
		{"handed over", func(svc *Service) error { return svc.EndKeysMadeBy(ctx, "camp-1", "owner-1") }},
		{"moved to the Trash", func(svc *Service) error { return svc.Disconnect(ctx, "camp-1") }},
		{"sign-ins ended", func(svc *Service) error { return svc.EndAllKeysMadeBy(ctx, "owner-1") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, own, _ := newTestService()
			raw, _, err := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
			if err != nil {
				t.Fatal(err)
			}
			own["camp-1"] = "owner-2"
			if err := tc.end(svc); err != nil {
				t.Fatal(err)
			}
			own["camp-1"] = "owner-1"
			if _, _, err := svc.Authenticate(ctx, "camp-1", raw); !errors.Is(err, ErrKeyEnded) {
				t.Errorf("err = %v, want ErrKeyEnded", err)
			}
		})
	}
}

// Ending one person's keys leaves other makers' keys working, and the
// per-campaign form leaves their keys elsewhere working.
func TestEndKeys_Scope(t *testing.T) {
	ctx := context.Background()
	svc, _, own, _ := newTestService()
	own["camp-3"] = "owner-1"
	mine1, _, _ := svc.Connect(ctx, "camp-1", "owner-1", aiexport.PrivacyModeSafe, false)
	mine3, _, _ := svc.Connect(ctx, "camp-3", "owner-1", aiexport.PrivacyModeSafe, false)
	theirs, _, _ := svc.Connect(ctx, "camp-2", "owner-2", aiexport.PrivacyModeSafe, false)

	if err := svc.EndKeysMadeBy(ctx, "camp-1", "owner-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-1", mine1); !errors.Is(err, ErrKeyEnded) {
		t.Errorf("camp-1 key: err = %v, want ErrKeyEnded", err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-3", mine3); err != nil {
		t.Errorf("the same person's key in another campaign ended: %v", err)
	}
	if err := svc.EndKeysMadeBy(ctx, "camp-2", "owner-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-2", theirs); err != nil {
		t.Errorf("another maker's key ended: %v", err)
	}

	if err := svc.EndAllKeysMadeBy(ctx, "owner-1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-3", mine3); !errors.Is(err, ErrKeyEnded) {
		t.Errorf("camp-3 key: err = %v, want ErrKeyEnded", err)
	}
	if _, _, err := svc.Authenticate(ctx, "camp-2", theirs); err != nil {
		t.Errorf("another maker's key ended: %v", err)
	}
}
