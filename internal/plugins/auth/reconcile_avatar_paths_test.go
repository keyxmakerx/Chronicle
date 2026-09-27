package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// fakeLegacyAvatarRepo simulates the users table's avatar_path column well
// enough for ReconcileLegacyAvatarPaths: ListLegacyAvatarPaths filters by
// prefix exactly like the real `LIKE ?` query would, and UpdateAvatarPath(nil)
// clears the column so a cleared/migrated row stops matching that prefix.
type fakeLegacyAvatarRepo struct {
	avatarPaths map[string]string // userID -> avatar_path
}

func (f *fakeLegacyAvatarRepo) ListLegacyAvatarPaths(ctx context.Context, prefix string) (map[string]string, error) {
	out := make(map[string]string)
	for userID, path := range f.avatarPaths {
		if strings.HasPrefix(path, prefix) {
			out[userID] = path
		}
	}
	return out, nil
}

func (f *fakeLegacyAvatarRepo) UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error {
	if avatarPath == nil {
		delete(f.avatarPaths, userID)
		return nil
	}
	f.avatarPaths[userID] = *avatarPath
	return nil
}

// ClearAvatarPathIfMatches mirrors the real conditional UPDATE: it only
// clears (and reports true) when the stored value still equals expectedPath.
func (f *fakeLegacyAvatarRepo) ClearAvatarPathIfMatches(ctx context.Context, userID, expectedPath string) (bool, error) {
	if f.avatarPaths[userID] != expectedPath {
		return false, nil
	}
	delete(f.avatarPaths, userID)
	return true, nil
}

// fakeAvatarUploaderForReconciler records every call so a test can assert
// the reconciler never invokes it for a row it shouldn't touch.
type fakeAvatarUploaderForReconciler struct {
	calledForUser []string
	mediaIDByUser map[string]string // userID -> media id to return
	errByUser     map[string]error  // userID -> error to return instead
}

func (f *fakeAvatarUploaderForReconciler) UploadAvatar(ctx context.Context, userID string, fileBytes []byte, originalName, mimeType string) (string, string, error) {
	f.calledForUser = append(f.calledForUser, userID)
	if err := f.errByUser[userID]; err != nil {
		return "", "", err
	}
	id := f.mediaIDByUser[userID]
	return id, "/media/" + id, nil
}

// DeleteAvatarMedia is unused by the reconciler (it never replaces or
// clears an avatar itself) but is required to satisfy AvatarUploader.
func (f *fakeAvatarUploaderForReconciler) DeleteAvatarMedia(ctx context.Context, userID, mediaID string) error {
	return nil
}

func TestReconcileLegacyAvatarPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.jpg"), []byte("fake-jpeg-bytes"), 0o600); err != nil {
		t.Fatalf("seeding avatar file: %v", err)
	}

	repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{
		"user-file-present":  "/uploads/avatars/present.jpg",
		"user-file-missing":  "/uploads/avatars/gone.jpg",
		"user-already-media": "existing-media-uuid",
	}}
	uploader := &fakeAvatarUploaderForReconciler{
		mediaIDByUser: map[string]string{"user-file-present": "new-media-id"},
	}

	moved, cleared, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
	if err != nil {
		t.Fatalf("ReconcileLegacyAvatarPaths: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved = %d, want 1", moved)
	}
	if cleared != 1 {
		t.Errorf("cleared = %d, want 1", cleared)
	}

	tests := []struct {
		name     string
		userID   string
		wantPath string // "" means the column must be absent/cleared
		wantSet  bool
	}{
		{"file present is moved into the media store", "user-file-present", "new-media-id", true},
		{"file missing is cleared", "user-file-missing", "", false},
		{"already a media id is left untouched", "user-already-media", "existing-media-uuid", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := repo.avatarPaths[tt.userID]
			if ok != tt.wantSet {
				t.Fatalf("avatarPaths[%s] present = %v, want %v", tt.userID, ok, tt.wantSet)
			}
			if ok && got != tt.wantPath {
				t.Errorf("avatarPaths[%s] = %q, want %q", tt.userID, got, tt.wantPath)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(dir, "present.jpg")); !os.IsNotExist(err) {
		t.Errorf("a successfully migrated file must be removed from the legacy directory (true move, not a copy); stat err = %v", err)
	}

	for _, userID := range []string{"user-already-media"} {
		for _, called := range uploader.calledForUser {
			if called == userID {
				t.Errorf("uploader must not be called for a row that already holds a media id (user %s)", userID)
			}
		}
	}

	// Second run: every row is now either a media id or cleared, so no row
	// matches the legacy prefix and nothing should change.
	uploader.calledForUser = nil
	before := map[string]string{}
	for k, v := range repo.avatarPaths {
		before[k] = v
	}
	moved2, cleared2, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
	if err != nil {
		t.Fatalf("second ReconcileLegacyAvatarPaths run: %v", err)
	}
	if moved2 != 0 || cleared2 != 0 {
		t.Errorf("second run: moved=%d cleared=%d, want 0/0 (idempotent)", moved2, cleared2)
	}
	if len(uploader.calledForUser) != 0 {
		t.Errorf("second run: uploader called %d times, want 0", len(uploader.calledForUser))
	}
	if len(before) != len(repo.avatarPaths) {
		t.Fatalf("second run changed the row count: before=%v after=%v", before, repo.avatarPaths)
	}
	for k, v := range before {
		if repo.avatarPaths[k] != v {
			t.Errorf("second run changed avatarPaths[%s]: %q -> %q", k, v, repo.avatarPaths[k])
		}
	}
}

func TestReconcileLegacyAvatarPaths_UploadFailure_LeavesRowUntouched(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.jpg"), []byte("fake-jpeg-bytes"), 0o600); err != nil {
		t.Fatalf("seeding avatar file: %v", err)
	}

	repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{
		"user-1": "/uploads/avatars/present.jpg",
	}}
	uploadErr := &testUploadErr{}
	uploader := &fakeAvatarUploaderForReconciler{
		errByUser: map[string]error{"user-1": uploadErr},
	}

	moved, cleared, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
	if err != nil {
		t.Fatalf("ReconcileLegacyAvatarPaths: %v", err)
	}
	if moved != 0 || cleared != 0 {
		t.Errorf("moved=%d cleared=%d, want 0/0 when the upload fails", moved, cleared)
	}
	if repo.avatarPaths["user-1"] != "/uploads/avatars/present.jpg" {
		t.Errorf("row must be left untouched after a failed upload so a later boot retries; got %q",
			repo.avatarPaths["user-1"])
	}
}

// testUploadErr is a minimal error type distinct from apperror so this test
// doesn't depend on that package's constructors.
type testUploadErr struct{}

func (*testUploadErr) Error() string { return "upload failed" }

// TestReconcileLegacyAvatarPaths_PermanentRejection_ClearsRow pins that a
// bad-request upload error (an undecodable legacy file) is treated as
// permanent -- cleared once, not retried on every future boot -- unlike the
// transient failure in TestReconcileLegacyAvatarPaths_UploadFailure_LeavesRowUntouched.
func TestReconcileLegacyAvatarPaths_PermanentRejection_ClearsRow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "present.jpg"), []byte("not-a-real-image"), 0o600); err != nil {
		t.Fatalf("seeding avatar file: %v", err)
	}

	repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{
		"user-1": "/uploads/avatars/present.jpg",
	}}
	uploader := &fakeAvatarUploaderForReconciler{
		errByUser: map[string]error{"user-1": apperror.NewBadRequest("image sanitization failed: unrecognized format")},
	}

	moved, cleared, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
	if err != nil {
		t.Fatalf("ReconcileLegacyAvatarPaths: %v", err)
	}
	if moved != 0 || cleared != 1 {
		t.Errorf("moved=%d cleared=%d, want 0/1 for a permanently rejected file", moved, cleared)
	}
	if _, ok := repo.avatarPaths["user-1"]; ok {
		t.Error("avatar_path must be cleared after a permanent (bad-request) upload rejection")
	}

	// A second run must not retry the upload -- the row no longer matches
	// the legacy prefix.
	uploader.calledForUser = nil
	if _, _, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir); err != nil {
		t.Fatalf("second ReconcileLegacyAvatarPaths run: %v", err)
	}
	if len(uploader.calledForUser) != 0 {
		t.Errorf("second run: uploader called %d times, want 0", len(uploader.calledForUser))
	}
}

