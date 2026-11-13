// Package restore provides the admin-facing UI for restoring a previous
// backup. The actual restore mechanics live in scripts/restore.sh; this
// package decides when it may run, closes the site while it does, and brings
// the site back afterwards.
//
// Restore is the highest-blast-radius action chronicle exposes: it
// overwrites every MariaDB table and replaces the media tree on disk
// (ADR-036). The admin types the literal string "RESTORE"; then Chronicle
// enters maintenance (internal/maintenance), waits for requests in flight,
// runs the script against itself with --from-inside, and on success migrates
// the restored schema and re-checks every session before opening again. On
// failure it stays closed and shows the admin what happened (ADR-073).
package restore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/keyxmakerx/chronicle/internal/maintenance"
)

// Service is the admin-facing restore operations interface.
type Service interface {
	// Start closes the site and runs the restore in the background. It
	// returns once the site is closed. ErrAlreadyRunning means a restore is
	// in progress (or the site is still closed after a failed one);
	// ErrInvalidManifest means the manifest name was rejected.
	Start(ctx context.Context, req StartRequest) error

	// Leave opens the site after a failed or interrupted restore, on an
	// admin's say-so. userID is recorded in the audit trail.
	Leave(ctx context.Context, userID string) error

	// Status is a copy of the maintenance state for the page and the poll.
	Status() maintenance.Snapshot

	// LeaveToken is the value the owner's page must send back to leave
	// maintenance; CheckLeaveToken verifies one in constant time.
	LeaveToken() string
	CheckLeaveToken(token string) bool

	// RunRestore invokes scripts/restore.sh with --manifest pointing to
	// the chosen artifact under BACKUP_DIR and waits for it. Start drives
	// it; it is separate so the script's lifecycle (timeout, process group,
	// single-flight) is testable on its own. Returns ErrAlreadyRunning if
	// the script is already running.
	RunRestore(ctx context.Context, manifestName string) (*RunResult, error)

	// ListManifests returns every backup, pre-update and pre-restore
	// manifest in BACKUP_DIR, newest first, parsed for display.
	ListManifests() ([]ManifestSummary, error)

	// LastRun returns the most recent restore RunResult observed by this
	// service instance, or nil if no restore has run since boot.
	LastRun() *RunResult

	// IsRunning reports whether a restore is currently in flight.
	IsRunning() bool

	// BackupDir returns the configured backup directory path.
	BackupDir() string

	// HandRestoreCommand is the exact shell command that restores the named
	// manifest by hand, for the failure page.
	HandRestoreCommand(manifestName string) string
}

// StartRequest says who is starting which restore.
type StartRequest struct {
	// Manifest is the manifest basename under BACKUP_DIR.
	Manifest string
	// UserID is the signed-in admin, for the audit trail.
	UserID string
	// SessionHash is the hex SHA-256 of that admin's session token
	// (maintenance.SessionHash). Only that session can see progress and
	// leave maintenance while the site is closed.
	SessionHash string
}

// Hooks are the steps that need the rest of the application. The app wires
// them; a nil Hooks makes each a no-op (tests).
type Hooks interface {
	// Quiesce disconnects clients the gate cannot refuse because they are
	// already connected (websockets).
	Quiesce(ctx context.Context)
	// ResetConnections drops pooled database connections. DROP DATABASE
	// leaves a pooled connection with no default database, and nothing
	// re-selects it when the database is created again.
	ResetConnections()
	// Migrate brings the restored schema up to this build, the same way a
	// normal start does (the backup may be older than the running image).
	Migrate(ctx context.Context) error
	// FlushCaches clears caches that held the old database's answers and
	// forces every live session to be re-checked against the restored
	// accounts. The returned text is a short plain-language result.
	FlushCaches(ctx context.Context) (string, error)
}

// ActivityRecorder is the slice of the admin change log this plugin writes to.
// It is declared here, not imported from the admin plugin, so the plugins stay
// independent; app wiring supplies an adapter. The restore finishes in a
// background goroutine, so entries carry the admin's id rather than a request.
// Recording is best-effort: implementations must not fail the restore.
type ActivityRecorder interface {
	RecordAdminChangeAs(ctx context.Context, userID, action, targetType, targetID, label string)
}

