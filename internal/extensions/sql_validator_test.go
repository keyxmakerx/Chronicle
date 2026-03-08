package extensions

import (
	"testing"
)

func TestValidateExtensionSQL(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		sql     string
		wantErr bool
	}{
		{
			name: "allows CREATE TABLE with correct prefix",
			slug: "myplugin",
			sql:  "CREATE TABLE ext_myplugin_nodes (id INT PRIMARY KEY, name VARCHAR(100));",
		},
		{
			name: "allows CREATE TABLE IF NOT EXISTS",
			slug: "myplugin",
			sql:  "CREATE TABLE IF NOT EXISTS ext_myplugin_edges (src INT, dst INT);",
		},
		{
			name: "allows ALTER TABLE with correct prefix",
			slug: "myplugin",
			sql:  "ALTER TABLE ext_myplugin_nodes ADD COLUMN description TEXT;",
		},
		{
			name: "allows DROP TABLE with correct prefix",
			slug: "myplugin",
			sql:  "DROP TABLE IF EXISTS ext_myplugin_nodes;",
		},
		{
			name: "allows CREATE INDEX on correct table",
			slug: "myplugin",
			sql:  "CREATE INDEX idx_nodes_name ON ext_myplugin_nodes (name);",
		},
		{
			name: "allows INSERT INTO correct table",
			slug: "myplugin",
			sql:  "INSERT INTO ext_myplugin_nodes (id, name) VALUES (1, 'test');",
		},
		{
			name: "allows multiple valid statements",
			slug: "myplugin",
			sql: `CREATE TABLE ext_myplugin_nodes (id INT PRIMARY KEY);
				  CREATE TABLE ext_myplugin_edges (src INT, dst INT);
				  CREATE INDEX idx_edges ON ext_myplugin_edges (src);`,
		},
		{
			name:    "rejects DROP TABLE on core table",
			slug:    "myplugin",
			sql:     "DROP TABLE users;",
			wantErr: true,
		},
		{
			name:    "rejects ALTER TABLE on core table",
			slug:    "myplugin",
			sql:     "ALTER TABLE campaigns ADD COLUMN hacked BOOLEAN;",
			wantErr: true,
		},
		{
			name:    "rejects CREATE TABLE without ext_ prefix",
			slug:    "myplugin",
			sql:     "CREATE TABLE my_custom_table (id INT);",
			wantErr: true,
		},
		{
			name:    "rejects CREATE TABLE with wrong slug",
			slug:    "myplugin",
			sql:     "CREATE TABLE ext_otherplugin_data (id INT);",
			wantErr: true,
		},
		{
			name:    "rejects INSERT INTO core table",
			slug:    "myplugin",
			sql:     "INSERT INTO users (id, email) VALUES ('x', 'hack@evil.com');",
			wantErr: true,
		},
		{
			name:    "rejects UPDATE on core table",
			slug:    "myplugin",
			sql:     "UPDATE campaigns SET name = 'hacked';",
			wantErr: true,
		},
		{
			name:    "rejects DELETE FROM core table",
			slug:    "myplugin",
			sql:     "DELETE FROM api_keys WHERE 1=1;",
			wantErr: true,
		},
		{
			name:    "rejects mixed valid and invalid",
			slug:    "myplugin",
			sql:     "CREATE TABLE ext_myplugin_ok (id INT); DROP TABLE users;",
			wantErr: true,
		},
		{
			name:    "rejects empty slug",
			slug:    "",
			sql:     "CREATE TABLE ext_test (id INT);",
			wantErr: true,
		},
		{
			name: "allows backtick-quoted table names",
			slug: "myplugin",
			sql:  "CREATE TABLE `ext_myplugin_nodes` (id INT);",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExtensionSQL(tt.slug, tt.sql)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateExtensionSQL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSplitStatements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"single", "SELECT 1", 1},
		{"two", "SELECT 1; SELECT 2", 2},
		{"with trailing semicolon", "SELECT 1;", 1},
		{"quoted semicolon", "SELECT 'a;b'", 1},
		{"empty", "", 0},
		{"whitespace only", "   ;   ", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitStatements(tt.input)
			if len(result) != tt.expected {
				t.Errorf("splitStatements() returned %d statements, expected %d: %v", len(result), tt.expected, result)
			}
		})
	}
}
