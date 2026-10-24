package backup

import (
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// requiredTables must each have a CREATE TABLE in a database dump. Without
// them a restored site cannot sign anyone in or find its campaigns, and the
// server refuses to boot without schema_migrations.
var requiredTables = []string{"schema_migrations", "users", "campaigns"}

// verifyManifestPrefix is the manifest name scripts/backup.sh writes.
const verifyManifestPrefix = "chronicle_manifest_"

// dumpTrailer is the last comment mysqldump writes once every table is out;
// a dump cut short by a full disk or a killed process never has it.
const dumpTrailer = "-- Dump completed"

// VerifyBackup reads a finished backup's database dump back from end to end
// without loading it anywhere: the checksum the manifest recorded must still
// match, the file must decompress completely, the dump must have finished,
// and the core tables must be inside. It catches the backups that would fail
// a restore (cut short, empty, damaged) using only read access, since the
// app's database account cannot create a scratch database to restore into.
//
// manifestName is a basename inside backupDir. The error says, in plain
// words, which check failed.
func VerifyBackup(backupDir, manifestName string) error {
	if manifestName != filepath.Base(manifestName) || !strings.HasPrefix(manifestName, verifyManifestPrefix) ||
		!strings.HasSuffix(manifestName, ".txt") {
		return fmt.Errorf("not a backup manifest: %q", manifestName)
	}
	dbFile, wantSum, err := manifestDBEntry(filepath.Join(backupDir, manifestName))
	if err != nil {
		return err
	}
	if dbFile != filepath.Base(dbFile) {
		return fmt.Errorf("the manifest names a database file outside the backup folder")
	}
	return verifyDump(filepath.Join(backupDir, dbFile), wantSum)
}

// manifestDBEntry returns the db_file line's file name and sha256.
func manifestDBEntry(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", fmt.Errorf("the backup's list of files can't be read: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "db_file=") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "db_file="))
		if len(fields) == 0 {
			break
		}
		var sum string
		for _, kv := range fields[1:] {
			if v, ok := strings.CutPrefix(kv, "sha256="); ok {
				sum = v
			}
		}
		return fields[0], sum, nil
	}
	if err := sc.Err(); err != nil {
		return "", "", fmt.Errorf("the backup's list of files can't be read: %w", err)
	}
	return "", "", fmt.Errorf("the backup has no database file listed")
}

// verifyDump checks one gzipped dump against its recorded checksum and reads
// it through, looking for the trailer and the required tables.
func verifyDump(path, wantSum string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("the database file is missing: %w", err)
	}
	defer f.Close()

	hash := sha256.New()
	gz, err := gzip.NewReader(io.TeeReader(f, hash))
	if err != nil {
		return fmt.Errorf("the database file is damaged and can't be unpacked: %w", err)
	}
	found := make(map[string]bool, len(requiredTables))
	lastLine := ""
	sc := bufio.NewScanner(gz)
	// Extended INSERT lines run to megabytes; the cap only bounds memory.
	sc.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "CREATE TABLE `") {
			name := strings.TrimPrefix(line, "CREATE TABLE `")
			if i := strings.IndexByte(name, '`'); i > 0 {
				found[name[:i]] = true
			}
		}
		if strings.TrimSpace(line) != "" {
			lastLine = line
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("the database file stops partway and can't be read to the end: %w", err)
	}
	// Drain what the scanner left so the checksum covers the whole file.
	if _, err := io.Copy(io.Discard, io.TeeReader(f, hash)); err != nil {
		return fmt.Errorf("the database file can't be read to the end: %w", err)
	}
	if wantSum != "" && hex.EncodeToString(hash.Sum(nil)) != wantSum {
		return fmt.Errorf("the database file has changed since the backup was made (its checksum no longer matches)")
	}
	if !strings.HasPrefix(lastLine, dumpTrailer) {
		return fmt.Errorf("the database dump stops before its last line, so it is incomplete")
	}
	var missing []string
	for _, t := range requiredTables {
		if !found[t] {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the database dump is missing tables a restore needs: %s", strings.Join(missing, ", "))
	}
	return nil
}
