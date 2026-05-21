// foundry_pin_mode_test.go — round-trip tests for the new
// SetFoundryModulePinMode / GetFoundryModulePinMode methods on the
// campaigns service.
//
// Added in C-FMC-ADMIN-UX-AUDIT Chunk 1. These methods underpin
// Chunks 2 (hook) + 3 (owner UI) + 6 (migration) by providing the
// storage layer for the new pin_mode key in CampaignSettings.
//
// Three properties:
//   1. Set writes pin_mode into the campaign's settings JSON.
//   2. Get reads pin_mode back from the settings JSON.
//   3. Empty pin_mode is preserved (the "not yet set" pre-backfill
//      state Chunk 6's migration relies on detecting).

package campaigns

import (
	"context"
	"encoding/json"
	"testing"
)

// TestSetFoundryModulePinMode_WritesSettingsJSON pins that the setter
// serializes CampaignSettings with the new key and calls UpdateSettings
// with the resulting JSON. The mock's updateSettingsFn captures the
// JSON so we can assert the wire shape.
func TestSetFoundryModulePinMode_WritesSettingsJSON(t *testing.T) {
	var capturedJSON string
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return &Campaign{ID: "camp-1", Settings: `{"foundry_module_pin":"v0.1.14"}`}, nil
		},
		updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
			capturedJSON = settingsJSON
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	if err := svc.SetFoundryModulePinMode(context.Background(), "camp-1", "promote"); err != nil {
		t.Fatalf("SetFoundryModulePinMode: %v", err)
	}

	// Parse the captured JSON and assert pin_mode = "promote" landed,
	// AND the existing foundry_module_pin field is preserved (the
	// setter loads + re-marshals the full settings struct).
	var got CampaignSettings
	if err := json.Unmarshal([]byte(capturedJSON), &got); err != nil {
		t.Fatalf("unmarshal captured settings: %v (raw=%q)", err, capturedJSON)
	}
	if got.FoundryModulePinMode != "promote" {
		t.Errorf("FoundryModulePinMode = %q, want promote", got.FoundryModulePinMode)
	}
	if got.FoundryModulePin != "v0.1.14" {
		t.Errorf("FoundryModulePin lost on round-trip: got %q, want v0.1.14", got.FoundryModulePin)
	}
}

// TestGetFoundryModulePinMode_ReadsSettingsJSON pins the reader path:
// when the campaign's settings JSON contains a pin_mode key, the
// getter returns it; empty/missing returns empty string.
func TestGetFoundryModulePinMode_ReadsSettingsJSON(t *testing.T) {
	cases := []struct {
		name         string
		settingsJSON string
		wantMode     string
	}{
		{"explicit promote", `{"foundry_module_pin_mode":"promote"}`, "promote"},
		{"explicit preserve", `{"foundry_module_pin_mode":"preserve"}`, "preserve"},
		{"explicit pinned", `{"foundry_module_pin_mode":"pinned"}`, "pinned"},
		{"missing key (pre-Chunk-6 backfill)", `{}`, ""},
		{"empty string value (post-migration clear)", `{"foundry_module_pin_mode":""}`, ""},
		{"unknown value passes through verbatim (defense to caller)", `{"foundry_module_pin_mode":"weird"}`, "weird"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockCampaignRepo{
				findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
					return &Campaign{ID: "camp-1", Settings: tc.settingsJSON}, nil
				},
			}
			svc := newTestCampaignService(repo, &mockUserFinder{})

			got, err := svc.GetFoundryModulePinMode(context.Background(), "camp-1")
			if err != nil {
				t.Fatalf("GetFoundryModulePinMode: %v", err)
			}
			if got != tc.wantMode {
				t.Errorf("mode = %q, want %q", got, tc.wantMode)
			}
		})
	}
}

// TestSetFoundryModulePinMode_NotFoundOnMissingCampaign pins the
// not-found path. Mirrors SetFoundryModulePin's behavior.
func TestSetFoundryModulePinMode_NotFoundOnMissingCampaign(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(_ context.Context, _ string) (*Campaign, error) {
			return nil, nil // not found
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	err := svc.SetFoundryModulePinMode(context.Background(), "camp-missing", "promote")
	if err == nil {
		t.Fatal("expected not-found error for missing campaign, got nil")
	}
}
