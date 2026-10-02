package components

import (
	"context"
	"strings"
	"testing"
)

func TestAdminGroupHeading(t *testing.T) {
	tests := []struct {
		name        string
		group       AdminTabGroup
		current     string
		wantCurrent int
		wantNav     bool
	}{
		{"marks only the open tab", AdminTabsStorage, "/admin/storage/settings", 1, true},
		{"unknown page marks none", AdminTabsHealth, "/admin/elsewhere", 0, true},
		{"single page has no strip", AdminTabsAPI, "/admin/api", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sb strings.Builder
			if err := AdminGroupHeading(tc.group, tc.current).Render(context.Background(), &sb); err != nil {
				t.Fatal(err)
			}
			out := sb.String()
			if !strings.Contains(out, "<h1") {
				t.Errorf("missing heading: %s", out)
			}
			if n := strings.Count(out, `aria-current="page"`); n != tc.wantCurrent {
				t.Errorf("aria-current count = %d, want %d", n, tc.wantCurrent)
			}
			if got := strings.Contains(out, "<nav"); got != tc.wantNav {
				t.Errorf("nav present = %v, want %v", got, tc.wantNav)
			}
		})
	}
}
