package extensions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMigrations(t *testing.T) {
	// Create a temporary migration directory.
	dir := t.TempDir()

	// Write test migration files.
	files := map[string]string{
		"001_create_tables.up.sql":   "CREATE TABLE ext_test_nodes (id INT PRIMARY KEY);",
		"001_create_tables.down.sql": "DROP TABLE IF EXISTS ext_test_nodes;",
		"002_add_columns.up.sql":     "ALTER TABLE ext_test_nodes ADD COLUMN name VARCHAR(100);",
		"002_add_columns.down.sql":   "ALTER TABLE ext_test_nodes DROP COLUMN name;",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	migrations, err := ParseMigrations(dir)
	if err != nil {
		t.Fatalf("ParseMigrations() error: %v", err)
	}

	if len(migrations) != 2 {
		t.Fatalf("expected 2 migrations, got %d", len(migrations))
	}

	if migrations[0].Version != 1 {
		t.Errorf("first migration version = %d, want 1", migrations[0].Version)
	}
	if migrations[1].Version != 2 {
		t.Errorf("second migration version = %d, want 2", migrations[1].Version)
	}
	if migrations[0].UpSQL == "" {
		t.Error("first migration has empty UpSQL")
	}
	if migrations[0].DownSQL == "" {
		t.Error("first migration has empty DownSQL")
	}
}

func TestParseMigrations_MissingUpFile(t *testing.T) {
	dir := t.TempDir()

	// Only write a down file — no up file.
	if err := os.WriteFile(filepath.Join(dir, "001_test.down.sql"), []byte("DROP TABLE x;"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := ParseMigrations(dir)
	if err == nil {
		t.Error("expected error for missing up file, got nil")
	}
}

func TestParseMigrations_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	migrations, err := ParseMigrations(dir)
	if err != nil {
		t.Fatalf("ParseMigrations() error: %v", err)
	}
	if len(migrations) != 0 {
		t.Errorf("expected 0 migrations from empty dir, got %d", len(migrations))
	}
}

func TestParseMigrations_SkipsNonMigrationFiles(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"001_test.up.sql":   "CREATE TABLE ext_t_x (id INT);",
		"001_test.down.sql": "DROP TABLE ext_t_x;",
		"README.md":         "# Not a migration",
		"notes.txt":         "Some notes",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
	}

	migrations, err := ParseMigrations(dir)
	if err != nil {
		t.Fatalf("ParseMigrations() error: %v", err)
	}
	if len(migrations) != 1 {
		t.Errorf("expected 1 migration, got %d", len(migrations))
	}
}

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"my-plugin", "my_plugin"},
		{"My.Plugin", "my_plugin"},
		{"simple", "simple"},
		{"with_underscores", "with_underscores"},
		{"UPPER", "upper"},
		{"a-b.c@d", "a_b_c_d"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeSlug(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
