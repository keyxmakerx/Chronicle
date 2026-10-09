// character_lists_test.go covers UpdateCharacterLists: both lists are stored,
// an empty choice stays a choice (distinct from never having chosen), and the
// campaign's other settings are left as they were.

package campaigns

import (
	"context"
	"reflect"
	"testing"
)

func TestUpdateCharacterLists(t *testing.T) {
	cases := []struct {
		name      string
		existing  string
		chars     []int
		npcs      []int
		wantChars []int
		wantNPCs  []int
	}{
		{"fresh campaign", "{}", []int{1, 2}, []int{3}, []int{1, 2}, []int{3}},
		{"empty lists are stored as empty, not dropped", "{}", nil, nil, []int{}, []int{}},
		{"replaces an earlier choice", `{"character_type_ids":[9],"npc_type_ids":[9]}`, []int{1}, []int{2}, []int{1}, []int{2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stored string
			repo := &mockCampaignRepo{
				findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id, Settings: tc.existing}, nil
				},
				updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
					stored = settingsJSON
					return nil
				},
			}
			svc := &campaignService{repo: repo}
			if err := svc.UpdateCharacterLists(context.Background(), "camp-1", tc.chars, tc.npcs); err != nil {
				t.Fatalf("UpdateCharacterLists: %v", err)
			}
			got := (&Campaign{Settings: stored}).ParseSettings()
			if got.CharacterTypeIDs == nil || got.NPCTypeIDs == nil {
				t.Fatalf("stored settings %s lost a list", stored)
			}
			if !reflect.DeepEqual(*got.CharacterTypeIDs, tc.wantChars) || !reflect.DeepEqual(*got.NPCTypeIDs, tc.wantNPCs) {
				t.Errorf("stored %s, want chars %v npcs %v", stored, tc.wantChars, tc.wantNPCs)
			}
		})
	}

	t.Run("other settings survive", func(t *testing.T) {
		var stored string
		repo := &mockCampaignRepo{
			findByIDFn: func(_ context.Context, id string) (*Campaign, error) {
				return &Campaign{ID: id, Settings: `{"accent_color":"#123456","welcome_message":"hi"}`}, nil
			},
			updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
				stored = settingsJSON
				return nil
			},
		}
		svc := &campaignService{repo: repo}
		if err := svc.UpdateCharacterLists(context.Background(), "camp-1", []int{1}, []int{1}); err != nil {
			t.Fatal(err)
		}
		got := (&Campaign{Settings: stored}).ParseSettings()
		if got.AccentColor != "#123456" || got.WelcomeMessage != "hi" {
			t.Errorf("other settings changed: %s", stored)
		}
	})
}
