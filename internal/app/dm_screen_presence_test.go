package app

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/syncapi"
)

func TestHereUsers(t *testing.T) {
	now := time.Date(2026, 10, 10, 20, 0, 0, 0, time.UTC)
	fresh := now.Add(-time.Minute)
	stale := now.Add(-foundryReportMaxAge - time.Second)
	tests := []struct {
		name      string
		browser   []string
		reports   []syncapi.FoundryPlayer
		foundryUp bool
		want      []string
	}{
		{"browser only", []string{"a"}, nil, false, []string{"a"}},
		{"fresh online foundry row", nil, []syncapi.FoundryPlayer{{MemberUserID: "b", Online: true, ReportedAt: fresh}}, true, []string{"b"}},
		{"gm foundry not connected ignores the report", nil, []syncapi.FoundryPlayer{{MemberUserID: "b", Online: true, ReportedAt: fresh}}, false, nil},
		{"stale report ignored", nil, []syncapi.FoundryPlayer{{MemberUserID: "b", Online: true, ReportedAt: stale}}, true, nil},
		{"offline row ignored", nil, []syncapi.FoundryPlayer{{MemberUserID: "b", Online: false, ReportedAt: fresh}}, true, nil},
		{"unlinked row ignored", nil, []syncapi.FoundryPlayer{{Online: true, ReportedAt: fresh}}, true, nil},
		{"both signals merge", []string{"a"}, []syncapi.FoundryPlayer{{MemberUserID: "b", Online: true, ReportedAt: fresh}, {MemberUserID: "a", Online: true, ReportedAt: fresh}}, true, []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hereUsers(tt.browser, tt.reports, tt.foundryUp, now)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for _, id := range tt.want {
				if !got[id] {
					t.Errorf("%s missing from %v", id, got)
				}
			}
		})
	}
}
