// Package restore provides the admin-facing UI for restoring a previous
// backup. The actual restore mechanics live in scripts/restore.sh — this
// package shells out to it under a timeout and a single-flight lock.
//
// Restore is the highest-blast-radius endpoint chronicle exposes: it
// reaches into MariaDB and overwrites every table, replaces the media
// tree on disk, and (if the backup includes one) reloads the Redis
// snapshot. ADR-035 originally deferred restore-via-UI for that reason;
// this package and ADR-036 reverse that decision in exchange for a
// confirmation flow on top of the standard auth + CSRF gates.
//
// Confirmation contract: callers POST a `confirm` form field whose value
// must equal the literal string "RESTORE". The shell script also
// supports an interactive RESTORE confirmation; we pass --yes to skip
// that one because we own the prompt now, and --force because the live
// chronicle process owns a non-empty target by definition.
package restore

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Service is the admin-facing restore operations interface.
type Service interface {
	// RunRestore invokes scripts/restore.sh with --manifest pointing to
	// the chosen artifact under BACKUP_DIR. Returns ErrAlreadyRunning
	// if a restore is in progress.
	RunRestore(ctx context.Context, manifestName string) (*RunResult, error)

	// ListManifests returns all chronicle_manifest_*.txt files in
	// BACKUP_DIR, newest first, parsed for display.
	ListManifests() ([]ManifestSummary, error)

	// LastRun returns the most recent restore RunResult observed by this
	// service instance, or nil if no restore has run since boot.
	LastRun() *RunResult

	// IsRunning reports whether a restore is currently in flight.
	IsRunning() bool

	// BackupDir returns the configured backup directory path.
	BackupDir() string
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
}

// ErrAlreadyRunning is returned when another restore is in flight.
var ErrAlreadyRunning = fmt.Errorf("restore already running")

// service is the production implementation.
type service struct {
	cfg Config

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
	return &service{cfg: cfg}
}

// RunRestore validates the manifest filename, then shells out to
// scripts/restore.sh --manifest <full path> --yes --force.
//
// --yes skips the script's interactive RESTORE prompt; we already
// confirmed via the form. --force allows restoring over a non-empty
// target; chronicle is alive against the database, so the target is
// always non-empty.
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

	cmd := exec.CommandContext(runCtx, "sh", s.cfg.ScriptPath,
		"--manifest", manifestPath,
		"--yes",
		"--force",
	)
	// Process group + cancel hook: same defense as the backup service —
	// SIGKILL the whole tree (sh, mysql, tar, gzip) when the context
	// expires or the caller cancels. Without this the script's children
	// chew CPU and disk after the parent shell dies.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}

	stdoutBuf, stderrBuf := newCapBuf(64*1024), newCapBuf(64*1024)
	cmd.Stdout = stdoutBuf
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

// IsRunning reports the running flag.
func (s *service) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// BackupDir returns the configured backup directory.
func (s *service) BackupDir() string { return s.cfg.BackupDir }
