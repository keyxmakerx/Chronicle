package addons

import (
	"context"
	"testing"
)

// stubSetupProvider is a minimal SetupProvider for registry/state tests.
type stubSetupProvider struct {
	slug   string
	checks []SetupCheck
}

func (s *stubSetupProvider) Slug() string { return s.slug }
func (s *stubSetupProvider) RunChecks(_ context.Context, _ string) ([]SetupCheck, error) {
	return s.checks, nil
}
func (s *stubSetupProvider) Questions(_ context.Context, _ string) ([]SetupQuestion, error) {
	return nil, nil
}
func (s *stubSetupProvider) Apply(_ context.Context, _ string, _ map[string]string) (SetupResult, error) {
	return SetupResult{Completed: true}, nil
}

func TestNeedsSetup(t *testing.T) {
	const slug = "player-character-claiming"
	warn := []SetupCheck{{ID: "x", Severity: SeverityWarning, Title: "needs attention"}}
	infoOnly := []SetupCheck{{ID: "x", Severity: SeverityInfo, Title: "fyi"}}

	cases := []struct {
		name      string
		register  bool
		checks    []SetupCheck
		enabled   bool
		config    map[string]any
		wantNeeds bool
	}{
		{"no provider registered", false, warn, true, nil, false},
		{"provider but addon disabled", true, warn, false, nil, false},
		{"enabled, only info checks", true, infoOnly, true, nil, false},
		{"enabled, actionable warning", true, warn, true, nil, true},
		{"enabled + warning but completed", true, warn, true, map[string]any{"setup": map[string]any{"completed": true}}, false},
		{"enabled + warning but dismissed", true, warn, true, map[string]any{"setup": map[string]any{"dismissed": true}}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockAddonRepo{
				listForCampaignFn: func(_ context.Context, _ string) ([]CampaignAddon, error) {
					return []CampaignAddon{{AddonID: 7, AddonSlug: slug, Enabled: tc.enabled, ConfigJSON: tc.config}}, nil
				},
			}
			svc := NewAddonService(repo)
			if tc.register {
				svc.RegisterSetupProvider(&stubSetupProvider{slug: slug, checks: tc.checks})
			}
			got, err := svc.NeedsSetup(context.Background(), "camp-1", slug)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantNeeds {
				t.Errorf("NeedsSetup = %v, want %v", got, tc.wantNeeds)
			}
		})
	}
}

func TestSetupStateRoundTrip(t *testing.T) {
	const slug = "player-character-claiming"

	t.Run("SaveSetupState merges the setup key, preserving other config", func(t *testing.T) {
		var captured map[string]any
		repo := &mockAddonRepo{
			listForCampaignFn: func(_ context.Context, _ string) ([]CampaignAddon, error) {
				return []CampaignAddon{{AddonID: 7, AddonSlug: slug, Enabled: true, ConfigJSON: map[string]any{"other": "keep"}}}, nil
			},
			updateCampaignCfgFn: func(_ context.Context, _ string, _ int, config map[string]any) error {
				captured = config
				return nil
			},
		}
		svc := NewAddonService(repo)

		err := svc.SaveSetupState(context.Background(), "camp-1", slug,
			SetupState{Completed: true, Answers: map[string]string{"a": "b"}})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured["other"] != "keep" {
			t.Errorf("other config key not preserved: %+v", captured)
		}
		st, ok := captured["setup"].(SetupState)
		if !ok {
			t.Fatalf("setup key is %T, want SetupState", captured["setup"])
		}
		if !st.Completed || st.Answers["a"] != "b" {
			t.Errorf("persisted setup state = %+v, want Completed + answers[a]=b", st)
		}
	})

	t.Run("GetSetupState parses config_json round-trip (map form)", func(t *testing.T) {
		repo := &mockAddonRepo{
			listForCampaignFn: func(_ context.Context, _ string) ([]CampaignAddon, error) {
				return []CampaignAddon{{AddonID: 7, AddonSlug: slug, Enabled: true, ConfigJSON: map[string]any{
					"setup": map[string]any{"completed": true, "answers": map[string]any{"k": "v"}},
				}}}, nil
			},
		}
		svc := NewAddonService(repo)

		st, err := svc.GetSetupState(context.Background(), "camp-1", slug)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !st.Completed || st.Answers["k"] != "v" {
			t.Errorf("GetSetupState = %+v, want Completed + answers[k]=v", st)
		}
	})
}