// RunResult captures the outcome of one restore invocation.
type RunResult struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	ManifestName string
	ExitCode     int
	Stdout       string
	Stderr       string
	TimedOut     bool
	ErrorString  string
}

// Duration returns how long the restore took.
func (r *RunResult) Duration() time.Duration {
	if r.FinishedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}

// Succeeded reports whether the restore finished cleanly.
func (r *RunResult) Succeeded() bool {
	return !r.TimedOut && r.ExitCode == 0 && r.ErrorString == ""
}

// Config holds tunables for the service. Defaults match production
// expectations: 30-minute timeout (restore can take longer than backup
// because mariadb is doing imports under FK constraints).
type Config struct {
	// ScriptPath is the absolute path to restore.sh. Required.
	ScriptPath string

	// BackupDir is where backup artifacts live (also where manifests
	// resolve from). Required.
	BackupDir string

	// Timeout caps a single RunRestore invocation. Default 30 minutes.
	Timeout time.Duration

	// State is the shared maintenance state the global gate also reads. If
	// nil the service keeps a private one in BackupDir, which is only right
	// for tests: nothing would close the site.
	State *maintenance.State

	// Hooks are the application-side steps; may be nil.
	Hooks Hooks

	// Activity receives the audit entries; may be nil.
	Activity ActivityRecorder

	// DrainTimeout is how long to wait for requests in flight before the
	// script starts. Default 15 seconds.
	DrainTimeout time.Duration

	// ComposeService names the docker compose service used in the
	// hand-restore command. Default "chronicle".
	ComposeService string
}

// Errors returned by Start.
var (
	// ErrAlreadyRunning is returned when another restore is in flight.
	ErrAlreadyRunning = fmt.Errorf("restore already running")
	// ErrInvalidManifest is returned for a manifest name that is not a
	// restorable file under BACKUP_DIR.
	ErrInvalidManifest = errors.New("invalid manifest")
)

// service is the production implementation.
type service struct {
	cfg   Config
	state *maintenance.State

	mu      sync.Mutex
	running bool
	last    *RunResult
}

// NewService constructs a Service.
func NewService(cfg Config) Service {
	if cfg.ScriptPath == "" {
		panic("restore: ScriptPath is required")
	}
	if cfg.BackupDir == "" {
		panic("restore: BackupDir is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Minute
	}
	if cfg.DrainTimeout == 0 {
		cfg.DrainTimeout = 15 * time.Second
	}
	if cfg.ComposeService == "" {
		cfg.ComposeService = "chronicle"
	}
	st := cfg.State
	if st == nil {
		st = maintenance.NewState(filepath.Join(cfg.BackupDir, maintenance.MarkerName))
	}
	return &service{cfg: cfg, state: st}
}

// Start enters maintenance and runs the restore in the background.
func (s *service) Start(ctx context.Context, req StartRequest) error {
	if _, err := ResolveManifestPath(s.cfg.BackupDir, req.Manifest); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if err := s.state.Enter(req.SessionHash, req.UserID, req.Manifest); err != nil {
		if errors.Is(err, maintenance.ErrBusy) {
			return ErrAlreadyRunning
		}
		return err
	}
	s.audit(ctx, req.UserID, "restore.started", req.Manifest)

	// Detached: the request that started this ends in milliseconds, and a
	// closed tab must not stop a restore halfway through the database.
	go s.execute(context.WithoutCancel(ctx), req)
	return nil
}

// execute is the whole restore after the site has closed. Every path ends in
// Succeed (open again) or Fail (closed, with the reason), never in silence.
func (s *service) execute(ctx context.Context, req StartRequest) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("restore: unexpected panic", slog.Any("panic", r))
			s.fail(ctx, req, "The restore stopped unexpectedly. The site's data may be incomplete.", fmt.Sprint(r), 1)
		}
	}()

	s.state.SetStep("Waiting for pages in use to finish")
	if !s.state.WaitIdle(s.cfg.DrainTimeout) {
		slog.Warn("restore: requests still running after the drain wait; continuing")
	}
	if s.cfg.Hooks != nil {
		s.cfg.Hooks.Quiesce(ctx)
	}

	s.state.SetStep("Starting the restore")
	res, err := s.RunRestore(ctx, req.Manifest)
	// The script may have dropped the database whatever its exit code, so
	// pooled connections are reset either way.
	if s.cfg.Hooks != nil {
		s.cfg.Hooks.ResetConnections()
	}
	if err != nil {
		s.fail(ctx, req, "The restore could not be started: "+err.Error(), "", 1)
		return
	}
	if !res.Succeeded() {
		s.fail(ctx, req, s.failureSummary(res), outputTail(res), failureExit(res))
		return
	}

	if s.cfg.Hooks != nil {
		s.state.SetStep("Bringing the restored data up to date")
		if err := s.cfg.Hooks.Migrate(ctx); err != nil {
			s.fail(ctx, req,
				"The backup was loaded, but the site could not be brought up to date with this version of Chronicle: "+err.Error(),
				outputTail(res), 0)
			return
		}
		s.state.SetStep("Checking everyone's sign-in against the restored accounts")
		note, err := s.cfg.Hooks.FlushCaches(ctx)
		if err != nil {
			s.fail(ctx, req,
				"The backup was loaded, but old cached data and sign-ins could not be cleared: "+err.Error(),
				outputTail(res), 0)
			return
		}
		s.finish(ctx, req, "Restored from "+req.Manifest+". "+note)
		return
	}
	s.finish(ctx, req, "Restored from "+req.Manifest+".")
}

