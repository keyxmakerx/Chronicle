package database

import (
	"context"
	"strings"
	"testing"
)

// --- Pre-flight classification (no database needed) ----------------------

// TestCheckStatementApplicable_ClassifiesTheUnrecoverableShapes covers the
// table-level DDL whose failure is what leaves a migration half-applied and a
// plugin permanently degraded. Each case states the schema the statement meets.
func TestCheckStatementApplicable_ClassifiesTheUnrecoverableShapes(t *testing.T) {
	tests := []struct {
		name    string
		have    []string // tables that already exist
		stmt    string
		wantErr string   // "" = must be allowed
		wantNow []string // catalogue after the statement, when it is allowed
	}{
		{
			name:    "bare CREATE TABLE over an existing table is refused",
			have:    []string{"things"},
			stmt:    "CREATE TABLE things (id INT)",
			wantErr: "CREATE TABLE things: the table already exists",
		},
		{
			name:    "CREATE TABLE IF NOT EXISTS is always applicable",
			have:    []string{"things"},
			stmt:    "CREATE TABLE IF NOT EXISTS things (id INT)",
			wantNow: []string{"things"},
		},
		{
			name:    "bare DROP TABLE of an absent table is refused",
			have:    nil,
			stmt:    "DROP TABLE gone",
			wantErr: "DROP TABLE gone: the table does not exist",
		},
		{
			name:    "DROP TABLE IF EXISTS is always applicable",
			have:    nil,
			stmt:    "DROP TABLE IF EXISTS gone",
			wantNow: nil,
		},
		{
			// The exact shape of data/fvtt-fresh-db-rename: an upgrade-path
			// RENAME meeting a database that was never in the state it
			// upgrades from.
			name:    "RENAME with no source table is refused, and says why",
			have:    nil,
			stmt:    "RENAME TABLE old_name TO new_name",
			wantErr: "the source table does not exist",
		},
		{
			name:    "RENAME onto an occupied name is refused",
			have:    []string{"old_name", "new_name"},
			stmt:    "RENAME TABLE old_name TO new_name",
			wantErr: "the destination table already exists",
		},
		{
			name:    "RENAME moves the name in the catalogue",
			have:    []string{"old_name"},
			stmt:    "RENAME TABLE old_name TO new_name",
			wantNow: []string{"new_name"},
		},
		{
			name:    "ALTER on an absent table is refused",
			have:    nil,
			stmt:    "ALTER TABLE nope ADD COLUMN c INT",
			wantErr: "ALTER TABLE nope: the table does not exist",
		},
		{
			name:    "ALTER on a present table is allowed",
			have:    []string{"things"},
			stmt:    "ALTER TABLE `things` ADD COLUMN c INT",
			wantNow: []string{"things"},
		},
		{
			name:    "CREATE INDEX on an absent table is refused",
			have:    nil,
			stmt:    "CREATE UNIQUE INDEX idx_x ON nope (c)",
			wantErr: "CREATE INDEX ... ON nope: the table does not exist",
		},
		{
			// The pre-flight narrows the failure surface; it must never
			// invent a refusal for a statement it cannot classify.
			name:    "an unclassifiable statement is passed through",
			have:    nil,
			stmt:    "INSERT INTO whatever (id) VALUES (1)",
			wantNow: nil,
		},
		{
			name:    "backquoted and schema-qualified names are understood",
			have:    []string{"things"},
			stmt:    "DROP TABLE `chronicle`.`things`",
			wantNow: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cat := make(tableCatalogue)
			for _, n := range tt.have {
				cat[n] = true
			}

			err := checkStatementApplicable(cat, tt.stmt)

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("statement was allowed but cannot succeed: %s", tt.stmt)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("statement was refused but is applicable: %s\nerror: %v", tt.stmt, err)
			}
			want := make(tableCatalogue)
			for _, n := range tt.wantNow {
				want[n] = true
			}
			if len(cat) != len(want) {
				t.Fatalf("catalogue = %v, want %v", cat, want)
			}
			for n := range want {
				if !cat[n] {
					t.Fatalf("catalogue = %v, want %v", cat, want)
				}
			}
		})
	}
}

// TestPreflight_SimulatesForwardWithinOneMigration is the case that would make
// this check worse than useless if it were wrong. A migration that creates a
// table and then alters it is completely normal, and the altered table does
// not exist in the pre-migration catalogue. Checking every statement against
// only the starting schema would refuse a perfectly good migration — a false
// abort is the one outcome worse than the partial-application bug.
func TestPreflight_SimulatesForwardWithinOneMigration(t *testing.T) {
	cat := make(tableCatalogue)
	stmts := []string{
		"CREATE TABLE IF NOT EXISTS brand_new (id INT PRIMARY KEY)",
		"ALTER TABLE brand_new ADD COLUMN note VARCHAR(50)",
		"CREATE INDEX idx_note ON brand_new (note)",
		"DROP TABLE brand_new",
	}
	for i, s := range stmts {
		if err := checkStatementApplicable(cat, s); err != nil {
			t.Fatalf("statement %d of a self-consistent migration was refused: %v\nSQL: %s", i+1, err, s)
		}
	}
}

// --- Resume-offset bookkeeping -------------------------------------------

// TestMigrationSQLSum_DistinguishesTexts: the resume offset is only safe
// because it is bound to the exact SQL that produced it. Two texts sharing a
// sum would let a resume skip the wrong statements.
func TestMigrationSQLSum_DistinguishesTexts(t *testing.T) {
	a := migrationSQLSum("ALTER TABLE t ADD COLUMN a INT;")
	b := migrationSQLSum("ALTER TABLE t ADD COLUMN b INT;")
	if a == b {
		t.Fatal("two different migration texts produced the same sum")
	}
	if a != migrationSQLSum("ALTER TABLE t ADD COLUMN a INT;") {
		t.Fatal("the sum is not stable for identical text")
	}
}

// TestPreflight_FailsOpenWhenTheCatalogueIsUnreadable pins the deliberate
// fail-open. A nil *sql.DB makes the catalogue query fail; the pre-flight must
// wave the migration through rather than refusing it, because blocking every
// plugin migration on an information_schema hiccup would be a new outage mode
// strictly worse than the partial-application bug this check mitigates.
func TestPreflight_FailsOpenWhenTheCatalogueIsUnreadable(t *testing.T) {
	err := preflightPluginMigration(context.Background(), nil, "someplugin", 1,
		[]string{"RENAME TABLE definitely_absent TO something"})
	if err != nil {
		t.Fatalf("pre-flight refused a migration when it could not read the schema: %v", err)
	}
}
