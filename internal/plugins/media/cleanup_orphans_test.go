package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCleanupOrphans_SkipsSymlinks pins that the CleanupOrphans walker
// refuses to delete symlinks even when they look like orphans (not present
// in ListAllFilenames) — a symlink planted into the media directory by
// another process should stay in place as a signal, not be unlinked
// silently (gosec G122, TOCTOU in filepath.Walk).
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

// TestCleanupOrphans_KeepsFileUsedOnlyByMapPicture pins that orphan cleanup is
// decided by the media_files table, never by what references a file. A picture
// placed on a map is referenced only by a map_drawings row, which
// FindReferences (entities only) does not see; its file must still survive
// because it has a media row. A true orphan next to it is still removed.
func TestCleanupOrphans_KeepsFileUsedOnlyByMapPicture(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-1 * time.Hour)
	for _, name := range []string{"map-picture.png", "stray.png"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, old, old)
	}

	repo := &mockMediaRepo{
		listAllFilenamesFn: func(context.Context) (map[string]bool, error) {
			return map[string]bool{"map-picture.png": true}, nil
		},
		findReferencesFn: func(context.Context, string, string) ([]MediaRef, error) {
			t.Error("orphan cleanup must not depend on entity references")
			return nil, nil
		},
	}
	svc := newTestMediaService(repo)
	svc.mediaPath = dir

	removed, err := svc.CleanupOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (the stray file only)", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "map-picture.png")); err != nil {
		t.Errorf("the map picture's file was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stray.png")); !os.IsNotExist(err) {
		t.Errorf("the stray file should be gone, Stat err = %v", err)
	}
}