func (s *service) finish(ctx context.Context, req StartRequest, summary string) {
	s.state.Succeed(summary)
	// Written after the gate opens, into the restored database.
	s.audit(ctx, req.UserID, "restore.run", req.Manifest)
}

func (s *service) fail(ctx context.Context, req StartRequest, summary, tail string, exit int) {
	s.state.Fail(summary, tail, exit)
	s.audit(ctx, req.UserID, "restore.failed", req.Manifest)
}

// failureSummary says in plain words what a stopped script means for the site.
func (s *service) failureSummary(res *RunResult) string {
	snap := s.state.Snapshot()
	var b strings.Builder
	switch {
	case res.TimedOut:
		b.WriteString("The restore took too long and was stopped. ")
	default:
		b.WriteString("The restore stopped with an error. ")
	}
	if snap.DBTouched {
		b.WriteString("It had already started replacing the database, so the site's data may be incomplete.")
	} else {
		b.WriteString("It stopped before changing anything, so the site is exactly as it was.")
	}
	return b.String()
}

func failureExit(res *RunResult) int {
	if res.TimedOut && res.ExitCode == 0 {
		return -1
	}
	return res.ExitCode
}

// outputTail is the end of what the script printed, errors first.
func outputTail(res *RunResult) string {
	const keep = 4000
	out := strings.TrimSpace(res.Stderr)
	if res.ErrorString != "" {
		out = strings.TrimSpace(res.ErrorString + "\n" + out)
	}
	if o := strings.TrimSpace(res.Stdout); o != "" {
		if out != "" {
			out += "\n\n"
		}
		out += o
	}
	if len(out) > keep {
		out = "…" + out[len(out)-keep:]
	}
	return out
}

// Leave opens the site after a failed or interrupted restore.
func (s *service) Leave(ctx context.Context, userID string) error {
	snap := s.state.Snapshot()
	if err := s.state.Leave(); err != nil {
		return err
	}
	if s.cfg.Hooks != nil {
		s.cfg.Hooks.ResetConnections()
	}
	s.audit(ctx, userID, "restore.left_maintenance", snap.Manifest)
	return nil
}

// LeaveToken returns the per-restore token for the leave form.
func (s *service) LeaveToken() string { return s.state.LeaveToken() }

// CheckLeaveToken verifies a submitted leave token.
func (s *service) CheckLeaveToken(token string) bool { return s.state.CheckLeaveToken(token) }

// Status returns the maintenance state.
func (s *service) Status() maintenance.Snapshot { return s.state.Snapshot() }

// audit writes the security log line and the admin change-log entry.
func (s *service) audit(ctx context.Context, userID, action, manifest string) {
	slog.Warn("restore audit",
		slog.String("event", action),
		slog.String("user_id", userID),
		slog.String("manifest", manifest),
	)
	if s.cfg.Activity != nil {
		s.cfg.Activity.RecordAdminChangeAs(ctx, userID, action, "backup", "", manifest)
	}
}

