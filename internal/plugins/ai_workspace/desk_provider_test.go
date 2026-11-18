// desk_provider_test.go pins how a key's state is worded on the owner's desk.

package ai_workspace

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/ai_workspace/connector"
	"github.com/keyxmakerx/chronicle/internal/plugins/campaigns"
)

func TestDeskAIState(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Hour), now.Add(-time.Hour)
	tests := []struct {
		name string
		conn *connector.Connection
		want campaigns.DeskAIState
	}{
		{"no key", nil, campaigns.DeskAINone},
		{"live", &connector.Connection{ExpiresAt: later}, campaigns.DeskAIConnected},
		{"paused", &connector.Connection{ExpiresAt: later, PausedAt: &earlier}, campaigns.DeskAIPaused},
		{"revoked", &connector.Connection{ExpiresAt: later, RevokedAt: &earlier}, campaigns.DeskAIEnded},
		{"run out", &connector.Connection{ExpiresAt: earlier}, campaigns.DeskAIEnded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deskAIState(tt.conn, now); got != tt.want {
				t.Errorf("state = %q, want %q", got, tt.want)
			}
		})
	}
}
