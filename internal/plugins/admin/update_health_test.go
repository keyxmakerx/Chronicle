package admin

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/updatehealth"
)

func TestUpdateHealthNeeds(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	seen := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	last := now.Add(-2 * time.Hour)
	f := func(name string) updatehealth.Feature {
		return updatehealth.Feature{ID: name, Name: name, Try: "/try/" + name}
	}
	statuses := []updatehealth.Status{
		{Feature: f("Shop"), State: updatehealth.Failing, FirstSeen: seen, Errors: 2, LastError: &last, LastErrorMsg: "POST /x: boom"},
		{Feature: f("Restore"), State: updatehealth.Unused, FirstSeen: seen},
		{Feature: f("Maps"), State: updatehealth.Unused, FirstSeen: seen, Uses: 3},
		{Feature: f("Notes"), State: updatehealth.Counting, FirstSeen: seen},
		{Feature: f("Dice"), State: updatehealth.Passed, FirstSeen: seen, Uses: 12},
	}
	got := updateHealthNeeds(statuses, now)
	want := []struct {
		text, href, button, detail string
		count                      int
		urgent                     bool
	}{
		{"Shop has failed since the update on Tue 3 Nov", "/admin/diagnostics/workspace", "See the errors", "Last: POST /x: boom, 2 h ago", 2, true},
		{"Restore changed 17 days ago and nobody has used it yet", "/try/Restore", "Try it now", "", 1, false},
		{"Maps changed 17 days ago and has been used only 3 times", "/try/Maps", "Try it now", "", 1, false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Text != w.text || g.Href != w.href || g.Button != w.button || g.Detail != w.detail || g.Count != w.count || g.Urgent() != w.urgent {
			t.Errorf("row %d = %+v, want %+v", i, g, w)
		}
	}

	// A failing feature goes above setup chores, an unused one below faults.
	merged := mergeNeeds(buildNeedsYou(needsInput{SMTPKnown: true, PendingMigrations: 1}), got)
	order := []string{merged[0].Text, merged[1].Text, merged[2].Text, merged[len(merged)-1].Text}
	if order[0] != want[0].text || merged[1].Button != "Open database" || order[2] != want[1].text || merged[len(merged)-1].Button != "Set up email" {
		t.Errorf("merged order wrong: %v", order)
	}
}

func TestPassedLine(t *testing.T) {
	tests := []struct {
		passed, counting int
		want             string
	}{
		{0, 0, ""},
		{11, 0, "11 changed features passed by use."},
		{1, 0, "1 changed feature passed by use."},
		{0, 2, "2 changed features still being counted after the update."},
		{3, 2, "3 changed features passed by use; 2 still being counted."},
	}
	for _, tt := range tests {
		if got := passedLine(tt.passed, tt.counting); got != tt.want {
			t.Errorf("passedLine(%d, %d) = %q, want %q", tt.passed, tt.counting, got, tt.want)
		}
	}
}
