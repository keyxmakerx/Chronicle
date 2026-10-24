package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CheckFreshFor is how new a finished backup must be for the site check-up
// to read it back instead of making its own. With daily backups on, last
// night's backup is always this new, so the check-up adds no extra run.
const CheckFreshFor = 24 * time.Hour

// BackupCheck is the outcome of the site check-up's backup check.
type BackupCheck struct {
	At       time.Time `json:"at"`
	BackupAt time.Time `json:"backupAt,omitempty"` // when the checked backup was made
	MadeNow  bool      `json:"madeNow,omitempty"`  // the check made this backup itself
	OK       bool      `json:"ok"`
	Problem  string    `json:"problem,omitempty"` // a plain sentence when !OK
}

// CheckBackup proves the site has a backup a restore could use: it reads the
// newest finished backup back from end to end, first making one when none is
// newer than CheckFreshFor. A backup it makes keeps the daily schedule's
// retention when that is on, and otherwise the same retention as the Run
// backup button, so the check-up never deletes more than the owner chose.
// ErrAlreadyRunning is returned untouched so the caller can try again later.
func CheckBackup(ctx context.Context, svc Service, store SettingsStore, now time.Time) (BackupCheck, error) {
	res := BackupCheck{At: now}
	dir := svc.BackupDir()
	if dir == "" {
		res.Problem = "No backup folder is set, so the site can't make backups."
		return res, nil
	}

	name, made, ok := newestManifest(dir)
	if !ok || now.Sub(made) > CheckFreshFor {
		keep := 0
		if store != nil {
			if sched, err := LoadSchedule(ctx, store); err == nil && sched.Enabled {
				keep = sched.KeepDays
			}
		}
		run, err := svc.RunBackupKeeping(ctx, keep)
		if err != nil {
			return res, err
		}
		if !run.Succeeded() {
			res.Problem = "The backup failed: " + failureDetail(run) + "."
			return res, nil
		}
		res.MadeNow = true
		if name = manifestFromStdout(run.Stdout); name == "" {
			res.Problem = "The backup finished but didn't say which files it wrote."
			return res, nil
		}
		made = run.FinishedAt
	}
	res.BackupAt = made
	if err := VerifyBackup(dir, name); err != nil {
		res.Problem = "The newest backup can't be restored. " + upperFirst(err.Error()) + "."
		return res, nil
	}
	res.OK = true
	return res, nil
}

// upperFirst capitalises a sentence that starts in lower case.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// newestManifest returns the newest finished backup's manifest and when it
// was written. A manifest is written last, so one existing means the run
// finished.
func newestManifest(dir string) (string, time.Time, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", time.Time{}, false
	}
	var best string
	var bestAt time.Time
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, verifyManifestPrefix) || !strings.HasSuffix(n, ".txt") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestAt) {
			best, bestAt = n, info.ModTime()
		}
	}
	return best, bestAt, best != ""
}

// manifestFromStdout reads the "artifact=manifest file=<path>" line
// scripts/backup.sh prints and returns the file's basename.
func manifestFromStdout(stdout string) string {
	for _, line := range strings.Split(stdout, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "artifact=manifest file=")
		if ok && rest != "" {
			return filepath.Base(rest)
		}
	}
	return ""
}
