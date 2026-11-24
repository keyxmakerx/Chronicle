package cutscenes

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

// "Everyone here" ticks the players who have Chronicle or Foundry open and
// never a Director: the asker's own screens and a Director's Foundry screen
// run the table. The empty line shows when the asker is alone.
func TestShowSheetEveryoneHere(t *testing.T) {
	here := Place{Chronicle: true}
	away := Place{}
	rows := func(rs ...AudienceRow) []AudienceRow { return rs }
	ana := AudienceRow{Member: Member{UserID: "ana", Name: "Ana"}, Place: here}
	bo := AudienceRow{Member: Member{UserID: "bo", Name: "Bo"}, Place: away}
	codm := AudienceRow{Member: Member{UserID: "codm", Name: "Cy", Director: true}, Place: Place{Foundry: true}}
	me := AudienceRow{Member: Member{UserID: "dir", Name: "Dee", Director: true}, Place: Place{Chronicle: true, Foundry: true}, You: true}

	tests := []struct {
		name      string
		rows      []AudienceRow
		wantHere  map[string]string
		wantTool  bool
		wantEmpty bool
	}{
		{name: "players here are ticked, Directors are not", rows: rows(ana, bo, codm, me),
			wantHere: map[string]string{"ana": "y", "bo": "n", "codm": "n", "dir": "n"}, wantTool: true},
		{name: "only Directors here: no Everyone here", rows: rows(bo, codm, me),
			wantHere: map[string]string{"bo": "n", "codm": "n", "dir": "n"}},
		{name: "alone: the empty line, and the asker can still tick themselves", rows: rows(me),
			wantHere: map[string]string{"dir": "n"}, wantEmpty: true},
	}
	box := regexp.MustCompile(`value="([a-z]+)" data-here="([yn])"`)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := ShowSheet(showSheet{CampaignID: "camp-1", Scene: Scene{ID: "s1", Name: "One"}, Rows: tt.rows}).Render(context.Background(), &b); err != nil {
				t.Fatal(err)
			}
			html := b.String()
			got := map[string]string{}
			for _, m := range box.FindAllStringSubmatch(html, -1) {
				got[m[1]] = m[2]
			}
			for id, want := range tt.wantHere {
				if got[id] != want {
					t.Errorf("%s data-here = %q, want %q", id, got[id], want)
				}
			}
			if has := strings.Contains(html, "Everyone here"); has != tt.wantTool {
				t.Errorf("Everyone here shown = %v, want %v", has, tt.wantTool)
			}
			if has := strings.Contains(html, "Nobody else is in this campaign yet."); has != tt.wantEmpty {
				t.Errorf("empty line shown = %v, want %v", has, tt.wantEmpty)
			}
		})
	}
}

// Stopping from "Shown lately" keeps the row: the name and the ticks stay,
// and it reads stopped with no Stop left.
func TestRecentRowStopped(t *testing.T) {
	now := time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)
	r := ShowingReport{ShowingID: "sh1", Name: "The Eye Opens", ShownAt: now.Add(-time.Hour), RewatchUntil: now.Add(time.Hour),
		Viewers: []ReportViewer{{UserID: "ana", Name: "Ana", Watched: true}, {UserID: "bo", Name: "Bo"}}}
	render := func(r ShowingReport) string {
		var b bytes.Buffer
		if err := RecentRow("camp-1", r, now).Render(context.Background(), &b); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	r.CanStop = true
	if h := render(r); !strings.Contains(h, ">Stop</button>") || !strings.Contains(h, "/stop?from=recent") || !strings.Contains(h, `id="`+recentRowID("sh1")+`"`) {
		t.Errorf("open row: %s", h)
	}
	r.CanStop, r.Stopped = false, true
	h := render(r)
	for _, want := range []string{"The Eye Opens", "Ana", "(watched)", "Bo", "stopped", `id="` + recentRowID("sh1") + `"`} {
		if !strings.Contains(h, want) {
			t.Errorf("stopped row lacks %q: %s", want, h)
		}
	}
	if strings.Contains(h, ">Stop</button>") {
		t.Errorf("a stopped row still offers Stop: %s", h)
	}
}
