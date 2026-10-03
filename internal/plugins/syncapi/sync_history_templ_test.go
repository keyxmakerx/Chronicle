package syncapi

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestSyncHistoryRows_Render(t *testing.T) {
	at := time.Date(2026, 10, 3, 9, 41, 7, 120e6, time.UTC)
	cases := []struct {
		name     string
		d        SyncHistoryPageData
		want     []string
		wantNone []string
	}{
		{"empty", SyncHistoryPageData{CampaignID: "c1"}, []string{"Nothing has synced yet"}, []string{"Show older"}},
		{"empty with filters", SyncHistoryPageData{CampaignID: "c1", Filter: SyncHistoryFilter{FailedOnly: true}}, []string{"Nothing matches"}, nil},
		{"rows, steps and paging", SyncHistoryPageData{CampaignID: "c1", NextBefore: 41, Filter: SyncHistoryFilter{Direction: DirToFoundry}, Events: []SyncEvent{
			{ID: 42, OccurredAt: at, Direction: DirLink, ReportedBy: reportedByClient, Action: "catch-up", Status: "ok", OK: true, Children: []SyncEvent{
				{ID: 43, OccurredAt: at, Direction: DirToFoundry, ResourceName: "<b>Harbour</b>", Action: "page updated", UserName: "Ren", Status: "failed", Message: "folder missing", DurationMs: 1500},
			}},
		}}, []string{
			`data-id="42"`, "09:41:07.120", "· link", "→ Foundry", "&lt;b&gt;Harbour&lt;/b&gt;", "page updated · Ren",
			"folder missing", "1.50 s", "sh-kid", "sh-fail", "Show older", "before=41", "direction=to_foundry", "Foundry",
		}, []string{"<b>Harbour</b>"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := SyncHistoryRows(tc.d).Render(context.Background(), &buf); err != nil {
				t.Fatal(err)
			}
			html := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(html, w) {
					t.Errorf("missing %q", w)
				}
			}
			for _, w := range tc.wantNone {
				if strings.Contains(html, w) {
					t.Errorf("unexpected %q", w)
				}
			}
		})
	}
}

func TestHistoryLastSeen(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	cases := []struct {
		seen *time.Time
		want string
	}{
		{nil, "Foundry never connected"},
		{ago(30 * time.Second), "Foundry connected"},
		{ago(12 * time.Minute), "Foundry last seen 12 min ago"},
		{ago(5 * time.Hour), "Foundry last seen 5 h ago"},
		{ago(72 * time.Hour), "Foundry last seen 30 Sep"},
	}
	for _, tc := range cases {
		if got := historyLastSeen(SyncHistoryPageData{LastSeen: tc.seen, Now: now}); got != tc.want {
			t.Errorf("got %q, want %q", got, tc.want)
		}
	}
}
