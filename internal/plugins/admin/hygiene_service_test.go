package admin

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/keyxmakerx/chronicle/internal/plugins/media"
)

// mockHygieneMediaRepo implements media.MediaRepository for hygiene tests.
// Only ListAllFilenames is needed by ScanStaleFiles; other methods panic
// if called (they shouldn't be).
type mockHygieneMediaRepo struct {
	media.MediaRepository // embed to satisfy interface
	filenames             map[string]bool
}

func (m *mockHygieneMediaRepo) ListAllFilenames(_ context.Context) (map[string]bool, error) {
	return m.filenames, nil
}

// mockSecurityEventRepo implements SecurityEventRepository for hygiene tests.
type mockSecurityEventRepo struct {
	events []*SecurityEvent
}

func (m *mockSecurityEventRepo) Log(_ context.Context, event *SecurityEvent) error {
	m.events = append(m.events, event)
	return nil
}

func (m *mockSecurityEventRepo) List(_ context.Context, _ string, _, _ int) ([]SecurityEvent, int, error) {
	return nil, 0, nil
}

func (m *mockSecurityEventRepo) GetStats(_ context.Context) (*SecurityStats, error) {
	return &SecurityStats{}, nil
}

func (m *mockSecurityEventRepo) CountRecentByIP(_ context.Context, _ string, _ string, _ time.Duration) (int, error) {
	return 0, nil
}

func TestScanStaleFiles_FindsUnknownFiles(t *testing.T) {
	dir := t.TempDir()

	// Create files on disk.
	writeTestFile(t, filepath.Join(dir, "known.jpg"), 100)
	writeTestFile(t, filepath.Join(dir, "unknown.png"), 200)

	// Mark files as old enough (>15 min) by backdating mod time.
	backdate(t, filepath.Join(dir, "known.jpg"))
	backdate(t, filepath.Join(dir, "unknown.png"))

	repo := &mockHygieneMediaRepo{
		filenames: map[string]bool{"known.jpg": true},
	}

	svc := &hygieneService{
		mediaRepo: repo,
		mediaPath: dir,
	}

	stale, err := svc.ScanStaleFiles(context.Background())
	if err != nil {
		t.Fatalf("ScanStaleFiles() error: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale file, got %d", len(stale))
	}
	if stale[0].Path != "unknown.png" {
		t.Errorf("stale file path = %q, want %q", stale[0].Path, "unknown.png")
	}
	if stale[0].Size != 200 {
		t.Errorf("stale file size = %d, want 200", stale[0].Size)
	}
}

func TestScanStaleFiles_SkipsRecentFiles(t *testing.T) {
	dir := t.TempDir()

	// Create a recent file (mod time is now, within 15-min threshold).
	writeTestFile(t, filepath.Join(dir, "recent.png"), 50)

	repo := &mockHygieneMediaRepo{
		filenames: map[string]bool{},
	}

	svc := &hygieneService{
		mediaRepo: repo,
		mediaPath: dir,
	}

	stale, err := svc.ScanStaleFiles(context.Background())
	if err != nil {
		t.Fatalf("ScanStaleFiles() error: %v", err)
	}

	if len(stale) != 0 {
		t.Errorf("expected 0 stale files (recent should be skipped), got %d", len(stale))
	}
}

func TestScanStaleFiles_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	repo := &mockHygieneMediaRepo{
		filenames: map[string]bool{},
	}

	svc := &hygieneService{
		mediaRepo: repo,
		mediaPath: dir,
	}

	stale, err := svc.ScanStaleFiles(context.Background())
	if err != nil {
		t.Fatalf("ScanStaleFiles() error: %v", err)
	}

	if len(stale) != 0 {
		t.Errorf("expected 0 stale files, got %d", len(stale))
	}
}

func TestPurgeStaleFiles_RemovesFiles(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "stale1.png"), 100)
	writeTestFile(t, filepath.Join(dir, "stale2.png"), 200)
	backdate(t, filepath.Join(dir, "stale1.png"))
	backdate(t, filepath.Join(dir, "stale2.png"))

	repo := &mockHygieneMediaRepo{
		filenames: map[string]bool{}, // nothing known — both are stale
	}
	secRepo := &mockSecurityEventRepo{}

	svc := &hygieneService{
		mediaRepo: repo,
		mediaPath: dir,
		secRepo:   secRepo,
	}

	purged, err := svc.PurgeStaleFiles(context.Background())
	if err != nil {
		t.Fatalf("PurgeStaleFiles() error: %v", err)
	}

	if purged != 2 {
		t.Errorf("purged = %d, want 2", purged)
	}

	// Verify files are gone.
	for _, name := range []string{"stale1.png", "stale2.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("file %q still exists after purge", name)
		}
	}

	// Verify security event was logged.
	if len(secRepo.events) != 1 {
		t.Errorf("expected 1 security event, got %d", len(secRepo.events))
	} else if secRepo.events[0].EventType != "stale_files_purged" {
		t.Errorf("event type = %q, want %q", secRepo.events[0].EventType, "stale_files_purged")
	}
}

func TestPurgeStaleFiles_NilSecurityRepo(t *testing.T) {
	dir := t.TempDir()

	writeTestFile(t, filepath.Join(dir, "stale.png"), 50)
	backdate(t, filepath.Join(dir, "stale.png"))

	repo := &mockHygieneMediaRepo{
		filenames: map[string]bool{},
	}

	svc := &hygieneService{
		mediaRepo: repo,
		mediaPath: dir,
		secRepo:   nil, // nil security repo should not panic
	}

	purged, err := svc.PurgeStaleFiles(context.Background())
	if err != nil {
		t.Fatalf("PurgeStaleFiles() error: %v", err)
	}

	if purged != 1 {
		t.Errorf("purged = %d, want 1", purged)
	}
}

func TestCountUnreferenced(t *testing.T) {
	items := []OrphanedMediaItem{
		{ID: "1", Referenced: false},
		{ID: "2", Referenced: true},
		{ID: "3", Referenced: false},
		{ID: "4", Referenced: true},
	}

	count := countUnreferenced(items)
	if count != 2 {
		t.Errorf("countUnreferenced() = %d, want 2", count)
	}
}

func TestCountUnreferenced_Empty(t *testing.T) {
	count := countUnreferenced(nil)
	if count != 0 {
		t.Errorf("countUnreferenced(nil) = %d, want 0", count)
	}
}

// writeTestFile creates a file with the given size at the specified path.
func writeTestFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	data := make([]byte, size)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

// backdate sets the file's mod time to 30 minutes ago so it's past the
// stale file threshold (15 minutes).
func backdate(t *testing.T, path string) {
	t.Helper()
	past := time.Now().Add(-30 * time.Minute)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatalf("failed to backdate file: %v", err)
	}
}