// HandRestoreCommand returns the by-hand command for a manifest.
func (s *service) HandRestoreCommand(manifestName string) string {
	path := filepath.Join(s.cfg.BackupDir, manifestName)
	return fmt.Sprintf(
		"docker compose stop %[1]s\ndocker compose run --rm %[1]s sh /app/scripts/restore.sh --manifest %[2]s --yes --force\ndocker compose start %[1]s",
		s.cfg.ComposeService, path)
}

// RunRestore validates the manifest filename, then shells out to
// scripts/restore.sh --manifest <full path> --yes --force. --yes skips the
// script's own prompt (the admin already typed RESTORE) and --force is needed
// because the live chronicle process owns a non-empty target by definition.
// While the site is in maintenance it also passes --from-inside and the run
// token, which is what lets the script run against a live process.
func (s *service) RunRestore(ctx context.Context, manifestName string) (*RunResult, error) {
	manifestPath, err := ResolveManifestPath(s.cfg.BackupDir, manifestName)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, ErrAlreadyRunning
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	runCtx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	result := &RunResult{StartedAt: time.Now(), ManifestName: manifestName}

	args := []string{s.cfg.ScriptPath, "--manifest", manifestPath, "--yes", "--force"}
	env := os.Environ()
	if token := s.state.RunToken(); token != "" {
		args = append(args, "--from-inside")
		env = append(env,
			"CHRONICLE_RESTORE_IN_PROCESS=1",
			"CHRONICLE_RESTORE_TOKEN="+token,
			"CHRONICLE_MAINTENANCE_FILE="+s.state.MarkerPath(),
			"BACKUP_DIR="+s.cfg.BackupDir,
		)
	}
	cmd := exec.CommandContext(runCtx, "sh", args...)
	cmd.Env = env
	// Process group + cancel hook: same defense as the backup service —
	// SIGKILL the whole tree (sh, mysql, tar, gzip) when the context
	// expires or the caller cancels. Without this the script's children
	// chew CPU and disk after the parent shell dies.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// ESRCH means the process group has already exited; treat as
		// success so a process that finishes naturally between the
		// context cancellation and our Kill call doesn't surface as a
		// spurious error.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return nil
		}
		return err
	}

	stdoutBuf, stderrBuf := newCapBuf(64*1024), newCapBuf(64*1024)
	cmd.Stdout = io.MultiWriter(stdoutBuf, &progressWriter{onLine: s.onScriptLine})
	cmd.Stderr = stderrBuf

	err = cmd.Run()
	result.FinishedAt = time.Now()
	result.Stdout = stdoutBuf.String()
	result.Stderr = stderrBuf.String()

	if runCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.ErrorString = fmt.Sprintf("timed out after %s", s.cfg.Timeout)
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ErrorString = err.Error()
		}
	}

	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
	return result, nil
}

// onScriptLine turns the script's own progress lines into the page's
// plain-language step and records what the failure page needs to know.
func (s *service) onScriptLine(line string) {
	switch {
	case strings.HasPrefix(line, "safety_backup="):
		s.state.NoteSafety(filepath.Base(strings.TrimPrefix(line, "safety_backup=")))
	case strings.HasPrefix(line, "step=safety_backup file="):
		s.state.SetStep("Saving a copy of the current site first")
	case strings.HasPrefix(line, "step=db_drop"):
		s.state.NoteDBTouched()
		s.state.SetStep("Erasing the current database")
	case strings.HasPrefix(line, "step=db_load"):
		s.state.SetStep("Loading the backup's database")
	case strings.HasPrefix(line, "step=media_inspect"):
		s.state.SetStep("Checking the backup's uploaded files")
	case strings.HasPrefix(line, "step=media_extract"):
		s.state.SetStep("Putting back the uploaded files")
	}
}

// LastRun returns a snapshot of the most recent restore result.
func (s *service) LastRun() *RunResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	cp := *s.last
	return &cp
}

// IsRunning reports whether a restore is working: the script is running, or
// the site is closed with a restore still in progress.
func (s *service) IsRunning() bool {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()
	if running {
		return true
	}
	snap := s.state.Snapshot()
	return snap.Active && snap.Phase == maintenance.PhaseRunning
}

// BackupDir returns the configured backup directory.
func (s *service) BackupDir() string { return s.cfg.BackupDir }
