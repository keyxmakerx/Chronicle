// welcome_message_test.go covers UpdateWelcomeMessage: the 500-character
// limit the handler enforces, and that a rejected message never reaches the
// repository.

package campaigns

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestUpdateWelcomeMessage(t *testing.T) {
	cases := []struct {
		name       string
		message    string
		wantErr    bool
		wantStored string
	}{
		{
			name:       "a plain message is stored as-is",
			message:    "Welcome back, adventurers!",
			wantStored: "Welcome back, adventurers!",
		},
		{
			name:       "empty message clears it",
			message:    "",
			wantStored: "",
		},
		{
			name:       "exactly the 500-char limit is accepted",
			message:    strings.Repeat("a", 500),
			wantStored: strings.Repeat("a", 500),
		},
		{
			name:    "over the 500-char limit is rejected",
			message: strings.Repeat("a", 501),
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var stored string
			called := false
			repo := &mockCampaignRepo{
				findByIDFn: func(ctx context.Context, id string) (*Campaign, error) {
					return &Campaign{ID: id, Settings: "{}"}, nil
				},
				updateSettingsFn: func(ctx context.Context, campaignID, settingsJSON string) error {
					called = true
					stored = settingsJSON
					return nil
				},
			}
			svc := &campaignService{repo: repo}

			err := svc.UpdateWelcomeMessage(context.Background(), "camp-1", tc.message)

			if tc.wantErr {
				assertAppError(t, err, http.StatusBadRequest)
				if called {
					t.Error("a rejected message must not reach the repository")
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateWelcomeMessage: unexpected error: %v", err)
			}
			if !called {
				t.Fatal("expected UpdateSettings to be called")
			}
			c := Campaign{Settings: stored}
			settings := c.ParseSettings()
			if settings.WelcomeMessage != tc.wantStored {
				t.Errorf("stored welcome_message = %q, want %q", settings.WelcomeMessage, tc.wantStored)
			}
		})
	}
}
