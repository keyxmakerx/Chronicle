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

// TestAccentHexValidation pins the #521 hardening: both accent service methods
// reject any non-#RRGGBB value and NEVER persist it (the value is emitted into a
// raw <style> block via templ.Raw at render — the last CSS-injection path).
// Empty is allowed (reset to default / inherit).
func TestAccentHexValidation(t *testing.T) {
	newSvc := func(wrote *bool) CampaignService {
		return newTestCampaignService(&mockCampaignRepo{
			findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
				return &Campaign{ID: id, Settings: `{}`}, nil
			},
			updateSettingsFn: func(context.Context, string, string) error { *wrote = true; return nil },
		}, &mockUserFinder{})
	}

	bad := []string{"red", "6366f1", "#red;}", "#12345", "#1234567", "#GGGGGG", `#111"}</style><script>`}
	for _, color := range bad {
		t.Run("reject/"+color, func(t *testing.T) {
			var wrote bool
			svc := newSvc(&wrote)
			if err := svc.UpdateAccentColor(context.Background(), "c1", color); err == nil {
				t.Errorf("UpdateAccentColor accepted invalid %q", color)
			}
			if err := svc.UpdateAccentSurface(context.Background(), "c1", 1, color); err == nil {
				t.Errorf("UpdateAccentSurface accepted invalid %q", color)
			}
			if wrote {
				t.Errorf("invalid color %q must never reach the repo", color)
			}
		})
	}
	// C-ACCENT-SLOTS: the two new semantic slots share the exact same
	// validation gate as the legacy slots above.
	for _, color := range bad {
		t.Run("reject-action/"+color, func(t *testing.T) {
			var wrote bool
			svc := newSvc(&wrote)
			if err := svc.UpdateAccentAction(context.Background(), "c1", color); err == nil {
				t.Errorf("UpdateAccentAction accepted invalid %q", color)
			}
			if wrote {
				t.Errorf("invalid color %q must never reach the repo (action)", color)
			}
		})
		t.Run("reject-app/"+color, func(t *testing.T) {
			var wrote bool
			svc := newSvc(&wrote)
			if err := svc.UpdateAccentApp(context.Background(), "c1", color); err == nil {
				t.Errorf("UpdateAccentApp accepted invalid %q", color)
			}
			if wrote {
				t.Errorf("invalid color %q must never reach the repo (app)", color)
			}
		})
	}

	for _, color := range []string{"#6366f1", "#ABCDEF", ""} {
		t.Run("accept/"+color, func(t *testing.T) {
			var wrote bool
			svc := newSvc(&wrote)
			if err := svc.UpdateAccentColor(context.Background(), "c1", color); err != nil {
				t.Errorf("UpdateAccentColor rejected valid %q: %v", color, err)
			}
			if err := svc.UpdateAccentSurface(context.Background(), "c1", 2, color); err != nil {
				t.Errorf("UpdateAccentSurface rejected valid %q: %v", color, err)
			}
			if err := svc.UpdateAccentAction(context.Background(), "c1", color); err != nil {
				t.Errorf("UpdateAccentAction rejected valid %q: %v", color, err)
			}
			if err := svc.UpdateAccentApp(context.Background(), "c1", color); err != nil {
				t.Errorf("UpdateAccentApp rejected valid %q: %v", color, err)
			}
		})
	}
}

// TestUpdateAccentAction_And_UpdateAccentApp covers the C-ACCENT-SLOTS save
// paths: load-merge-write on settings JSON, reset-to-inherit, and
// preservation of unrelated settings (including the legacy trio fields).
func TestUpdateAccentAction_And_UpdateAccentApp(t *testing.T) {
	newRepo := func(savedJSON *string) *mockCampaignRepo {
		return &mockCampaignRepo{
			findByIDFn: func(ctx context.Context, id string) (*Campaign, error) {
				// Existing settings carry a site accent + legacy surface slot
				// that these writes must NOT clobber (load-merge-write).
				return &Campaign{
					ID:       id,
					Settings: `{"accent_color":"#6366f1","accent_surface_1":"#10b981","brand_name":"Therin"}`,
				}, nil
			},
			updateSettingsFn: func(ctx context.Context, campaignID, settingsJSON string) error {
				*savedJSON = settingsJSON
				return nil
			},
		}
	}

	t.Run("UpdateAccentAction sets the field and preserves the rest", func(t *testing.T) {
		var savedJSON string
		svc := newTestCampaignService(newRepo(&savedJSON), &mockUserFinder{})
		if err := svc.UpdateAccentAction(context.Background(), "c1", "#f59e0b"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var saved CampaignSettings
		if err := json.Unmarshal([]byte(savedJSON), &saved); err != nil {
			t.Fatalf("saved settings not valid JSON: %v", err)
		}
		if saved.AccentAction != "#f59e0b" {
			t.Errorf("AccentAction = %q, want #f59e0b", saved.AccentAction)
		}
		if saved.AccentColor != "#6366f1" || saved.AccentSurface1 != "#10b981" || saved.BrandName != "Therin" {
			t.Errorf("unrelated settings clobbered: %+v", saved)
		}
	})

	t.Run("UpdateAccentApp sets the field and preserves the rest", func(t *testing.T) {
		var savedJSON string
		svc := newTestCampaignService(newRepo(&savedJSON), &mockUserFinder{})
		if err := svc.UpdateAccentApp(context.Background(), "c1", "#0ea5e9"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var saved CampaignSettings
		if err := json.Unmarshal([]byte(savedJSON), &saved); err != nil {
			t.Fatalf("saved settings not valid JSON: %v", err)
		}
		if saved.AccentApp != "#0ea5e9" {
			t.Errorf("AccentApp = %q, want #0ea5e9", saved.AccentApp)
		}
		if saved.AccentColor != "#6366f1" || saved.AccentSurface1 != "#10b981" || saved.BrandName != "Therin" {
			t.Errorf("unrelated settings clobbered: %+v", saved)
		}
	})

	t.Run("reset to inherit", func(t *testing.T) {
		var savedJSON string
		svc := newTestCampaignService(newRepo(&savedJSON), &mockUserFinder{})
		if err := svc.UpdateAccentAction(context.Background(), "c1", ""); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var saved CampaignSettings
		if err := json.Unmarshal([]byte(savedJSON), &saved); err != nil {
			t.Fatalf("saved settings not valid JSON: %v", err)
		}
		if saved.AccentAction != "" {
			t.Errorf("AccentAction = %q, want empty (inherit)", saved.AccentAction)
		}
	})
}
