package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// legacyAvatarPathPrefix is the dead web path the pre-M0 avatar handler
// wrote to users.avatar_path: relative to the working directory (outside
// the Docker data volume) and served by no route (#610). A media id never
// starts with "/", so matching this prefix can't confuse a migrated row
// with an unmigrated one.
const legacyAvatarPathPrefix = "/uploads/avatars/"

// LegacyAvatarRepository is the repository surface ReconcileLegacyAvatarPaths
// needs. Satisfied by UserRepository.
type LegacyAvatarRepository interface {
	ListLegacyAvatarPaths(ctx context.Context, prefix string) (map[string]string, error)
	UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error
}

// ReconcileLegacyAvatarPaths migrates every users.avatar_path still pointing
// at the dead pre-M0 /uploads/avatars/ web path (#610): if the file still
// exists under avatarsDir, its bytes are moved into the media store through
// uploader and the column is rewritten to the returned media id; if the
// file is already gone (the common case — the path was never served and
// the directory itself lived outside the Docker volume), the column is
// cleared so the default avatar renders instead of a broken image. Rows
// that already hold a media id never match the prefix, so a second run
// changes nothing. Idempotent reconciler, not a migration; safe to re-run
// every boot. Best-effort: reports its error to the caller to log and
// continue, and never blocks startup.
func ReconcileLegacyAvatarPaths(ctx context.Context, repo LegacyAvatarRepository, uploader AvatarUploader, avatarsDir string) (moved, cleared int, err error) {
	if repo == nil || uploader == nil {
		return 0, 0, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: nil dependency (repo=%v uploader=%v)",
			repo != nil, uploader != nil)
	}

	rows, err := repo.ListLegacyAvatarPaths(ctx, legacyAvatarPathPrefix)
	if err != nil {
		return 0, 0, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: listing legacy avatar paths: %w", err)
	}

	for userID, oldPath := range rows {
		filename := strings.TrimPrefix(oldPath, legacyAvatarPathPrefix)
		// A malformed value (no filename, or one that escapes avatarsDir)
		// has nothing safe to move — treat it exactly like a missing file.
		if filename == "" || strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
			if err := repo.UpdateAvatarPath(ctx, userID, nil); err != nil {
				return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: clearing malformed avatar_path for user %s: %w", userID, err)
			}
			slog.Info("auth: cleared an unrecoverable legacy avatar_path value",
				slog.String("user_id", userID), slog.String("old_value", oldPath))
			cleared++
			continue
		}

		fullPath := filepath.Join(avatarsDir, filename)
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			// The common case: the file lived outside the Docker volume and
			// is already gone. Clear the column so the default avatar shows
			// instead of a permanent 404.
			if err := repo.UpdateAvatarPath(ctx, userID, nil); err != nil {
				return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: clearing avatar_path for user %s: %w", userID, err)
			}
			slog.Info("auth: legacy avatar file no longer exists; cleared avatar_path so the default avatar shows",
				slog.String("user_id", userID), slog.String("old_value", oldPath))
			cleared++
			continue
		}

		mediaID, _, uploadErr := uploader.UploadAvatar(ctx, userID, data, filename, http.DetectContentType(data))
		if uploadErr != nil {
			// Leave the row untouched — the file is still on disk, so a
			// later boot can retry once whatever failed (quota, disk space,
			// a transient DB error) is resolved.
			slog.Error("auth: failed to migrate a legacy avatar file into the media store; leaving avatar_path untouched for a later boot to retry",
				slog.String("user_id", userID), slog.String("old_value", oldPath), slog.Any("error", uploadErr))
			continue
		}
		if err := repo.UpdateAvatarPath(ctx, userID, &mediaID); err != nil {
			return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: updating avatar_path for user %s: %w", userID, err)
		}
		slog.Info("auth: migrated a legacy avatar file into the media store",
			slog.String("user_id", userID), slog.String("old_value", oldPath), slog.String("media_id", mediaID))
		moved++
	}

	return moved, cleared, nil
}
