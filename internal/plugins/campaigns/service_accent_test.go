package campaigns

import (
	"context"
	"encoding/json"
	"testing"
)

// TestUpdateAccentSurface covers the surface-pair save path (C-ACCENT-TRIO
// rev 2): load-merge-write on settings JSON, slot routing, reset-to-inherit,
// preservation of unrelated settings, and slot validation.
func TestUpdateAccentSurface(t *testing.T) {
	cases := []struct {
		name      string
		slot      int
		color     string
		wantErr   bool
		wantS1    string
		wantS2    string
	}{
		{"set slot 1", 1, "#10b981", false, "#10b981", ""},
		{"set slot 2", 2, "#f59e0b", false, "", "#f59e0b"},
		{"reset slot 1 to inherit", 1, "", false, "", ""},
		{"invalid slot 0", 0, "#10b981", true, "", ""},
		{"invalid slot 3", 3, "#10b981", true, "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var savedJSON string
			repo := &mockCampaignRepo{
				findByIDFn: func(ctx context.Context, id string) (*Campaign, error) {
					// Existing settings carry a chrome accent + brand name that
					// the surface write must NOT clobber (load-merge-write).
					return &Campaign{
						ID:       id,
						Settings: `{"accent_color":"#6366f1","brand_name":"Therin"}`,
					}, nil
				},
				updateSettingsFn: func(ctx context.Context, campaignID, settingsJSON string) error {
					savedJSON = settingsJSON
					return nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})

			err := svc.UpdateAccentSurface(context.Background(), "c1", tc.slot, tc.color)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for slot %d, got nil", tc.slot)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var saved CampaignSettings
			if err := json.Unmarshal([]byte(savedJSON), &saved); err != nil {
				t.Fatalf("saved settings not valid JSON: %v", err)
			}
			if saved.AccentSurface1 != tc.wantS1 || saved.AccentSurface2 != tc.wantS2 {
				t.Errorf("surface slots = (%q, %q), want (%q, %q)",
					saved.AccentSurface1, saved.AccentSurface2, tc.wantS1, tc.wantS2)
			}
			// Merge guarantee: untouched settings survive the write.
			if saved.AccentColor != "#6366f1" || saved.BrandName != "Therin" {
				t.Errorf("unrelated settings clobbered: accent=%q brand=%q", saved.AccentColor, saved.BrandName)
			}
		})
	}
}
