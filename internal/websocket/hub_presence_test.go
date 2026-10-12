package websocket

import (
	"reflect"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/plugins/foundry_vtt"
)

// BrowserUserIDs feeds the DM Screen's "who's here": only signed-in browser
// sockets count, once per user, and only for the asked-for campaign.
func TestHub_BrowserUserIDs(t *testing.T) {
	h := NewHub()
	go h.Run()

	registerTestClientAs(t, h, "browser", "c1", "u-b", 1, false)
	registerTestClientAs(t, h, "browser", "c1", "u-a", 1, false)
	registerTestClientAs(t, h, foundry_vtt.ModuleSource, "c1", "u-gm", 3, false)
	registerTestClientAs(t, h, NotesAppSource, "c1", "u-notes", 1, false)
	registerTestClientAs(t, h, "browser", "c2", "u-other", 1, false)

	tests := []struct {
		name     string
		campaign string
		want     []string
	}{
		{"browser sockets only, sorted", "c1", []string{"u-a", "u-b"}},
		{"other campaign", "c2", []string{"u-other"}},
		{"unknown campaign", "nope", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.BrowserUserIDs(tc.campaign); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Two tabs of one user count once. The helper keys clients by campaign
	// and user, so add the second tab with a distinct id directly.
	c := &Client{ID: "second-tab", CampaignID: "c1", UserID: "u-a", Source: "browser", hub: h, send: make(chan []byte, 1), done: make(chan struct{})}
	h.register <- c
	settleBroadcast()
	if got := h.BrowserUserIDs("c1"); !reflect.DeepEqual(got, []string{"u-a", "u-b"}) {
		t.Errorf("duplicate tab: got %v", got)
	}
}
