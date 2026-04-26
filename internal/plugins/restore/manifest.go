package restore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ManifestSummary is the parsed view of one chronicle_manifest_*.txt file.
// We surface enough information that an operator can confirm which
// backup they're about to restore — chronicle version, migration
// version, the artifacts the restore will touch, and the file size for
// each.
type ManifestSummary struct {
	// Name is the basename of the manifest file. Used as the parameter
	// the operator submits to /admin/restore/run.
	Name string

	// ModTime is the manifest's mtime. Stand-in for "when this backup
	// was taken" since we sort and label by it.
	ModTime time.Time

	// ChronicleVersion and MigrationVersion are pulled from the manifest
	// body. Operators use these to sanity-check whether the live binary
	// is compatible with the snapshot they're about to roll back to.
	ChronicleVersion string
	MigrationVersion string

	// DBFile, MediaFile, RedisFile are the basenames of the bundled
	// artifacts. An empty string means that artifact was not produced
	// (Redis is best-effort; media may be empty if MEDIA_PATH was unset).
	DBFile    string
	MediaFile string
	RedisFile string

	// SizeBytes is the manifest file size itself. Ancillary; the bundled
	// artifact sizes appear in the manifest body but we don't surface
	// them in the list view to keep the UI scannable.
	SizeBytes int64

	// ParseError is populated when the manifest exists but couldn't be
	// fully parsed. Surfaced in the UI so the operator knows not to
	// trust the row before clicking Restore.
	ParseError string
}

// manifestPrefix is the script-side filename prefix for backup
// manifests. Mirrors the pattern in scripts/backup.sh.
const manifestPrefix = "chronicle_manifest_"

// ListManifests scans BACKUP_DIR for chronicle_manifest_*.txt files,
// parses each, and returns them newest-first. Files that exist but
// can't be parsed are still returned, with ParseError set, so the UI
// can warn rather than silently hide them.
func (s *service) ListManifests() ([]ManifestSummary, error) {
	dir := s.cfg.BackupDir
	if dir == "" {
		return nil, fmt.Errorf("backup directory not configured")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}
	out := make([]ManifestSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, manifestPrefix) || !strings.HasSuffix(name, ".txt") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		summary := ManifestSummary{
			Name:      name,
			ModTime:   info.ModTime(),
			SizeBytes: info.Size(),
		}
		if err := parseManifestInto(&summary, filepath.Join(dir, name)); err != nil {
			summary.ParseError = err.Error()
		}
		out = append(out, summary)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ModTime.After(out[j].ModTime)
	})
	return out, nil
}

// parseManifestInto reads the chronicle_manifest format from disk and
// fills the relevant fields on the summary. The format is line-oriented
// `key=value` pairs plus `db_file=NAME sha256=… size=…` style lines for
// each artifact (see scripts/backup.sh). We pull just the basenames
// here; sha + size verification is the script's job at restore time.
func parseManifestInto(s *ManifestSummary, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open manifest: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "chronicle_version="):
			s.ChronicleVersion = strings.TrimPrefix(line, "chronicle_version=")
		case strings.HasPrefix(line, "migration_version="):
			s.MigrationVersion = strings.TrimPrefix(line, "migration_version=")
		case strings.HasPrefix(line, "db_file="):
			s.DBFile = firstToken(line, "db_file=")
		case strings.HasPrefix(line, "media_file="):
			s.MediaFile = firstToken(line, "media_file=")
		case strings.HasPrefix(line, "redis_file="):
			s.RedisFile = firstToken(line, "redis_file=")
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan manifest: %w", err)
	}
	return nil
}

// firstToken returns the value of `key=VALUE` up to the first space.
// The script writes `db_file=NAME sha256=… size=…` on a single line so
// we can't just trim the prefix — we have to stop at the first space.
func firstToken(line, prefix string) string {
	rest := strings.TrimPrefix(line, prefix)
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		return rest[:i]
	}
	return rest
}

// ResolveManifestPath validates a manifest basename against BACKUP_DIR,
// rejects path-traversal attempts, and returns the absolute path. Same
// shape as backup.ResolveArtifactPath but additionally enforces that
// the file is a chronicle_manifest_*.txt — restore.sh would refuse
// other inputs anyway, but failing fast at the HTTP boundary surfaces
// the wrong-file error to the operator rather than burying it in a
// shell exit code.
func ResolveManifestPath(backupDir, name string) (string, error) {
	if backupDir == "" {
		return "", fmt.Errorf("backup directory not configured")
	}
	if name == "" {
		return "", fmt.Errorf("manifest filename is required")
	}
	if name != filepath.Base(name) || strings.ContainsRune(name, os.PathSeparator) || strings.Contains(name, "..") {
		return "", fmt.Errorf("invalid manifest filename")
	}
	if !strings.HasPrefix(name, manifestPrefix) || !strings.HasSuffix(name, ".txt") {
		return "", fmt.Errorf("not a chronicle backup manifest")
	}
	full := filepath.Clean(filepath.Join(backupDir, name))
	cleanDir := filepath.Clean(backupDir)
	if !strings.HasPrefix(full, cleanDir+string(os.PathSeparator)) && full != cleanDir {
		return "", fmt.Errorf("path escapes backup directory")
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("not a file")
	}
	return full, nil
}
