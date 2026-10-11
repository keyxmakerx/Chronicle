package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The strip says only what it could read: an unread figure is left out,
// never shown as a zero, and missing backups are called out.
func TestAdminStatusFigures(t *testing.T) {
	cases := []struct {
		name    string
		f       StatusFigures
		want    []string
		notWant []string
	}{
		{"all known", StatusFigures{Sessions: 5, BackupKnown: true, BackupEnabled: true, BackupAge: "6 hours"},
			[]string{"<b>5</b> sessions signed in", "Last backup <b>6 hours</b> ago"}, nil},
		{"one session", StatusFigures{Sessions: 1}, []string{"<b>1</b> session signed in"}, []string{"backup"}},
		{"sessions unread", StatusFigures{Sessions: -1, BackupKnown: true, BackupEnabled: true, BackupAge: "2 days"},
			[]string{"Last backup"}, []string{"signed in"}},
		{"backups off", StatusFigures{Sessions: 0, BackupKnown: true}, []string{"Backups are off"}, []string{"Last backup"}},
		{"no backup yet", StatusFigures{Sessions: 0, BackupKnown: true, BackupEnabled: true}, []string{"No backup yet"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := AdminStatusFigures(tc.f).Render(context.Background(), &buf); err != nil {
				t.Fatalf("render: %v", err)
			}
			out := buf.String()
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("missing %q in %s", w, out)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(out, w) {
					t.Errorf("unexpected %q in %s", w, out)
				}
			}
		})
	}
}

// Faults (failed parts, database behind, security alerts) are urgent;
// chores (reviews, email setup) are not.
func TestNeedsItemUrgent(t *testing.T) {
	items := buildNeedsYou(needsInput{
		UnhealthyPlugins: 1, PendingMigrations: 1, APIAlerts: 1, PendingSubmissions: 1,
		SMTPKnown: true,
	})
	want := []bool{true, true, true, false, false}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d", len(items), len(want))
	}
	for i, it := range items {
		if it.Urgent() != want[i] {
			t.Errorf("%q urgent = %v, want %v", it.Text, it.Urgent(), want[i])
		}
	}
}
