package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// checkSvc records the retention a check-up run asked for and can write a
// backup pair into its folder, as the real script would.
type checkSvc struct {
	stubService
	ran      bool
	keep     int
	write    func(dir string) string // returns the manifest basename
	result   *RunResult
	runError error
}

func (s *checkSvc) RunBackupKeeping(_ context.Context, keep int) (*RunResult, error) {
	s.ran, s.keep = true, keep
	if s.runError != nil {
		return nil, s.runError
	}
	if s.result != nil {
		return s.result, nil
	}
	man := s.write(s.dir)
	return &RunResult{FinishedAt: time.Now(), Stdout: "artifact=db file=x\nartifact=manifest file=" + filepath.Join(s.dir, man) + "\nBACKUP=ok\n"}, nil
}

func TestCheckBackup(t *testing.T) {
	now := time.Now()
	good := gzipBytes(t, goodDump)
	writeGood := func(t *testing.T) func(string) string {
		return func(dir string) string { return writeBackupPair(t, dir, good, good) }
	}
	age := func(t *testing.T, dir, name string, d time.Duration) {
		at := now.Add(-d)
		if err := os.Chtimes(filepath.Join(dir, name), at, at); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a fresh backup is read back without making another", func(t *testing.T) {
		dir := t.TempDir()
		age(t, dir, writeBackupPair(t, dir, good, good), 2*time.Hour)
		svc := &checkSvc{stubService: stubService{dir: dir}}
		res, err := CheckBackup(context.Background(), svc, newMemStore(), now)
		if err != nil || !res.OK || res.MadeNow || svc.ran {
			t.Fatalf("res=%+v err=%v ran=%v", res, err, svc.ran)
		}
	})

	t.Run("an old backup means the check makes one, keeping the schedule's days", func(t *testing.T) {
		dir := t.TempDir()
		age(t, dir, writeBackupPair(t, dir, good, good), 3*24*time.Hour)
		svc := &checkSvc{stubService: stubService{dir: dir}, write: writeGood(t)}
		store := newMemStore(keyScheduleEnabled, "true", keyScheduleKeep, "30")
		res, err := CheckBackup(context.Background(), svc, store, now)
		if err != nil || !res.OK || !res.MadeNow || svc.keep != 30 {
			t.Fatalf("res=%+v err=%v keep=%d", res, err, svc.keep)
		}
	})

	t.Run("with daily backups off the run keeps the button's retention", func(t *testing.T) {
		svc := &checkSvc{stubService: stubService{dir: t.TempDir()}, write: writeGood(t)}
		res, err := CheckBackup(context.Background(), svc, newMemStore(keyScheduleKeep, "30"), now)
		if err != nil || !res.OK || svc.keep != 0 {
			t.Fatalf("res=%+v err=%v keep=%d", res, err, svc.keep)
		}
	})

	t.Run("a damaged backup fails the check", func(t *testing.T) {
		dir := t.TempDir()
		writeBackupPair(t, dir, good[:len(good)/2], good[:len(good)/2])
		res, err := CheckBackup(context.Background(), &checkSvc{stubService: stubService{dir: dir}}, nil, now)
		if err != nil || res.OK || !strings.Contains(res.Problem, "can't be restored. The database file") {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("a failed run fails the check", func(t *testing.T) {
		svc := &checkSvc{stubService: stubService{dir: t.TempDir()}, result: &RunResult{ExitCode: 3}}
		res, err := CheckBackup(context.Background(), svc, nil, now)
		if err != nil || res.OK || !strings.Contains(res.Problem, "exit code 3") {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})

	t.Run("a backup already running is handed back to try later", func(t *testing.T) {
		svc := &checkSvc{stubService: stubService{dir: t.TempDir()}, runError: ErrAlreadyRunning}
		if _, err := CheckBackup(context.Background(), svc, nil, now); err != ErrAlreadyRunning {
			t.Fatalf("err = %v, want ErrAlreadyRunning", err)
		}
	})

	t.Run("no backup folder", func(t *testing.T) {
		res, err := CheckBackup(context.Background(), &checkSvc{}, nil, now)
		if err != nil || res.OK || res.Problem == "" {
			t.Fatalf("res=%+v err=%v", res, err)
		}
	})
}

func TestManifestFromStdout(t *testing.T) {
	out := "artifact=db file=/b/chronicle_db_x.sql.gz size=1\nartifact=manifest file=/b/chronicle_manifest_x.txt\nretention=ok\n"
	if got := manifestFromStdout(out); got != "chronicle_manifest_x.txt" {
		t.Fatalf("got %q", got)
	}
	if got := manifestFromStdout("BACKUP=ok\n"); got != "" {
		t.Fatalf("got %q", got)
	}
}
