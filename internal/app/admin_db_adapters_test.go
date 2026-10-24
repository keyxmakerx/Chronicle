package app

import (
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/backup"
)

// The newer of the button run and the stored scheduled run decides, so a
// later success clears an earlier failure from either source.
func TestLastRunFailed(t *testing.T) {
	t0 := time.Date(2026, 5, 10, 3, 0, 0, 0, time.UTC)
	ok := &backup.RunResult{FinishedAt: t0}
	bad := &backup.RunResult{FinishedAt: t0, ExitCode: 3}
	tests := []struct {
		name      string
		manual    *backup.RunResult
		scheduled *backup.ScheduledRun
		want      bool
	}{
		{"nothing has run", nil, nil, false},
		{"button run failed", bad, nil, true},
		{"button run succeeded", ok, nil, false},
		{"scheduled run failed", nil, &backup.ScheduledRun{At: t0, OK: false}, true},
		{"later button success clears a scheduled failure", ok, &backup.ScheduledRun{At: t0.Add(-time.Hour), OK: false}, false},
		{"later scheduled failure beats a button success", ok, &backup.ScheduledRun{At: t0.Add(time.Hour), OK: false}, true},
		{"later scheduled success clears a button failure", bad, &backup.ScheduledRun{At: t0.Add(time.Hour), OK: true}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastRunFailed(tc.manual, tc.scheduled); got != tc.want {
				t.Errorf("lastRunFailed = %v, want %v", got, tc.want)
			}
		})
	}
}
