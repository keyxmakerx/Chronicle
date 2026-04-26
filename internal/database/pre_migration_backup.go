// Package database — pre_migration_backup.go captures a full snapshot of
// chronicle's persistent state immediately before migrations apply. Three
// artifact families plus a manifest:
//
//   chronicle_pre_migrate_db_<TS>.sql.gz       (mysqldump, gzip-compressed)
//   chronicle_pre_migrate_media_<TS>.tar.gz    (media tree, optional)
//   chronicle_pre_migrate_redis_<TS>.rdb       (redis snapshot, optional)
//   chronicle_pre_migrate_manifest_<TS>.txt    (sha256 + size for each artifact,
//                                               plus chronicle_version and
//                                               migration_version stamping)
//
// The manifest format matches scripts/backup.sh so scripts/restore.sh can
// roll forward or back from a pre-migration snapshot the same way it
// handles operator-triggered backups. The only manifest difference is a
// `chronicle_pre_migrate=1` line that lets the restore script warn that
// the bundle came from an automatic boot-time capture.
//
// Two control knobs:
//
//   - BackupDir    — empty disables the whole subsystem (legacy behavior).
//                    Required for any artifact to be written.
//   - BackupRequired — when true, any artifact failure causes
//                    PreMigrationBackup to return an error, which main()
//                    surfaces as a startup failure. The default
//                    (false) keeps the historical "best-effort, never
//                    block boot" semantics.
//
// Verification: after each artifact is written we compute sha256 + size
// in-process and refuse to finalize the manifest if any artifact is
// zero-byte or sha256 fails. Output files are written with 0600
// permissions (the dump contains all data) and atomic-renamed from a
// .partial suffix so a half-written file never persists.
package database

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// preMigrationManifestVersion is the schema version for the manifest
// format. Bumped on incompatible changes; restore.sh checks this.
const preMigrationManifestVersion = 1

// artifact is one captured file plus its verification metadata.
type artifact struct {
	kind     string // "db", "media", "redis"
	basename string
	path     string
	size     int64
	sha256   string
}

// PreMigrationBackup captures a full snapshot of the database, media
// tree, and Redis dataset before migrations apply.
//
// Returns nil when:
//   - BackupDir is empty (subsystem disabled, legacy behavior).
//   - All requested artifacts succeeded and the manifest is committed.
//
// Returns an error when:
//   - Any required artifact (DB always required if BackupDir set) fails.
//   - BackupRequired is true and ANY optional artifact fails too.
//   - Manifest writing fails after artifacts succeeded (half-state).
//
// The caller (cmd/server/main.go) decides whether to abort startup
// based on BackupRequired.
func PreMigrationBackup(db *sql.DB, cfg HealthCheckConfig) error {
	if cfg.BackupDir == "" {
		return nil
	}
	if err := os.MkdirAll(cfg.BackupDir, 0o700); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	if _, err := exec.LookPath("mysqldump"); err != nil {
		// mysqldump is the only hard requirement — without it we
		// can't snapshot the DB at all and the whole pre-migration
		// safety net is useless. In legacy fail-open mode we log
		// and return nil so boot proceeds; in fail-closed mode the
		// caller surfaces this as a startup failure.
		slog.Warn("pre-migration backup skipped: mysqldump not on PATH")
		if cfg.BackupRequired {
			return errors.New("BACKUP_REQUIRED=1 but mysqldump is not on PATH")
		}
		return nil
	}

	timestamp := time.Now().UTC().Format("20060102T150405Z")
	migrationVersion := readMigrationVersion(db)

	slog.Info("pre-migration backup starting",
		slog.String("dir", cfg.BackupDir),
		slog.String("timestamp", timestamp),
		slog.Uint64("migration_version_pre", uint64(migrationVersion)),
	)

	// Capture each artifact. The DB dump is mandatory; media and
	// redis are best-effort unless BackupRequired is true.
	var artifacts []artifact

	dbArtifact, err := snapshotDB(cfg, timestamp)
	if err != nil {
		return fmt.Errorf("DB snapshot: %w", err)
	}
	artifacts = append(artifacts, dbArtifact)

	if cfg.MediaPath != "" {
		mediaArtifact, err := snapshotMedia(cfg, timestamp)
		if err != nil {
			slog.Warn("pre-migration media snapshot failed",
				slog.Any("error", err),
			)
			if cfg.BackupRequired {
				return fmt.Errorf("media snapshot: %w", err)
			}
		} else if mediaArtifact != nil {
			artifacts = append(artifacts, *mediaArtifact)
		}
	}

	if cfg.RedisURL != "" {
		redisArtifact, err := snapshotRedis(cfg, timestamp)
		if err != nil {
			slog.Warn("pre-migration redis snapshot failed (sessions only; non-fatal in default mode)",
				slog.Any("error", err),
			)
			if cfg.BackupRequired {
				return fmt.Errorf("redis snapshot: %w", err)
			}
		} else if redisArtifact != nil {
			artifacts = append(artifacts, *redisArtifact)
		}
	}

	manifestPath, err := writeManifest(cfg, timestamp, migrationVersion, artifacts)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	slog.Info("pre-migration backup complete",
		slog.String("manifest", manifestPath),
		slog.Int("artifacts", len(artifacts)),
	)

	rotatePreMigrationBackups(cfg)
	return nil
}

