package systems

import (
	"errors"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// A campaign's own system must only answer for its own id: AI Import passes
// the id from the campaign's settings, and a stale or mistyped one must not
// open whatever custom system happens to be uploaded.
func TestCampaignBookPackage_OwnSystemMustMatch(t *testing.T) {
	mgr := &CampaignSystemManager{modules: map[string]*GenericSystem{
		"camp": {manifest: &SystemManifest{ID: "homebrew"}},
	}}
	h := &SystemHandler{campaignSystems: mgr, bookEdits: struct{ BookEditService }{}}
	tests := []struct {
		name, systemID string
	}{
		{"different id", "other-system"},
		{"empty id", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := h.CampaignBookPackage("camp", tt.systemID)
			var ae *apperror.AppError
			if !errors.As(err, &ae) || ae.Code != 404 {
				t.Fatalf("err = %v, want not found", err)
			}
		})
	}
}
