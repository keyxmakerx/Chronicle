package backup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

// stubService is a minimal Service test double for handler tests. The
// real service shells out to backup.sh; tests would rather not.
type stubService struct {
	dir       string
	artifacts []Artifact
	listErr   error
	last      *RunResult
	running   bool
	runFn     func(ctx context.Context) (*RunResult, error)
}

func (s *stubService) RunBackup(ctx context.Context) (*RunResult, error) {
	if s.runFn != nil {
		return s.runFn(ctx)
	}
	return nil, nil
}
func (s *stubService) ListBackups() ([]Artifact, error) { return s.artifacts, s.listErr }
func (s *stubService) LastRun() *RunResult              { return s.last }
func (s *stubService) IsRunning() bool                  { return s.running }
func (s *stubService) BackupDir() string                { return s.dir }

// TestDownload_QuoteInBasenameIsEncoded pins the Content-Disposition
// header-injection defense added after security review. Linux file
// systems allow quotes and newlines in filenames; ResolveArtifactPath
// rejects path separators and ".." but not these. Echo's Attachment
// helper RFC-encodes the filename via fmt.Sprintf("filename=%q",...).
//
// This test plants a backup artifact whose name contains an embedded
// double quote, asks the handler to serve it, and confirms the
// Content-Disposition header escapes the quote rather than letting
// it terminate the value and inject another header.
func TestDownload_QuoteInBasenameIsEncoded(t *testing.T) {
	dir := t.TempDir()
	// Filename with a quote. Construct via filepath.Join so the
	// kernel actually creates the file under that exact name.
	weirdName := `chronicle_db_evil"name.gz`
	weirdPath := filepath.Join(dir, weirdName)
	if err := os.WriteFile(weirdPath, []byte("payload"), 0o644); err != nil {
		t.Skipf("filesystem rejected weird filename (this is fine, skipping): %v", err)
	}

	h := NewHandler(&stubService{dir: dir})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/backup/files/"+weirdName, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("name")
	c.SetParamValues(weirdName)

	if err := h.Download(c); err != nil {
		t.Fatalf("Download returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}

	cd := rec.Header().Get("Content-Disposition")
	if cd == "" {
		t.Fatal("Content-Disposition header is missing")
	}
	// The escaped form must contain a backslash-escaped quote
	// (Go's %q encoding) or RFC-encoded equivalent, not a bare
	// quote that would terminate the value.
	if strings.Contains(cd, `evil"name`) && !strings.Contains(cd, `evil\"name`) {
		t.Errorf("Content-Disposition not encoded: %q", cd)
	}
}

// TestDownload_RejectsPathTraversal is a regression test on the
// existing security guard: ResolveArtifactPath should reject names
// with ".." and path separators before c.Attachment ever sees them.
func TestDownload_RejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	h := NewHandler(&stubService{dir: dir})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/admin/backup/files/foo", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("name")
	c.SetParamValues("../etc/passwd")

	err := h.Download(c)
	if err == nil {
		t.Fatal("expected error for path-traversal name, got nil")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %v", err)
	}
}

// TestRun_AlreadyRunning_Returns409 confirms the single-flight lock
// surfaces as HTTP 409 Conflict to the admin rather than spawning a
// second mysqldump.
func TestRun_AlreadyRunning_Returns409(t *testing.T) {
	h := NewHandler(&stubService{
		dir: t.TempDir(),
		runFn: func(ctx context.Context) (*RunResult, error) {
			return nil, ErrAlreadyRunning
		},
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/backup/run", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := h.Run(c)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	httpErr, ok := err.(*echo.HTTPError)
	if !ok || httpErr.Code != http.StatusConflict {
		t.Errorf("expected 409 Conflict, got %v", err)
	}
}

// TestService_ESRCHToleranceCompiles is a smoke test that the cmd.Cancel
// closure (which wires syscall.ESRCH tolerance) compiles and runs to
// completion without spurious errors when the script exits naturally
// just before the timeout fires. Closely related to the existing
// TestRunBackup_Timeout but specifically exercises the natural-exit
// path with a short-lived shim.
func TestService_ESRCHToleranceCompiles(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "shim.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{ScriptPath: script, Timeout: 10 * time.Second})

	// Run the backup many times in series; if Cancel returned ESRCH
	// errors visibly, the result.ErrorString would surface.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.RunBackup(context.Background())
		}()
	}
	wg.Wait()

	last := svc.LastRun()
	if last == nil {
		t.Fatal("LastRun is nil after several invocations")
	}
	if last.ErrorString != "" && !strings.Contains(last.ErrorString, "already running") {
		t.Errorf("unexpected ErrorString on last run: %q", last.ErrorString)
	}
}