// readMigrationVersion returns MAX(version) from schema_migrations, or 0
// if the table doesn't exist (fresh DB) or the query fails. We never
// return an error — a missing version annotation is a soft loss; the
// manifest still records the artifacts.
func readMigrationVersion(db *sql.DB) uint {
	if db == nil {
		return 0
	}
	var v sql.NullInt64
	row := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
	if err := row.Scan(&v); err != nil {
		slog.Debug("read pre-migration version: query failed (fresh DB?)",
			slog.Any("error", err),
		)
		return 0
	}
	if !v.Valid || v.Int64 < 0 {
		return 0
	}
	return uint(v.Int64)
}

// snapshotDB runs mysqldump and writes the gzipped output to a final
// path under BackupDir. The artifact returned has sha256 + size
// computed in-process so the manifest never references a file we
// haven't verified.
func snapshotDB(cfg HealthCheckConfig, timestamp string) (artifact, error) {
	basename := fmt.Sprintf("chronicle_pre_migrate_db_%s.sql.gz", timestamp)
	finalPath := filepath.Join(cfg.BackupDir, basename)
	partialPath := finalPath + ".partial"

	host, port := splitHostPort(cfg.DBHost, "3306")
	// mysqldump is invoked with --single-transaction so InnoDB tables
	// are dumped at a consistent snapshot without table locks.
	// --routines and --triggers cover stored procedures and triggers
	// that wouldn't otherwise survive a restore.
	dumpArgs := []string{
		"-h", host,
		"-P", port,
		"-u", cfg.DBUser,
		"--single-transaction",
		"--routines",
		"--triggers",
		cfg.DBName,
	}
	cmd := exec.Command("mysqldump", dumpArgs...)
	cmd.Env = append(cmd.Environ(), "MYSQL_PWD="+cfg.DBPassword)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return artifact{}, fmt.Errorf("stdout pipe: %w", err)
	}

	out, err := os.OpenFile(partialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return artifact{}, fmt.Errorf("create %s: %w", partialPath, err)
	}
	gzw := gzip.NewWriter(out)

	if err := cmd.Start(); err != nil {
		_ = out.Close()
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("start mysqldump: %w", err)
	}

	// Hash is computed by re-reading the on-disk file after gzip
	// closes. Matches the operator-script convention (sha256 of the
	// compressed artifact, not the raw dump bytes).
	if _, copyErr := io.Copy(gzw, stdout); copyErr != nil {
		_ = cmd.Wait()
		_ = gzw.Close()
		_ = out.Close()
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("copy mysqldump: %w", copyErr)
	}
	if err := gzw.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("close gzip: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("close file: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("mysqldump exited non-zero: %w", err)
	}

	// Verify and stamp the artifact.
	sum, size, err := sha256AndSize(partialPath)
	if err != nil {
		_ = os.Remove(partialPath)
		return artifact{}, err
	}
	if size == 0 {
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("DB snapshot is zero bytes")
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		_ = os.Remove(partialPath)
		return artifact{}, fmt.Errorf("rename: %w", err)
	}
	return artifact{kind: "db", basename: basename, path: finalPath, size: size, sha256: sum}, nil
}

