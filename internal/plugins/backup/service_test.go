// Tests for the backup service. The service shells out to
// scripts/backup.sh in production; tests substitute /bin/true,
// /bin/false, and a sleeping shim to exercise success / failure /
// timeout paths without ever touching real backup tooling.
package backup

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// writeShim writes a tiny shell script that does whatever body says and
// exits with the given code. Returns the script path. Used so tests
// don't depend on the real backup.sh (which needs mysqldump etc).
func writeShim(t *testing.T, body string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "shim.sh")
	content := "#!/bin/sh\n" + body + "\nexit " + itoa(exitCode) + "\n"
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return path
}

// itoa avoids pulling strconv into the test file just for a single int.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// TestRunBackup_Success pins the happy path: shim exits 0, RunResult
// reports success, LastRun is populated, IsRunning returns to false.
func TestRunBackup_Success(t *testing.T) {
	script := writeShim(t, `echo ok`, 0)
	svc := NewService(Config{ScriptPath: script, Timeout: 5 * time.Second})

	r, err := svc.RunBackup(context.Background())
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if !r.Succeeded() {
		t.Errorf("expected success, got result: %+v", r)
	}
	if r.Stdout == "" {
		t.Errorf("expected stdout to be captured")
	}
	if svc.IsRunning() {
		t.Errorf("IsRunning should be false after RunBackup returns")
	}
	if last := svc.LastRun(); last == nil || !last.Succeeded() {
		t.Errorf("LastRun should reflect the successful run")
	}
}

// TestRunBackup_NonZeroExit confirms script failure surfaces in
// RunResult.ExitCode and Succeeded() returns false.
func TestRunBackup_NonZeroExit(t *testing.T) {
	script := writeShim(t, `echo failing >&2`, 3)
	svc := NewService(Config{ScriptPath: script, Timeout: 5 * time.Second})

	r, err := svc.RunBackup(context.Background())
	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if r.Succeeded() {
		t.Errorf("expected failure, got success")
	}
	if r.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", r.ExitCode)
	}
	if r.Stderr == "" {
		t.Errorf("expected stderr to be captured")
	}
}

// TestRunBackup_Timeout confirms a long-running script is killed at the
// configured timeout and the result reports TimedOut. Sleeps 5s but
// caps the test at 200ms so the test itself doesn't hang.
func TestRunBackup_Timeout(t *testing.T) {
	script := writeShim(t, `sleep 5`, 0)
	svc := NewService(Config{ScriptPath: script, Timeout: 100 * time.Millisecond})

	start := time.Now()
	r, err := svc.RunBackup(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if !r.TimedOut {
		t.Errorf("expected TimedOut, got %+v", r)
	}
	if elapsed > time.Second {
		t.Errorf("timeout did not kill child within reasonable bound: %s", elapsed)
	}
}

// TestRunBackup_SingleFlight confirms two simultaneous invocations
// don't both spawn the script — the second returns ErrAlreadyRunning.
// Uses a sleeping shim so the first run is in flight when the second
// arrives.
func TestRunBackup_SingleFlight(t *testing.T) {
	script := writeShim(t, `sleep 0.3`, 0)
	svc := NewService(Config{ScriptPath: script, Timeout: 5 * time.Second})

	var wg sync.WaitGroup
	var firstErr, secondErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, firstErr = svc.RunBackup(context.Background())
	}()

	// Give the first goroutine a moment to acquire the lock and start
	// the script. 50ms is generous on any reasonable CI machine.
	time.Sleep(50 * time.Millisecond)

	_, secondErr = svc.RunBackup(context.Background())
	if secondErr != ErrAlreadyRunning {
		t.Errorf("second concurrent RunBackup should return ErrAlreadyRunning, got %v", secondErr)
	}
	wg.Wait()
	if firstErr != nil {
		t.Errorf("first RunBackup returned %v", firstErr)
	}
}

// TestRunBackup_ContextCancel confirms cancelling the caller's context
// kills the running script promptly. Sleeps 5s, cancels at 100ms,
// expects the run to wrap up well under a second.
func TestRunBackup_ContextCancel(t *testing.T) {
	script := writeShim(t, `sleep 5`, 0)
	svc := NewService(Config{ScriptPath: script, Timeout: 5 * time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	r, err := svc.RunBackup(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if r.Succeeded() {
		t.Errorf("expected failure on context cancel, got success")
	}
	if elapsed > time.Second {
		t.Errorf("cancel did not stop child promptly: %s", elapsed)
	}
}
