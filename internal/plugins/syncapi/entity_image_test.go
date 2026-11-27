package syncapi

import (
	"context"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

// imageEntitySvc records the pictures set on the acting test hero.
type imageEntitySvc struct {
	actingEntitySvc
	images []string
}

func (s *imageEntitySvc) UpdateImage(_ context.Context, _ string, path string) error {
	s.images = append(s.images, path)
	return nil
}

// A portrait chosen in Foundry becomes the page's picture: the GM's sync and
// the hero's own player may set it, another player may not.
func TestUpdateEntityImage(t *testing.T) {
	roles := map[string]campaigns.Role{"u-1": campaigns.RoleOwner, "pat": campaigns.RolePlayer, "lee": campaigns.RolePlayer}
	media := "0b0e4c5e-6f1a-4c55-9c39-6a1f0f6e8a10"
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantImage  string
	}{
		{"the GM's sync", `{"image_path":"` + media + `"}`, http.StatusOK, media},
		{"the hero's own player", `{"image_path":"` + media + `","actingUserId":"pat"}`, http.StatusOK, media},
		{"another player", `{"image_path":"` + media + `","actingUserId":"lee"}`, http.StatusForbidden, ""},
		{"someone not in the campaign", `{"image_path":"` + media + `","actingUserId":"ghost"}`, http.StatusForbidden, ""},
		{"a bad body", `{"image_path":`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ents := &imageEntitySvc{}
			h := NewAPIHandler(&stubSyncSvcForRole{}, ents, actingCampaignSvc{roles: roles}, nil)
			c, _ := gateContext(http.MethodPut, tt.body)
			err := h.UpdateEntityImage(c)
			if got := gateStatus(err); got != tt.wantStatus {
				t.Fatalf("status = %d (%v), want %d", got, err, tt.wantStatus)
			}
			if tt.wantImage == "" {
				if len(ents.images) != 0 {
					t.Fatalf("a refused write set %v", ents.images)
				}
				return
			}
			if len(ents.images) != 1 || ents.images[0] != tt.wantImage {
				t.Fatalf("set %v, want %q", ents.images, tt.wantImage)
			}
		})
	}
}