// snapshotMedia walks MediaPath and writes a gzipped tarball. Returns
// nil + nil if the directory is empty (nothing worth archiving). The
// archive is rooted at the directory's basename so untar over an empty
// MEDIA_PATH reproduces the original layout.
func snapshotMedia(cfg HealthCheckConfig, timestamp string) (*artifact, error) {
	info, err := os.Stat(cfg.MediaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat media path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("media path %q is not a directory", cfg.MediaPath)
	}

	// Quick emptiness check: skip the tarball if there are no entries.
	// This avoids polluting BackupDir with empty .tar.gz files on
	// fresh installs that haven't received any uploads.
	entries, err := os.ReadDir(cfg.MediaPath)
	if err != nil {
		return nil, fmt.Errorf("read media path: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}

	basename := fmt.Sprintf("chronicle_pre_migrate_media_%s.tar.gz", timestamp)
	finalPath := filepath.Join(cfg.BackupDir, basename)
	partialPath := finalPath + ".partial"

	out, err := os.OpenFile(partialPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", partialPath, err)
	}
	gzw := gzip.NewWriter(out)
	tw := tar.NewWriter(gzw)

	walkErr := filepath.Walk(cfg.MediaPath, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip symlinks defensively — the operator backup script
		// doesn't follow them either, and we don't want a symlink
		// out of MEDIA_PATH to leak unrelated files into the bundle.
		if fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(cfg.MediaPath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		header, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		// Use forward-slash relative paths inside the archive so it
		// extracts cleanly on any platform.
		header.Name = filepath.ToSlash(rel)
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		_, err = io.Copy(tw, f)
		return err
	})
	closeErr := tw.Close()
	gzCloseErr := gzw.Close()
	fileCloseErr := out.Close()
	if walkErr != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("walk media path: %w", walkErr)
	}
	if closeErr != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("close tar: %w", closeErr)
	}
	if gzCloseErr != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("close gzip: %w", gzCloseErr)
	}
	if fileCloseErr != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("close file: %w", fileCloseErr)
	}

	sum, size, err := sha256AndSize(partialPath)
	if err != nil {
		_ = os.Remove(partialPath)
		return nil, err
	}
	if size == 0 {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("media snapshot is zero bytes")
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("rename: %w", err)
	}
	return &artifact{kind: "media", basename: basename, path: finalPath, size: size, sha256: sum}, nil
}

// snapshotRedis invokes `redis-cli -u <url> --rdb <out>` to dump the
// keyspace as an RDB file. Returns nil + nil if redis-cli is missing
// (best-effort: chronicle's only redis state is sessions, and a
// session reset is recoverable).
func snapshotRedis(cfg HealthCheckConfig, timestamp string) (*artifact, error) {
	if _, err := exec.LookPath("redis-cli"); err != nil {
		slog.Debug("redis snapshot skipped: redis-cli not on PATH")
		return nil, nil
	}
	if _, err := url.Parse(cfg.RedisURL); err != nil {
		return nil, fmt.Errorf("invalid REDIS_URL: %w", err)
	}

	basename := fmt.Sprintf("chronicle_pre_migrate_redis_%s.rdb", timestamp)
	finalPath := filepath.Join(cfg.BackupDir, basename)
	partialPath := finalPath + ".partial"

	cmd := exec.Command("redis-cli", "-u", cfg.RedisURL, "--rdb", partialPath)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("redis-cli --rdb: %w", err)
	}

	// redis-cli --rdb writes with default permissions; tighten.
	if err := os.Chmod(partialPath, 0o600); err != nil {
		// Non-fatal — file exists, just permissions are looser than
		// preferred. Log and continue.
		slog.Warn("could not tighten redis snapshot permissions",
			slog.Any("error", err),
		)
	}

	sum, size, err := sha256AndSize(partialPath)
	if err != nil {
		_ = os.Remove(partialPath)
		return nil, err
	}
	if size == 0 {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("redis snapshot is zero bytes")
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		_ = os.Remove(partialPath)
		return nil, fmt.Errorf("rename: %w", err)
	}
	return &artifact{kind: "redis", basename: basename, path: finalPath, size: size, sha256: sum}, nil
}

