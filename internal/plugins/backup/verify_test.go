package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const goodDump = "-- MariaDB dump\n" +
	"CREATE TABLE `schema_migrations` (\n  `version` bigint\n);\n" +
	"CREATE TABLE `users` (\n  `id` char(36)\n);\n" +
	"CREATE TABLE `campaigns` (\n  `id` char(36)\n);\n" +
	"INSERT INTO `users` VALUES ('u1');\n\n" +
	"-- Dump completed on 2026-10-24  7:58:55\n"

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// writeBackupPair writes a dump and a manifest naming it, with the checksum
// of sumOf (normally the dump itself).
func writeBackupPair(t *testing.T, dir string, dump, sumOf []byte) string {
	t.Helper()
	db := "chronicle_db_20261024T075855Z.sql.gz"
	if err := os.WriteFile(filepath.Join(dir, db), dump, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(sumOf)
	man := "chronicle_manifest_20261024T075855Z.txt"
	body := "chronicle_manifest_version=1\nmigration_version=40\n" +
		"db_file=" + db + " sha256=" + hex.EncodeToString(sum[:]) + " size=1\n"
	if err := os.WriteFile(filepath.Join(dir, man), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return man
}

func TestVerifyBackup(t *testing.T) {
	good := gzipBytes(t, goodDump)
	tests := []struct {
		name    string
		dump    []byte
		sumOf   []byte
		wantErr string
	}{
		{"complete dump passes", good, good, ""},
		{"cut short before the trailer", gzipBytes(t, strings.TrimSuffix(goodDump, "-- Dump completed on 2026-10-24  7:58:55\n")), nil, "incomplete"},
		{"a core table missing", gzipBytes(t, strings.Replace(goodDump, "CREATE TABLE `campaigns`", "CREATE TABLE `other`", 1)), nil, "campaigns"},
		{"file changed since the backup", good, []byte("something else"), "checksum"},
		{"truncated gzip", good[:len(good)/2], good[:len(good)/2], "can't be read to the end"},
		{"not gzip at all", []byte("plain text"), []byte("plain text"), "can't be unpacked"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			sumOf := tt.sumOf
			if sumOf == nil {
				sumOf = tt.dump
			}
			man := writeBackupPair(t, dir, tt.dump, sumOf)
			err := VerifyBackup(dir, man)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("VerifyBackup: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("VerifyBackup error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyBackup_RejectsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"../chronicle_manifest_x.txt", "chronicle_db_x.sql.gz", "chronicle_pre_migrate_manifest_x.txt", "chronicle_manifest_x.txt.partial"} {
		if err := VerifyBackup(dir, name); err == nil {
			t.Errorf("VerifyBackup(%q) should refuse a file that isn't a backup manifest", name)
		}
	}
	// A manifest pointing outside the folder is refused before anything is opened.
	man := "chronicle_manifest_20261024T000000Z.txt"
	if err := os.WriteFile(filepath.Join(dir, man), []byte("db_file=../../etc/passwd sha256=x size=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackup(dir, man); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("manifest naming a path outside the folder: err = %v", err)
	}
}
