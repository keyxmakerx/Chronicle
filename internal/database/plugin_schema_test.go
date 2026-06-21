// Package database provides connection setup for MariaDB and Redis.
// This file tests plugin migration parsing, including the duplicate-version guard.
package database

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

// TestParsePluginMigrations_DuplicateVersionGuard verifies that
// parsePluginMigrations returns an error when two .up.sql (or two .down.sql)
// files share the same version number, instead of silently overwriting one.
// Regression guard for P2-1 (bestiary 002_* collision) from
// 2026-06-20-migration-startup-safety-audit.md.
func TestParsePluginMigrations_DuplicateVersionGuard(t *testing.T) {
	tests := []struct {
		name    string
		files   fstest.MapFS
		wantErr string
	}{
		{
			name: "duplicate up.sql versions error",
			files: fstest.MapFS{
				"001_create_tables.up.sql":         {Data: []byte("CREATE TABLE IF NOT EXISTS foo (id INT);")},
				"001_create_tables.down.sql":        {Data: []byte("DROP TABLE IF EXISTS foo;")},
				"001_another_migration.up.sql":      {Data: []byte("CREATE TABLE IF NOT EXISTS bar (id INT);")},
			},
			wantErr: "duplicate plugin migration version 1: two .up.sql files found",
		},
		{
			name: "duplicate down.sql versions error",
			files: fstest.MapFS{
				"002_create_things.up.sql":          {Data: []byte("CREATE TABLE IF NOT EXISTS things (id INT);")},
				"002_create_things.down.sql":        {Data: []byte("DROP TABLE IF EXISTS things;")},
				"002_also_down.down.sql":            {Data: []byte("DROP TABLE IF EXISTS also_things;")},
			},
			wantErr: "duplicate plugin migration version 2: two .down.sql files found",
		},
		{
			name: "unique versions parse correctly",
			files: fstest.MapFS{
				"001_create_tables.up.sql":   {Data: []byte("CREATE TABLE IF NOT EXISTS foo (id INT);")},
				"001_create_tables.down.sql": {Data: []byte("DROP TABLE IF EXISTS foo;")},
				"002_add_flags.up.sql":       {Data: []byte("CREATE TABLE IF NOT EXISTS flags (id INT);")},
				"002_add_flags.down.sql":     {Data: []byte("DROP TABLE IF EXISTS flags;")},
				"003_alter_col.up.sql":       {Data: []byte("ALTER TABLE foo ADD COLUMN name VARCHAR(100);")},
				"003_alter_col.down.sql":     {Data: []byte("ALTER TABLE foo DROP COLUMN name;")},
			},
			wantErr: "",
		},
		{
			name:    "empty fs returns empty slice",
			files:   fstest.MapFS{},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var migrFS fs.FS = tt.files
			migrations, err := parsePluginMigrations(migrFS)
			if tt.wantErr != "" {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
					return
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			// For the unique-versions case, confirm order is ascending.
			for i := 1; i < len(migrations); i++ {
				if migrations[i].Version <= migrations[i-1].Version {
					t.Errorf("migrations not sorted ascending: %d then %d", migrations[i-1].Version, migrations[i].Version)
				}
			}
		})
	}
}
