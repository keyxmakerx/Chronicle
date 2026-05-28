// event_tier_definitions_test.go covers V2 Wave 0 PR 2's tier-definitions
// service surface — validation rules + empty-means-default fallback +
// round-trip semantics. Mirror of how AccentColor / FontFamily are
// tested (campaign-config field; settings JSON nest; Owner-gate
// enforced at route + handler).

package campaigns

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// tierTestRepo wraps mockCampaignRepo with a tiny shim returning a
// fixed campaign + capturing UpdateSettings calls for assertion. Built
// per-test so state doesn't leak between cases.
func tierTestRepo(campaignSettingsJSON string, capture *string) *mockCampaignRepo {
	return &mockCampaignRepo{
		findByIDFn: func(ctx context.Context, id string) (*Campaign, error) {
			return &Campaign{ID: id, Settings: campaignSettingsJSON}, nil
		},
		updateSettingsFn: func(ctx context.Context, campaignID, settingsJSON string) error {
			if capture != nil {
				*capture = settingsJSON
			}
			return nil
		},
	}
}

// TestGetEventTierDefinitions_EmptyReturnsPlatformDefaults — when a
// campaign has no override, the service returns the platform default
// trio (major / standard / detail). Empty-means-default semantic per
// Option B locked 2026-05-28.
func TestGetEventTierDefinitions_EmptyReturnsPlatformDefaults(t *testing.T) {
	repo := tierTestRepo("{}", nil)
	svc := &campaignService{repo: repo}

	defs, err := svc.GetEventTierDefinitions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("GetEventTierDefinitions: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3 default tiers, got %d: %+v", len(defs), defs)
	}
	wantSlugs := []string{"major", "standard", "detail"}
	for i, want := range wantSlugs {
		if defs[i].Slug != want {
			t.Errorf("tier[%d].Slug = %q; want %q", i, defs[i].Slug, want)
		}
	}
	defaults := 0
	var defaultSlug string
	for _, d := range defs {
		if d.IsDefault {
			defaults++
			defaultSlug = d.Slug
		}
	}
	if defaults != 1 || defaultSlug != "standard" {
		t.Errorf("default tier = %q (count=%d); want exactly one default = standard",
			defaultSlug, defaults)
	}
}

// TestGetEventTierDefinitions_DefensiveCopy — returned default slice
// must NOT share backing storage with platformDefaultTiers; mutating
// the caller's copy must not poison the platform default.
func TestGetEventTierDefinitions_DefensiveCopy(t *testing.T) {
	repo := tierTestRepo("{}", nil)
	svc := &campaignService{repo: repo}

	defs, err := svc.GetEventTierDefinitions(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("GetEventTierDefinitions: %v", err)
	}
	defs[0].Name = "MUTATED"

	if platformDefaultTiers[0].Name == "MUTATED" {
		t.Fatal("returned slice shares backing storage with platformDefaultTiers; " +
			"GetEventTierDefinitions must return a defensive copy")
	}
}

// TestGetEventTierDefinitions_StoredOverride — when a campaign has a
// custom tier set, return it verbatim (no platform-default fallback).
func TestGetEventTierDefinitions_StoredOverride(t *testing.T) {
	custom := []TierDefinition{
		{Slug: "epic", Name: "Epic", Color: "#dc2626", Prominence: 100, IsDefault: false},
		{Slug: "routine", Name: "Routine", Color: "#10b981", Prominence: 30, IsDefault: true},
	}
	settingsJSON, _ := json.Marshal(CampaignSettings{EventTierDefinitions: custom})

	repo := tierTestRepo(string(settingsJSON), nil)
	svc := &campaignService{repo: repo}

	defs, err := svc.GetEventTierDefinitions(context.Background(), "camp-2")
	if err != nil {
		t.Fatalf("GetEventTierDefinitions: %v", err)
	}
	if len(defs) != 2 || defs[0].Slug != "epic" || defs[1].Slug != "routine" {
		t.Errorf("returned defs = %+v; want stored override [epic, routine]", defs)
	}
}

