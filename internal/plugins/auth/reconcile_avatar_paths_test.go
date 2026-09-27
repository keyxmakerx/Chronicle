package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
