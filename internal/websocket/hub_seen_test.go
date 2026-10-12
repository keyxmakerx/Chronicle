package websocket

import (
	"reflect"
	"testing"
	"time"
)

// A page request counts as "has Chronicle open" for a short window, per
// campaign, and old records are swept so the map cannot grow without bound.
func TestHub_BrowserSeen(t *testing.T) {
	h := NewHub()
	clock := time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return clock }
	window := 5 * time.Minute

	h.MarkBrowserSeen("c1", "u-a")
	h.MarkBrowserSeen("c2", "u-b")
	h.MarkBrowserSeen("", "ignored")
	h.MarkBrowserSeen("c1", "")

	clock = clock.Add(4 * time.Minute)
	h.MarkBrowserSeen("c1", "u-c")

	tests := []struct {
		name     string
		campaign string
		want     []string
	}{
		{"inside the window", "c1", []string{"u-a", "u-c"}},
		{"scoped to the campaign", "c2", []string{"u-b"}},
		{"unknown campaign", "nope", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.RecentBrowserUsers(tc.campaign, window); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	clock = clock.Add(2 * time.Minute) // u-a is now 6 minutes old
	if got := h.RecentBrowserUsers("c1", window); !reflect.DeepEqual(got, []string{"u-c"}) {
		t.Errorf("after the window: got %v", got)
	}

	// A mark long after sweeps everything older than the retention.
	clock = clock.Add(browserSeenRetention + time.Minute)
	h.MarkBrowserSeen("c3", "u-d")
	h.browserMu.Lock()
	left := len(h.browserSeen)
	h.browserMu.Unlock()
	if left != 1 {
		t.Errorf("campaigns kept after sweep = %d, want 1", left)
	}
}
