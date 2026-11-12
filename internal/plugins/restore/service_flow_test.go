package restore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
)

const testOwner = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type recHooks struct {
	mu         sync.Mutex
	calls      []string
	migrateErr error
	flushErr   error
	activeAt   map[string]bool
	state      *maintenance.State
}

func (h *recHooks) note(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, name)
	if h.state != nil {
		if h.activeAt == nil {
			h.activeAt = map[string]bool{}
		}
		h.activeAt[name] = h.state.Active()
	}
}
func (h *recHooks) Quiesce(context.Context) { h.note("quiesce") }
func (h *recHooks) ResetConnections()       { h.note("reset") }
func (h *recHooks) Migrate(context.Context) error {
	h.note("migrate")
	return h.migrateErr
}
func (h *recHooks) FlushCaches(context.Context) (string, error) {
	h.note("flush")
	return "flushed.", h.flushErr
}
func (h *recHooks) order() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return strings.Join(h.calls, ",")
}

type recActivity struct {
	mu      sync.Mutex
	actions []string
}

func (r *recActivity) RecordAdminChangeAs(_ context.Context, user, action, _, _, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions = append(r.actions, action+":"+user)
}
func (r *recActivity) list() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.actions, ",")
}

type flow struct {
	svc      Service
	state    *maintenance.State
	hooks    *recHooks
	activity *recActivity
	manifest string
	marker   string
}

func newFlow(t *testing.T, script string, exit int) *flow {
	t.Helper()
	backupDir := t.TempDir()
	marker := filepath.Join(backupDir, maintenance.MarkerName)
	st := maintenance.NewState(marker)
	hooks := &recHooks{state: st}
	act := &recActivity{}
	svc := NewService(Config{
		ScriptPath:   writeShim(t, script, exit),
		BackupDir:    backupDir,
		Timeout:      5 * time.Second,
		DrainTimeout: 100 * time.Millisecond,
		State:        st,
		Hooks:        hooks,
		Activity:     act,
	})
	return &flow{svc: svc, state: st, hooks: hooks, activity: act, manifest: writeManifest(t, backupDir), marker: marker}
}