// TestSetEventTierDefinitions_Validation runs each validation rule by
// constructing a violating set + asserting a specific error substring
// + asserting the repo's UpdateSettings was NEVER called (validation
// fails before the write).
func TestSetEventTierDefinitions_Validation(t *testing.T) {
	validBase := func() []TierDefinition {
		return []TierDefinition{
			{Slug: "major", Name: "Major", Color: "#ef4444", Prominence: 100, IsDefault: false},
			{Slug: "standard", Name: "Standard", Color: "#6366f1", Prominence: 50, IsDefault: true},
		}
	}

	cases := []struct {
		name       string
		defs       []TierDefinition
		wantErrSub string
	}{
		{
			name:       "empty array rejected",
			defs:       []TierDefinition{},
			wantErrSub: "at least one tier definition is required",
		},
		{
			name: "invalid slug rejected (uppercase)",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Slug = "Major"
				return d
			}(),
			wantErrSub: "lowercase alphanumeric",
		},
		{
			name: "invalid slug rejected (space)",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Slug = "ma jor"
				return d
			}(),
			wantErrSub: "lowercase alphanumeric",
		},
		{
			name: "duplicate slug rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[1].Slug = "major"
				return d
			}(),
			wantErrSub: `slug "major" appears more than once`,
		},
		{
			name: "empty name rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Name = ""
				return d
			}(),
			wantErrSub: "name is required",
		},
		{
			name: "invalid hex color rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Color = "red"
				return d
			}(),
			wantErrSub: "#RRGGBB hex format",
		},
		{
			name: "prominence too low rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Prominence = -1
				return d
			}(),
			wantErrSub: "must be 0-100",
		},
		{
			name: "prominence too high rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].Prominence = 101
				return d
			}(),
			wantErrSub: "must be 0-100",
		},
		{
			name: "no default rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[1].IsDefault = false
				return d
			}(),
			wantErrSub: "exactly one tier must be marked is_default; got 0",
		},
		{
			name: "two defaults rejected",
			defs: func() []TierDefinition {
				d := validBase()
				d[0].IsDefault = true
				return d
			}(),
			wantErrSub: "exactly one tier must be marked is_default; got 2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var captured string
			repo := tierTestRepo("{}", &captured)
			svc := &campaignService{repo: repo}

			err := svc.SetEventTierDefinitions(context.Background(), "camp-1", tc.defs)
			if err == nil {
				t.Fatalf("expected error %q; got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error = %q; want substring %q", err.Error(), tc.wantErrSub)
			}
			if captured != "" {
				t.Errorf("repo.UpdateSettings was called with %q; expected validation to short-circuit before write",
					captured)
			}
		})
	}
}

// TestSetEventTierDefinitions_RoundTrip — happy path: valid set
// validates + persists via repo.UpdateSettings with the marshaled
// settings containing the EventTierDefinitions array. Verifies that
// pre-existing settings (e.g., AccentColor) are NOT clobbered by the
// tier-defs write — important because tier defs nest into the same
// settings JSON (Option B).
func TestSetEventTierDefinitions_RoundTrip(t *testing.T) {
	var captured string
	repo := tierTestRepo(`{"accent_color":"#22c55e"}`, &captured)
	svc := &campaignService{repo: repo}

	custom := []TierDefinition{
		{Slug: "epic", Name: "Epic", Color: "#dc2626", Prominence: 100, IsDefault: false},
		{Slug: "routine", Name: "Routine", Color: "#10b981", Prominence: 30, IsDefault: true},
	}
	if err := svc.SetEventTierDefinitions(context.Background(), "camp-1", custom); err != nil {
		t.Fatalf("SetEventTierDefinitions: %v", err)
	}

	if captured == "" {
		t.Fatal("expected repo.UpdateSettings to be called")
	}
	if !strings.Contains(captured, `"accent_color":"#22c55e"`) {
		t.Errorf("marshaled settings dropped pre-existing accent_color: %s", captured)
	}
	if !strings.Contains(captured, `"slug":"epic"`) {
		t.Errorf("marshaled settings missing event_tier_definitions entry: %s", captured)
	}
}
