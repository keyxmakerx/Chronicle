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

// TestValidateExtensionSQL_RefusesBypasses pins the statements that got past
// the old pattern checks, and the other ways a statement can read or change a
// table that isn't the extension's own.
func TestValidateExtensionSQL_RefusesBypasses(t *testing.T) {
	refused := []string{
		"DROP TABLE IF EXISTS ext_foo_a, users",
		"TRUNCATE TABLE users",
		"RENAME TABLE users TO ext_foo_x",
		"UPDATE ext_foo_a JOIN users ON 1=1 SET users.is_admin=1",
		"UPDATE ext_foo_a, users SET users.is_admin=1",
		"UPDATE ext_foo_a AS a SET a.x = 1",
		"DROP /* x */ TABLE users",
		"-- harmless\nDROP DATABASE chronicle",
		"/*! DROP TABLE users */",
		"/*M! DROP TABLE users */",
		"CREATE TABLE ext_foo_a AS SELECT * FROM users",
		"CREATE TABLE ext_foo_a SELECT password_hash FROM users",
		"CREATE TABLE ext_foo_a LIKE users",
		"CREATE TABLE ext_foo_a (id INT) ENGINE=MERGE UNION=(users)",
		"CREATE TABLE ext_foo_a (id INT) ENGINE=FEDERATED CONNECTION='mysql://u@h/db/users'",
		"CREATE TABLE ext_foo_a (id INT) ENGINE=CONNECT",
		"CREATE TABLE ext_foo_a (id INT) DATA DIRECTORY='/tmp'",
		"CREATE TABLE ext_foo_a (id INT, u CHAR(36), FOREIGN KEY (u) REFERENCES users(id))",
		"ALTER TABLE ext_foo_a RENAME TO users2",
		"ALTER TABLE ext_foo_a RENAME users2",
		"ALTER TABLE ext_foo_a EXCHANGE PARTITION p WITH TABLE users",
		"INSERT INTO ext_foo_a SELECT * FROM users",
		"INSERT INTO ext_foo_a (x) VALUES ((SELECT password_hash FROM users LIMIT 1))",
		"INSERT INTO ext_foo_a TABLE users",
		"INSERT INTO ext_foo_a (x) VALUES (LOAD_FILE('/etc/passwd'))",
		"INSERT INTO ext_foo_a (x) VALUES (@@hostname)",
		"INSERT INTO chronicle.ext_foo_a (x) VALUES (1)",
		"INSERT INTO ext_foo_a (x) VALUES (\"a\")",
		"UPDATE ext_foo_a SET x = (SELECT 1 FROM users)",
		"DELETE ext_foo_a, users FROM ext_foo_a JOIN users",
		"DELETE FROM ext_foo_a USING ext_foo_a JOIN users",
		"SET GLOBAL general_log = 1",
		"GRANT ALL ON *.* TO x",
		"CREATE TABLE ext_foo_ (id INT)",
		"CREATE TABLE ext_foo_a (id INT) /* unterminated",
		"INSERT INTO ext_foo_a (x) VALUES ('unterminated)",
		"INSERT INTO ext_foo_a (x) VALUES (1--1)",
		"SELECT * FROM users INTO OUTFILE '/tmp/x'",
	}
	for _, sql := range refused {
		t.Run(sql, func(t *testing.T) {
			if err := ValidateExtensionSQL("foo", sql); err == nil {
				t.Fatalf("accepted %q", sql)
			}
		})
	}
}

// TestParseExtensionSQL_AllowsOrdinaryMigrations is the other side: ordinary
// schema and data statements on the extension's own tables still pass.
func TestParseExtensionSQL_AllowsOrdinaryMigrations(t *testing.T) {
	allowed := []string{
		"CREATE TABLE IF NOT EXISTS ext_foo_a (id INT PRIMARY KEY, name VARCHAR(100) NOT NULL DEFAULT 'x', INDEX idx_name (name) USING BTREE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4",
		"CREATE TABLE ext_foo_b (id INT, a_id INT, FOREIGN KEY (a_id) REFERENCES ext_foo_a(id) ON DELETE CASCADE)",
		"CREATE TABLE ext_foo_c LIKE ext_foo_a",
		"ALTER TABLE ext_foo_a ADD COLUMN score DECIMAL(5,2) DEFAULT 1.5",
		"ALTER TABLE ext_foo_a RENAME COLUMN name TO title",
		"ALTER TABLE ext_foo_a RENAME TO ext_foo_renamed",
		"DROP TABLE IF EXISTS ext_foo_a, ext_foo_b",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_a ON ext_foo_a (name)",
		"DROP INDEX uq_a ON ext_foo_a",
		"INSERT IGNORE INTO ext_foo_a (id, name) VALUES (1, 'it''s; fine'), (2, 'back\\'slash')",
		"INSERT INTO ext_foo_a (id) VALUES (1) ON DUPLICATE KEY UPDATE id = id",
		"UPDATE ext_foo_a SET `table` = 'x' WHERE id >= 2 AND name LIKE 'a%'",
		"DELETE FROM ext_foo_a WHERE id <> 1",
		"DELETE FROM ext_foo_a",
		"CREATE TABLE `ext_foo_q` (`select` INT) # trailing comment",
	}
	for _, sql := range allowed {
		t.Run(sql, func(t *testing.T) {
			if _, err := ParseExtensionSQL("foo", sql); err != nil {
				t.Fatalf("refused %q: %v", sql, err)
			}
		})
	}
}

// TestParseExtensionSQL_RunsWhatItChecked pins that the runner gets the
// statements the check read, split at the same semicolons, with comments
// blanked, so the server can't see a statement the check didn't.
func TestParseExtensionSQL_RunsWhatItChecked(t *testing.T) {
	got, err := ParseExtensionSQL("foo", "INSERT INTO ext_foo_a (x) VALUES ('a;b'); -- note; DROP TABLE users\n/* c; */ DELETE FROM ext_foo_a;")
	if err != nil {
		t.Fatalf("ParseExtensionSQL: %v", err)
	}
	want := []string{"INSERT INTO ext_foo_a (x) VALUES ('a;b')", "DELETE FROM ext_foo_a"}
	if len(got) != len(want) {
		t.Fatalf("got %d statements %q, want %q", len(got), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("statement %d = %q, want %q", i, got[i], want[i])
		}
	}
}