// writeManifest builds the per-snapshot manifest file in the format
// scripts/restore.sh expects, with one extra `chronicle_pre_migrate=1`
// line so restore tooling can distinguish boot-time bundles from
// operator-triggered ones. Atomic via .partial rename.
func writeManifest(cfg HealthCheckConfig, timestamp string, migrationVersion uint, artifacts []artifact) (string, error) {
	basename := fmt.Sprintf("chronicle_pre_migrate_manifest_%s.txt", timestamp)
	finalPath := filepath.Join(cfg.BackupDir, basename)
	partialPath := finalPath + ".partial"

	chronicleVersion := os.Getenv("CHRONICLE_VERSION")
	if chronicleVersion == "" {
		chronicleVersion = "unknown"
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "chronicle_manifest_version=%d\n", preMigrationManifestVersion)
	fmt.Fprintf(&sb, "chronicle_pre_migrate=1\n")
	fmt.Fprintf(&sb, "timestamp=%s\n", timestamp)
	fmt.Fprintf(&sb, "chronicle_version=%s\n", chronicleVersion)
	fmt.Fprintf(&sb, "migration_version=%d\n", migrationVersion)
	for _, a := range artifacts {
		fmt.Fprintf(&sb, "%s_file=%s sha256=%s size=%d\n",
			a.kind, a.basename, a.sha256, a.size)
	}

	if err := os.WriteFile(partialPath, []byte(sb.String()), 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", partialPath, err)
	}
	if err := os.Rename(partialPath, finalPath); err != nil {
		_ = os.Remove(partialPath)
		return "", fmt.Errorf("rename manifest: %w", err)
	}
	return finalPath, nil
}

// sha256AndSize streams the file, computing both in one pass.
func sha256AndSize(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("open for hashing: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// splitHostPort parses "host:port" or "host" into separate values,
// defaulting the port if absent. Tolerates IPv6 brackets via net.SplitHostPort.
func splitHostPort(addr, defaultPort string) (string, string) {
	if i := strings.LastIndex(addr, ":"); i > 0 {
		// Don't split an IPv6 bracketed address that ends with `]`.
		if !strings.Contains(addr[i:], "]") {
			return addr[:i], addr[i+1:]
		}
	}
	return addr, defaultPort
}

// rotatePreMigrationBackups removes pre-migration artifacts older than
// BackupMaxAge. Globs all four artifact families plus the legacy
// `chronicle_pre_migrate_<TS>.sql.gz` (no `_db_` infix) for backwards
// compatibility with snapshots from earlier versions.
func rotatePreMigrationBackups(cfg HealthCheckConfig) {
	maxAge := cfg.BackupMaxAge
	if maxAge == 0 {
		maxAge = 7 * 24 * time.Hour
	}
	cutoff := time.Now().Add(-maxAge)

	// Each pattern → timestamp-extraction prefix/suffix pair.
	patterns := []struct {
		glob   string
		prefix string
		suffix string
	}{
		{"chronicle_pre_migrate_db_*.sql.gz", "chronicle_pre_migrate_db_", ".sql.gz"},
		{"chronicle_pre_migrate_media_*.tar.gz", "chronicle_pre_migrate_media_", ".tar.gz"},
		{"chronicle_pre_migrate_redis_*.rdb", "chronicle_pre_migrate_redis_", ".rdb"},
		{"chronicle_pre_migrate_manifest_*.txt", "chronicle_pre_migrate_manifest_", ".txt"},
		// Legacy: pre-symmetry snapshots produced
		// chronicle_pre_migrate_<TS>.sql.gz with no infix.
		{"chronicle_pre_migrate_2*.sql.gz", "chronicle_pre_migrate_", ".sql.gz"},
	}

	removed := 0
	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(cfg.BackupDir, p.glob))
		if err != nil {
			continue
		}
		for _, f := range matches {
			base := filepath.Base(f)
			tsStr := strings.TrimPrefix(base, p.prefix)
			tsStr = strings.TrimSuffix(tsStr, p.suffix)
			t, err := time.Parse("20060102T150405Z", tsStr)
			if err != nil {
				continue
			}
			if t.Before(cutoff) {
				if err := os.Remove(f); err == nil {
					removed++
				}
			}
		}
	}
	if removed > 0 {
		slog.Info("rotated old pre-migration backups",
			slog.Int("removed", removed),
			slog.Duration("max_age", maxAge),
		)
	}
}