func (f *flow) start(t *testing.T) {
	t.Helper()
	err := f.svc.Start(context.Background(), StartRequest{Manifest: f.manifest, UserID: "admin-1", SessionHash: testOwner})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func (f *flow) waitStopped(t *testing.T) maintenance.Snapshot {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s := f.state.Snapshot()
		if s.Phase != maintenance.PhaseRunning {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("restore did not finish")
	return maintenance.Snapshot{}
}

// waitAudit waits for the audit entry written just after the state flips.
func (f *flow) waitAudit(t *testing.T, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && f.activity.list() != want {
		time.Sleep(5 * time.Millisecond)
	}
	if got := f.activity.list(); got != want {
		t.Errorf("audit = %s, want %s", got, want)
	}
}

// TestStartSuccess walks the happy path: the site closes before anything
// else, the hooks run in order while it is closed, and it opens at the end.
func TestStartSuccess(t *testing.T) {
	f := newFlow(t, `echo step=db_drop; echo step=db_load file=x`, 0)
	f.start(t)
	if !f.state.Active() {
		t.Fatal("site not closed when Start returned")
	}
	snap := f.waitStopped(t)

	if snap.Phase != maintenance.PhaseDone || snap.Active {
		t.Fatalf("phase=%s active=%v, want done and open", snap.Phase, snap.Active)
	}
	if got := f.hooks.order(); got != "quiesce,reset,migrate,flush" {
		t.Errorf("hook order = %s", got)
	}
	for _, name := range []string{"quiesce", "reset", "migrate", "flush"} {
		if !f.hooks.activeAt[name] {
			t.Errorf("%s ran while the site was open", name)
		}
	}
	if !strings.Contains(snap.Summary, "flushed.") {
		t.Errorf("summary = %q", snap.Summary)
	}
	f.waitAudit(t, "restore.started:admin-1,restore.run:admin-1")
	if _, err := os.Stat(f.marker); err == nil {
		t.Error("marker left behind after success")
	}
}

// TestStartFailures covers every way a restore can stop; each must leave the
// site closed with a reason, never open on a half-restored database.
func TestStartFailures(t *testing.T) {
	tests := []struct {
		name         string
		script       string
		exit         int
		migrateErr   error
		flushErr     error
		wantTouched  bool
		wantSafety   string
		wantInSum    string
		wantHooks    string
		wantTailPart string
	}{
		{
			name:         "stops before touching the database",
			script:       `echo "RESTORE=failed reason=sha_mismatch detail=bad" >&2`,
			exit:         2,
			wantInSum:    "exactly as it was",
			wantHooks:    "quiesce,reset",
			wantTailPart: "sha_mismatch",
		},
		{
			name:         "stops after the database was dropped",
			script:       "echo step=safety_backup file=x.sql.gz; echo safety_backup=/b/chronicle_pre_restore_manifest_1.txt; echo step=db_drop; echo 'RESTORE=failed reason=db_load_failed' >&2",
			exit:         3,
			wantTouched:  true,
			wantSafety:   "chronicle_pre_restore_manifest_1.txt",
			wantInSum:    "may be incomplete",
			wantHooks:    "quiesce,reset",
			wantTailPart: "db_load_failed",
		},
		{
			name:       "restored but migrations fail",
			script:     `echo step=db_drop`,
			migrateErr: errors.New("migration 99 failed"),
			wantInSum:  "migration 99 failed",
			wantHooks:  "quiesce,reset,migrate",
		},
		{
			name:      "restored but caches cannot be cleared",
			script:    `echo step=db_drop`,
			flushErr:  errors.New("redis down"),
			wantInSum: "redis down",
			wantHooks: "quiesce,reset,migrate,flush",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFlow(t, tc.script, tc.exit)
			f.hooks.migrateErr, f.hooks.flushErr = tc.migrateErr, tc.flushErr
			f.start(t)
			snap := f.waitStopped(t)

			if snap.Phase != maintenance.PhaseFailed || !snap.Active {
				t.Fatalf("phase=%s active=%v, want failed and closed", snap.Phase, snap.Active)
			}
			if !strings.Contains(snap.Summary, tc.wantInSum) {
				t.Errorf("summary %q lacks %q", snap.Summary, tc.wantInSum)
			}
			if snap.Safety != tc.wantSafety {
				t.Errorf("safety = %q, want %q", snap.Safety, tc.wantSafety)
			}
			if snap.DBTouched != tc.wantTouched && tc.migrateErr == nil && tc.flushErr == nil {
				t.Errorf("DBTouched = %v, want %v", snap.DBTouched, tc.wantTouched)
			}
			if tc.wantTailPart != "" && !strings.Contains(snap.Tail, tc.wantTailPart) {
				t.Errorf("tail %q lacks %q", snap.Tail, tc.wantTailPart)
			}
			if got := f.hooks.order(); got != tc.wantHooks {
				t.Errorf("hooks = %s, want %s", got, tc.wantHooks)
			}
			f.waitAudit(t, "restore.started:admin-1,restore.failed:admin-1")
			if _, err := os.Stat(f.marker); err != nil {
				t.Error("marker removed after a failure; a restart would open the site")
			}
		})
	}
}

// TestStartRefusals pins what Start rejects before closing the site.
func TestStartRefusals(t *testing.T) {
	f := newFlow(t, `sleep 1`, 0)
	if err := f.svc.Start(context.Background(), StartRequest{Manifest: "../etc/passwd", SessionHash: testOwner}); !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("bad manifest: %v, want ErrInvalidManifest", err)
	}
	if f.state.Active() {
		t.Error("a rejected manifest closed the site")
	}
	f.start(t)
	if err := f.svc.Start(context.Background(), StartRequest{Manifest: f.manifest, SessionHash: testOwner}); !errors.Is(err, ErrAlreadyRunning) {
		t.Errorf("second start: %v, want ErrAlreadyRunning", err)
	}
	if !f.svc.IsRunning() {
		t.Error("IsRunning false while the restore runs")
	}
	f.waitStopped(t)
}