// TestReconcileLegacyAvatarPaths_ClearRacesConcurrentUpload pins that a
// clear (missing file, malformed value, or permanent rejection) never
// clobbers a row a user has already overwritten with a real upload between
// the reconciler's listing and its write.
func TestReconcileLegacyAvatarPaths_ClearRacesConcurrentUpload(t *testing.T) {
	repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{
		"user-1": "/uploads/avatars/gone.jpg",
	}}

	// A concurrent real upload lands between the reconciler's listing and
	// the clear it will attempt for the (now stale) expected value.
	repo.avatarPaths["user-1"] = "fresh-real-upload-id"

	if didClear, err := repo.ClearAvatarPathIfMatches(context.Background(), "user-1", "/uploads/avatars/gone.jpg"); err != nil || didClear {
		t.Errorf("ClearAvatarPathIfMatches must refuse to clear a row that changed since the expected value was read: didClear=%v err=%v", didClear, err)
	}
	if repo.avatarPaths["user-1"] != "fresh-real-upload-id" {
		t.Errorf("a fresh real upload must never be clobbered by a stale clear; got %q", repo.avatarPaths["user-1"])
	}
}

// TestReconcileLegacyAvatarPaths_MalformedPath_Cleared pins the path-
// traversal guard: any avatar_path value that doesn't reduce to a single,
// same-directory filename is treated as unrecoverable and cleared, exactly
// like a missing file, rather than being joined into avatarsDir unchecked.
func TestReconcileLegacyAvatarPaths_MalformedPath_Cleared(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name  string
		value string
	}{
		{"empty filename", "/uploads/avatars/"},
		{"parent traversal", "/uploads/avatars/../../etc/passwd"},
		{"nested path", "/uploads/avatars/sub/dir.jpg"},
		{"backslash", "/uploads/avatars/a\\b.jpg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{"user-1": tt.value}}
			uploader := &fakeAvatarUploaderForReconciler{}

			moved, cleared, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
			if err != nil {
				t.Fatalf("ReconcileLegacyAvatarPaths: %v", err)
			}
			if moved != 0 || cleared != 1 {
				t.Errorf("moved=%d cleared=%d, want 0/1 for a malformed value", moved, cleared)
			}
			if _, ok := repo.avatarPaths["user-1"]; ok {
				t.Errorf("avatar_path must be cleared for a malformed value %q", tt.value)
			}
			if len(uploader.calledForUser) != 0 {
				t.Errorf("uploader must never be called for a malformed value %q", tt.value)
			}
		})
	}
}

// TestReconcileLegacyAvatarPaths_UnreadableFile_LeavesRowUntouched pins that
// a read failure OTHER than "file does not exist" (permissions, I/O, the
// path being a directory) leaves the row untouched rather than being
// treated like a missing file — clearing it would be a permanent,
// un-retriable loss for what may be a transient condition. A directory in
// place of the expected file reproduces such an error portably (root can
// still read a permission-denied file, so chmod alone won't trigger this).
func TestReconcileLegacyAvatarPaths_UnreadableFile_LeavesRowUntouched(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "present.jpg"), 0o755); err != nil {
		t.Fatalf("seeding a directory in place of the avatar file: %v", err)
	}

	repo := &fakeLegacyAvatarRepo{avatarPaths: map[string]string{
		"user-1": "/uploads/avatars/present.jpg",
	}}
	uploader := &fakeAvatarUploaderForReconciler{}

	moved, cleared, err := ReconcileLegacyAvatarPaths(context.Background(), repo, uploader, dir)
	if err != nil {
		t.Fatalf("ReconcileLegacyAvatarPaths: %v", err)
	}
	if moved != 0 || cleared != 0 {
		t.Errorf("moved=%d cleared=%d, want 0/0 for a non-NotExist read error", moved, cleared)
	}
	if repo.avatarPaths["user-1"] != "/uploads/avatars/present.jpg" {
		t.Errorf("row must be left untouched after a non-NotExist read error; got %q", repo.avatarPaths["user-1"])
	}
	if len(uploader.calledForUser) != 0 {
		t.Errorf("uploader must never be called when the file can't be read")
	}
}
