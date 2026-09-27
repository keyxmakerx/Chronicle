package auth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// legacyAvatarPathPrefix is the dead web path a legacy avatar handler wrote
// to users.avatar_path: relative to the working directory (outside the
// Docker data volume) and served by no route. A media id never starts with
// "/", so matching this prefix can't confuse a migrated row with an
// unmigrated one.
const legacyAvatarPathPrefix = "/uploads/avatars/"

// LegacyAvatarRepository is the repository surface ReconcileLegacyAvatarPaths
// needs. Satisfied by UserRepository.
type LegacyAvatarRepository interface {
	ListLegacyAvatarPaths(ctx context.Context, prefix string) (map[string]string, error)
	UpdateAvatarPath(ctx context.Context, userID string, avatarPath *string) error
	ClearAvatarPathIfMatches(ctx context.Context, userID, expectedPath string) (bool, error)
}

// ReconcileLegacyAvatarPaths migrates every users.avatar_path still pointing
// at a dead legacy /uploads/avatars/ web path: if the file still exists
// under avatarsDir, its bytes are moved into the media store through
// uploader and the column is rewritten to the returned media id; if the
// file is already gone (the common case — the path was never served and
// the directory itself lived outside the Docker volume), the column is
// cleared so the default avatar renders instead of a broken image. Every
// clear uses ClearAvatarPathIfMatches rather than an unconditional write,
// so a row a user has already overwritten with a real upload between the
// listing above and this call is never clobbered back to NULL. Rows that
// already hold a media id never match the prefix, so a second run changes
// nothing. Idempotent reconciler, not a migration; safe to re-run every
// boot. Best-effort: reports its error to the caller to log and continue,
// and never blocks startup.
func ReconcileLegacyAvatarPaths(ctx context.Context, repo LegacyAvatarRepository, uploader AvatarUploader, avatarsDir string) (moved, cleared int, err error) {
	if repo == nil || uploader == nil {
		return 0, 0, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: nil dependency (repo=%v uploader=%v)",
			repo != nil, uploader != nil)
	}

	absDir, absErr := filepath.Abs(avatarsDir)
	if absErr != nil {
		absDir = avatarsDir
	}
	slog.Info("auth: scanning for legacy avatar files", slog.String("dir", absDir))

	rows, err := repo.ListLegacyAvatarPaths(ctx, legacyAvatarPathPrefix)
	if err != nil {
		return 0, 0, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: listing legacy avatar paths: %w", err)
	}

	for userID, oldPath := range rows {
		filename := strings.TrimPrefix(oldPath, legacyAvatarPathPrefix)
		// A malformed value (no filename, or one that escapes avatarsDir)
		// has nothing safe to move — treat it exactly like a missing file.
		if filename == "" || strings.ContainsAny(filename, "/\\") || strings.Contains(filename, "..") {
			didClear, clearErr := repo.ClearAvatarPathIfMatches(ctx, userID, oldPath)
			if clearErr != nil {
				return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: clearing malformed avatar_path for user %s: %w", userID, clearErr)
			}
			if didClear {
				slog.Info("auth: cleared an unrecoverable legacy avatar_path value",
					slog.String("user_id", userID), slog.String("old_value", oldPath))
				cleared++
			}
			continue
		}

		fullPath := filepath.Join(avatarsDir, filename)
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			if !errors.Is(readErr, fs.ErrNotExist) {
				// Something other than "file is gone" (permissions, I/O,
				// the path being a directory): the file may still be
				// there, so leave the row untouched for a later boot to
				// retry, exactly like a failed upload below. Clearing here
				// would be a permanent, un-retriable data loss for a
				// transient condition.
				slog.Error("auth: failed to read a legacy avatar file; leaving avatar_path untouched for a later boot to retry",
					slog.String("user_id", userID), slog.String("old_value", oldPath), slog.String("dir", absDir), slog.Any("error", readErr))
				continue
			}
			// The common case: the file lived outside the Docker volume and
			// is already gone. Clear the column so the default avatar shows
			// instead of a permanent 404.
			didClear, clearErr := repo.ClearAvatarPathIfMatches(ctx, userID, oldPath)
			if clearErr != nil {
				return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: clearing avatar_path for user %s: %w", userID, clearErr)
			}
			if didClear {
				slog.Info("auth: legacy avatar file no longer exists; cleared avatar_path so the default avatar shows",
					slog.String("user_id", userID), slog.String("old_value", oldPath), slog.String("dir", absDir))
				cleared++
			}
			continue
		}

		mediaID, _, uploadErr := uploader.UploadAvatar(ctx, userID, data, filename, http.DetectContentType(data))
		if uploadErr != nil {
			if apperror.SafeCode(uploadErr) == http.StatusBadRequest {
				// The media pipeline permanently refuses this file's bytes
				// (e.g. an image it can't decode) -- that verdict will be
				// identical on every future boot, so retrying forever only
				// wastes work. Clear the row instead, same as a missing file.
				didClear, clearErr := repo.ClearAvatarPathIfMatches(ctx, userID, oldPath)
				if clearErr != nil {
					return moved, cleared, fmt.Errorf("auth.ReconcileLegacyAvatarPaths: clearing unrecoverable avatar_path for user %s: %w", userID, clearErr)
				}
				if didClear {
					slog.Info("auth: legacy avatar file was permanently rejected by the media pipeline; cleared avatar_path",
						slog.String("user_id", userID), slog.String("old_value", oldPath), slog.Any("error", uploadErr))
					cleared++
				}
				continue
			}
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
		// The column now points at the media store's own copy; remove the
		// legacy file so it doesn't sit in the volume forever. Best-effort:
		// the row is already correctly migrated either way, and admin
		// hygiene's orphan scan would eventually need to cover this
		// directory too if it didn't run here.
		if rmErr := os.Remove(fullPath); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			slog.Warn("auth: migrated a legacy avatar file but could not remove the original",
				slog.String("user_id", userID), slog.String("path", fullPath), slog.Any("error", rmErr))
		}
		slog.Info("auth: migrated a legacy avatar file into the media store",
			slog.String("user_id", userID), slog.String("old_value", oldPath), slog.String("media_id", mediaID))
		moved++
	}

	return moved, cleared, nil
}