// TestRestartMidRestore is the crash story end to end: a process that dies
// while the script runs comes back closed, names what was happening, and the
// admin can then leave maintenance.
func TestRestartMidRestore(t *testing.T) {
	f := newFlow(t, `echo step=db_drop; echo safety_backup=/b/chronicle_pre_restore_manifest_9.txt; sleep 2`, 0)
	f.start(t)
	// Wait until the script has reported its steps, then "crash": a new
	// process reads only what is on disk.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && f.state.Snapshot().Safety == "" {
		time.Sleep(10 * time.Millisecond)
	}
	reborn := maintenance.NewState(f.marker)
	snap := reborn.Snapshot()
	if !snap.Active || snap.Phase != maintenance.PhaseInterrupted {
		t.Fatalf("after restart active=%v phase=%s, want closed and interrupted", snap.Active, snap.Phase)
	}
	if snap.Safety != "chronicle_pre_restore_manifest_9.txt" || !snap.DBTouched || snap.Manifest != f.manifest {
		t.Errorf("snapshot lost details: %+v", snap)
	}

	svc2 := NewService(Config{ScriptPath: writeShim(t, "true", 0), BackupDir: filepath.Dir(f.marker), State: reborn})
	if err := svc2.Leave(context.Background(), "admin-1"); err != nil {
		t.Fatalf("Leave: %v", err)
	}
	if reborn.Active() {
		t.Error("still closed after leaving")
	}
	if _, err := os.Stat(f.marker); err == nil {
		t.Error("marker survived leaving; the next boot would close again")
	}
	f.waitStopped(t) // let the first process's goroutine finish before cleanup
}

// TestLeave pins that leaving is refused mid-restore and audited otherwise.
func TestLeave(t *testing.T) {
	f := newFlow(t, `echo "RESTORE=failed" >&2`, 3)
	f.start(t)
	f.waitStopped(t)
	if err := f.svc.Leave(context.Background(), "admin-1"); err != nil {
		t.Fatalf("Leave after failure: %v", err)
	}
	if f.state.Active() {
		t.Error("still closed")
	}
	if got := f.activity.list(); !strings.HasSuffix(got, "restore.left_maintenance:admin-1") {
		t.Errorf("audit = %s", got)
	}
	if err := f.svc.Leave(context.Background(), "admin-1"); !errors.Is(err, maintenance.ErrNotActive) {
		t.Errorf("second Leave: %v, want ErrNotActive", err)
	}
}

// TestScriptReceivesInsideFlags pins what lets the script run against a live
// process: the flag and the token the marker holds.
func TestScriptReceivesInsideFlags(t *testing.T) {
	out := filepath.Join(t.TempDir(), "seen")
	f := newFlow(t, `echo "$@" > `+out+`; echo "tok=$CHRONICLE_RESTORE_TOKEN file=$CHRONICLE_MAINTENANCE_FILE" >> `+out, 0)
	f.start(t)
	f.waitStopped(t)
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{"--from-inside", "--yes", "--force", "--manifest", "file=" + f.marker} {
		if !strings.Contains(got, want) {
			t.Errorf("script saw %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "tok=\n") || !strings.Contains(got, "tok=") {
		t.Errorf("script got no run token: %q", got)
	}
}

// TestHandRestoreCommand pins the command shown on the failure page.
func TestHandRestoreCommand(t *testing.T) {
	svc := NewService(Config{ScriptPath: "/x", BackupDir: "/app/data/backups"})
	got := svc.HandRestoreCommand("chronicle_pre_restore_manifest_1.txt")
	for _, want := range []string{
		"docker compose stop chronicle",
		"restore.sh --manifest /app/data/backups/chronicle_pre_restore_manifest_1.txt --yes --force",
		"docker compose start chronicle",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("command lacks %q:\n%s", want, got)
		}
	}
}

// TestPreRestoreManifestListed pins that the safety copy shows up in the
// restore list, flagged, so the admin can undo a restore from the page.
func TestPreRestoreManifestListed(t *testing.T) {
	dir := t.TempDir()
	body := "chronicle_pre_restore=1\nmigration_version=5/0\ndb_file=chronicle_pre_restore_db_1.sql.gz sha256=a size=1\n"
	if err := os.WriteFile(filepath.Join(dir, "chronicle_pre_restore_manifest_1.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{ScriptPath: "/x", BackupDir: dir})
	list, err := svc.ListManifests()
	if err != nil || len(list) != 1 || !list[0].PreRestore {
		t.Fatalf("list = %+v, err %v", list, err)
	}
	if _, err := ResolveManifestPath(dir, "chronicle_pre_restore_manifest_1.txt"); err != nil {
		t.Errorf("safety manifest not restorable: %v", err)
	}
}
