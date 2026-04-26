package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleanupOrphans_SkipsSymlinks pins the defense-in-depth check
// added after a gosec G122 (TOCTOU in filepath.Walk) advisory. The
// CleanupOrphans walker must refuse to delete symlinks even when
// they would otherwise look like orphans (not present in
// ListAllFilenames). Without the guard, a symlink planted into the
// media directory by another process could be unlinked unexpectedly;
// even though os.Remove on a symlink unlinks the symlink itself
// (not its target), leaving symlinks in place gives the operator
// signal that something unusual is in the media tree.
func TestCleanupOrphans_SkipsSymlinks(t *testing.T) {
	dir := t.TempDir()

	// Plant a regular orphan file. Old mtime so the 15-minute grace
	// period doesn't cover it.
	orphan := filepath.Join(dir, "orphan.bin")
	if err := os.WriteFile(orphan, []byte("orphan"), 0o644); err != nil {
		t.Fatalf("write orphan: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(orphan, old, old); err != nil {
		t.Fatalf("chtimes orphan: %v", err)
	}

	// Plant a symlink. Target need not exist; CleanupOrphans must
	// refuse to touch the symlink regardless of its destination.
	link := filepath.Join(dir, "evil.lnk")
	if err := os.Symlink("/etc/passwd", link); err != nil {
		t.Skipf("symlink unsupported in this filesystem: %v", err)
	}
	if err := os.Chtimes(link, old, old); err != nil {
		// Some filesystems don't let us set times on symlinks; not
		// critical for the assertion below.
		t.Logf("chtimes on symlink: %v", err)
	}

	repo := &mockMediaRepo{
		listAllFilenamesFn: func(ctx context.Context) (map[string]bool, error) {
			// No known files — every entry on disk is an "orphan".
			return map[string]bool{}, nil
		},
	}
	svc := newTestMediaService(repo)
	svc.mediaPath = dir

	removed, err := svc.CleanupOrphans(context.Background())
	if err != nil {
		t.Fatalf("CleanupOrphans: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (orphan only)", removed)
	}
	if _, err := os.Lstat(orphan); !os.IsNotExist(err) {
		t.Errorf("expected orphan to be removed, Lstat err = %v", err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected symlink to remain, Lstat err = %v", err)
	}
}

// TestCleanupOrphans_SkipsKnownFiles confirms the existing
// "known file" gate still works alongside the new symlink check —
// regression guard for the refactor.
func TestCleanupOrphans_SkipsKnownFiles(t *testing.T) {
	dir := t.TempDir()
	known := filepath.Join(dir, "tracked.bin")
	if err := os.WriteFile(known, []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-1 * time.Hour)
	_ = os.Chtimes(known, old, old)

	repo := &mockMediaRepo{
		listAllFilenamesFn: func(ctx context.Context) (map[string]bool, error) {
			return map[string]bool{"tracked.bin": true}, nil
		},
	}
	svc := newTestMediaService(repo)
	svc.mediaPath = dir

	removed, err := svc.CleanupOrphans(context.Background())
	if err != nil {
		t.Fatalf("CleanupOrphans: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (known file must not be deleted)", removed)
	}
	if _, err := os.Stat(known); err != nil {
		t.Errorf("expected known file to remain, Stat err = %v", err)
	}
}
