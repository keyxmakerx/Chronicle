package entities

import (
	"context"
	"net/http"
	"testing"
)

// TestSetPrivateInCampaign: the flag is set, never flipped, so a repeated
// request leaves it where the first one put it; another campaign's entity is
// refused before any write.
func TestSetPrivateInCampaign(t *testing.T) {
	tests := []struct {
		name       string
		campaign   string
		isPrivate  bool
		setTo      bool
		wantStatus int
		wantWrite  bool
	}{
		{"reveals a hidden entity", "camp-owner", true, false, 0, true},
		{"already visible is a no-op", "camp-owner", false, false, 0, false},
		{"hides a visible entity", "camp-owner", false, true, 0, true},
		{"another campaign is refused", "camp-attacker", true, false, http.StatusNotFound, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrote := false
			var written bool
			repo := &mockEntityRepo{
				findByIDFn: func(_ context.Context, id string) (*Entity, error) {
					return &Entity{ID: id, CampaignID: "camp-owner", IsPrivate: tt.isPrivate}, nil
				},
				updatePrivateFn: func(_ context.Context, _ string, p bool) error {
					wrote, written = true, p
					return nil
				},
			}
			svc := newTestService(repo, &mockEntityTypeRepo{})
			err := svc.SetPrivateInCampaign(context.Background(), "ent-1", tt.campaign, tt.setTo)
			if tt.wantStatus != 0 {
				assertAppError(t, err, tt.wantStatus)
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if wrote != tt.wantWrite {
				t.Fatalf("wrote = %v, want %v", wrote, tt.wantWrite)
			}
			if wrote && written != tt.setTo {
				t.Fatalf("wrote is_private=%v, want %v", written, tt.setTo)
			}
		})
	}
}
