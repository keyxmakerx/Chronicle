package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestListBackups_SortsAndClassifies pins the contract: artifacts come
// back newest-first, with kind classification reflecting the prefix
// conventions in scripts/backup.sh.
func TestListBackups_SortsAndClassifies(t *testing.T) {
	dir := t.TempDir()
	files := map[string]time.Time{
		"chronicle_db_20260101T120000.sql.gz":         time.Now().Add(-2 * time.Hour),
		"chronicle_media_20260101T120000.tar.gz":      time.Now().Add(-1 * time.Hour),
		"chronicle_pre_migrate_20260102T030000.sql.gz": time.Now().Add(-30 * time.Minute),
		"unrelated_dump.sql":                          time.Now().Add(-15 * time.Minute),
	}
	for name, ts := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	svc := &service{cfg: Config{ScriptPath: "/dev/null", BackupDir: dir}}
	got, err := svc.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 artifacts, got %d", len(got))
	}
	if got[0].Name != "unrelated_dump.sql" {
		t.Errorf("expected newest first, got order: %v", got)
	}
	expectedKinds := map[string]string{
		"chronicle_db_20260101T120000.sql.gz":          "db",
		"chronicle_media_20260101T120000.tar.gz":       "media",
		"chronicle_pre_migrate_20260102T030000.sql.gz": "pre-migrate",
		"unrelated_dump.sql":                           "other",
	}
	for _, a := range got {
		if expectedKinds[a.Name] != a.Kind {
			t.Errorf("%s: kind = %q, want %q", a.Name, a.Kind, expectedKinds[a.Name])
		}
	}
}

// TestListBackups_EmptyOrMissingDir is the "fresh deploy" case — the
// directory may not exist yet. We return nil + nil error so the page
// renders as "no artifacts" rather than as a hard failure.
func TestListBackups_EmptyOrMissingDir(t *testing.T) {
	svc := &service{cfg: Config{ScriptPath: "/dev/null", BackupDir: filepath.Join(t.TempDir(), "missing")}}
	got, err := svc.ListBackups()
	if err != nil {
		t.Errorf("expected nil error for missing dir, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil slice, got %v", got)
	}
}

// TestResolveArtifactPath_Valid confirms the happy path: a basename of
// an existing file inside BACKUP_DIR resolves to the absolute path.
func TestResolveArtifactPath_Valid(t *testing.T) {
	dir := t.TempDir()
	name := "chronicle_db_x.sql.gz"
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveArtifactPath(dir, name)
	if err != nil {
		t.Fatalf("ResolveArtifactPath: %v", err)
	}
	if !strings.HasSuffix(got, name) {
		t.Errorf("path = %q, want suffix %q", got, name)
	}
}

// TestResolveArtifactPath_RejectsTraversal pins the security guard.
// Each of these inputs would, if accepted, let an admin download files
// outside BACKUP_DIR. All must be rejected before the os.Stat.
func TestResolveArtifactPath_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{
		"../etc/passwd",
		"sub/file",
		"./file",
		"a/b",
		"",
	} {
		if _, err := ResolveArtifactPath(dir, bad); err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
	}
}

// TestResolveArtifactPath_RejectsDirectory makes sure callers can't use
// the helper to download a directory listing — file-only.
func TestResolveArtifactPath_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := "sub"
	if err := os.Mkdir(filepath.Join(dir, subdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveArtifactPath(dir, subdir); err == nil {
		t.Errorf("expected rejection of directory")
	}
}

// TestResolveArtifactPath_RejectsMissingDir refuses when BACKUP_DIR
// itself is empty (misconfiguration).
func TestResolveArtifactPath_RejectsMissingDir(t *testing.T) {
	if _, err := ResolveArtifactPath("", "x"); err == nil {
		t.Errorf("expected rejection on empty BACKUP_DIR")
	}
}
