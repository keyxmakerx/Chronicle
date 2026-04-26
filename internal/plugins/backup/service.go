// Package backup provides the admin-facing UI for triggering whole-database
// backups and inspecting prior runs. The actual backup mechanics live in
// scripts/backup.sh — this package shells out to that script under a
// timeout and a single-flight lock so a flood of admin clicks (or a
// runaway browser refresh) cannot spawn parallel mysqldumps.
//
// Restore is intentionally a separate package (internal/plugins/restore)
// because its blast radius is much larger. See ADR-035 (and the planned
// ADR-036 reversal) in .ai/decisions.md for the policy reasoning.
package backup

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Service is the admin-facing backup operations interface. Hidden behind
// an interface so tests can stub the script execution without spawning
// real mysqldump processes.
type Service interface {
	// RunBackup invokes scripts/backup.sh under the configured timeout.
	// Returns ErrAlreadyRunning if a backup is in progress; the caller
	// should surface that as 429-style "try again later" rather than
	// silently coalescing — operators want to know their click did not
	// start a fresh run.
	RunBackup(ctx context.Context) (*RunResult, error)

	// ListBackups returns the artifacts currently in the backup directory
	// as Lister sees them. Read-only; never mutates disk.
	ListBackups() ([]Artifact, error)

	// LastRun returns the most recent RunResult observed by this service
	// instance, or nil if no backup has run since boot.
	LastRun() *RunResult

	// IsRunning reports whether a backup is currently in flight.
	IsRunning() bool

	// BackupDir returns the configured backup directory path. Used by the
	// download handler to validate filename parameters against a known
	// root.
	BackupDir() string
}

// RunResult captures the outcome of one backup invocation. ExitCode
// follows the script's own convention: 0 success, 1 operator error,
// 2 precondition failure, 3 tool failure.
type RunResult struct {
	StartedAt   time.Time
	FinishedAt  time.Time
	ExitCode    int
	Stdout      string
	Stderr      string
	TimedOut    bool
	ErrorString string // populated when the run could not start at all
}

// Duration returns how long the run took. Zero if FinishedAt is unset.
func (r *RunResult) Duration() time.Duration {
	if r.FinishedAt.IsZero() {
		return 0
	}
	return r.FinishedAt.Sub(r.StartedAt)
}

// Succeeded reports whether the run finished with exit code 0 and did
// not time out.
func (r *RunResult) Succeeded() bool {
	return !r.TimedOut && r.ExitCode == 0 && r.ErrorString == ""
}

// Config holds tunables for the service. Defaults are sensible for a
// production deployment; tests inject smaller values to exercise the
// timeout path without sleeping for minutes.
type Config struct {
	// ScriptPath is the absolute path to backup.sh. Required.
	ScriptPath string

	// BackupDir is where backup.sh writes artifacts. Defaults to the
	// script's own default if empty (i.e. /app/data/backups in the
	// container). Must match the BACKUP_DIR env var the script reads.
	BackupDir string

	// Timeout caps a single RunBackup invocation. Default 20 minutes.
	Timeout time.Duration
}

// ErrAlreadyRunning is returned by RunBackup when another invocation is
// in flight. Callers translate this into HTTP 409 (Conflict).
var ErrAlreadyRunning = fmt.Errorf("backup already running")

// service is the production implementation. The mutex serializes
// RunBackup against itself; lastRun is guarded by the same lock so
// readers see a consistent snapshot.
type service struct {
	cfg Config

	mu      sync.Mutex
	running bool
	last    *RunResult
}

// NewService constructs a Service. Panics on missing ScriptPath because
// the caller has no recovery path for that — it is a wiring bug, not a
// runtime condition.
func NewService(cfg Config) Service {
	if cfg.ScriptPath == "" {
		panic("backup: ScriptPath is required")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 20 * time.Minute
	}
	return &service{cfg: cfg}
}

// RunBackup shells out to scripts/backup.sh under cfg.Timeout. The
// caller's context is honored: if the request is cancelled, the
// child process is killed via exec.CommandContext.
func (s *service) RunBackup(ctx context.Context) (*RunResult, error) {
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

	result := &RunResult{StartedAt: time.Now()}

	cmd := exec.CommandContext(runCtx, "sh", s.cfg.ScriptPath)
	if s.cfg.BackupDir != "" {
		cmd.Env = append(cmd.Environ(), "BACKUP_DIR="+s.cfg.BackupDir)
	}
	// Run the shell in its own process group so any descendants spawned
	// by the script (mysqldump, tar, gzip) are killed together when the
	// context is cancelled. Without this, exec.CommandContext sends
	// SIGKILL only to the immediate shell and leaves long-running child
	// processes behind to chew CPU and disk.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID == process group: SIGKILL the script, mysqldump,
		// gzip, tar, and any other descendants in one shot.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutBuf, stderrBuf := newCapBuf(64*1024), newCapBuf(64*1024)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	err := cmd.Run()
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

// LastRun returns a copy of the most recent RunResult. Returning a copy
// rather than a pointer to the live one prevents the templ render path
// from observing a torn mid-update value.
func (s *service) LastRun() *RunResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last == nil {
		return nil
	}
	cp := *s.last
	return &cp
}

// IsRunning reports the running flag.
func (s *service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// BackupDir returns the configured backup directory.
func (s *service) BackupDir() string { return s.cfg.BackupDir }
