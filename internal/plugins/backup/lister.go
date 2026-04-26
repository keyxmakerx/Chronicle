package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Artifact describes one file in BACKUP_DIR. The admin UI lists these so
// operators can confirm the latest backup landed and download an artifact
// from outside the host shell when convenient.
type Artifact struct {
	// Name is the basename. Validated against BACKUP_DIR for traversal
	// safety before any download.
	Name string

	// SizeBytes is the on-disk size at scan time.
	SizeBytes int64

	// ModTime is the filesystem mtime, used as the "created at" label.
	ModTime time.Time

	// Kind classifies the artifact. Known kinds: "db", "media", "redis",
	// "manifest", "pre-migrate", "other". Used for grouping in the UI.
	Kind string
}

// Filename prefixes that the backup pipeline emits. Centralized so the
// classifier and the script stay in sync; if the script's naming changes,
// this list updates with it.
const (
	prefixOperatorDB    = "chronicle_db_"
	prefixOperatorMedia = "chronicle_media_"
	prefixOperatorRedis = "chronicle_redis_"
	prefixManifest      = "chronicle_backup_"
	prefixPreMigrate    = "chronicle_pre_migrate_"
)

// ListBackups scans BACKUP_DIR for known artifact filenames and returns
// them newest-first. Unknown files are reported with kind "other" rather
// than hidden — operators may have parked diagnostic files in the same
// directory, and silently dropping them would mislead the listing.
func (s *service) ListBackups() ([]Artifact, error) {
	dir := s.cfg.BackupDir
	if dir == "" {
		return nil, fmt.Errorf("backup directory not configured")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // empty directory is a valid state on a fresh deploy
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	out := make([]Artifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue // skip unreadable entries silently; ListBackups is best-effort
		}
		out = append(out, Artifact{
			Name:      entry.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime(),
			Kind:      classify(entry.Name()),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out, nil
}

// classify maps a filename to one of the known artifact kinds. Unknown
// names get "other" so the listing surfaces them rather than dropping.
func classify(name string) string {
	switch {
	case strings.HasPrefix(name, prefixOperatorDB):
		return "db"
	case strings.HasPrefix(name, prefixOperatorMedia):
		return "media"
	case strings.HasPrefix(name, prefixOperatorRedis):
		return "redis"
	case strings.HasPrefix(name, prefixManifest):
		return "manifest"
	case strings.HasPrefix(name, prefixPreMigrate):
		return "pre-migrate"
	default:
		return "other"
	}
}

// ResolveArtifactPath validates a basename against BACKUP_DIR and returns
// the absolute path if and only if the file exists inside that directory.
// Rejects path-traversal attempts and any name that escapes the configured
// backup root after Clean. Used by the download handler.
func ResolveArtifactPath(backupDir, name string) (string, error) {
	if backupDir == "" {
		return "", fmt.Errorf("backup directory not configured")
	}
	if name == "" {
		return "", fmt.Errorf("filename is required")
	}
	// Refuse anything that even smells like a path; only basenames here.
	if name != filepath.Base(name) || strings.ContainsRune(name, os.PathSeparator) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid filename")
	}
	full := filepath.Join(backupDir, name)
	cleanFull := filepath.Clean(full)
	cleanDir := filepath.Clean(backupDir)
	// Defense-in-depth: refuse if the cleaned path falls outside the
	// cleaned backup root, even though the basename check above already
	// makes this impossible. The redundancy is cheap.
	if !strings.HasPrefix(cleanFull, cleanDir+string(os.PathSeparator)) && cleanFull != cleanDir {
		return "", fmt.Errorf("path escapes backup directory")
	}
	info, err := os.Stat(cleanFull)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file")
	}
	return cleanFull, nil
}
