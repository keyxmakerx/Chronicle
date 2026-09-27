// icon_reconcile_test.go runs ReconcileIconColumns against a real MariaDB
// scratch schema. Skips (never fails) when no server is reachable.
//
//	tools/start-test-db.sh
//	CHRONICLE_TEST_DB_DSN='root@tcp(127.0.0.1:13306)/' go test ./internal/database/ -run Icon
package database

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// newIconScratchDB creates an empty throwaway schema and drops it on cleanup.
func newIconScratchDB(t *testing.T) *sql.DB {
	t.Helper()
	raw := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if raw == "" {
		t.Skip("set CHRONICLE_TEST_DB_DSN (make test-db-up) to run the icon reconcile tests")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil {
		t.Skipf("CHRONICLE_TEST_DB_DSN is not a valid DSN: %v", err)
	}
	serverCfg := *cfg
	serverCfg.DBName = ""
	admin, err := sql.Open("mysql", serverCfg.FormatDSN())
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("no test DB server reachable at %s: %v — run `make test-db-up`", cfg.Addr, err)
	}
	name := fmt.Sprintf("chronicle_icons_%06d", rand.Intn(1000000)) //nolint:gosec // test schema name
	if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Skipf("cannot create scratch schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE IF EXISTS `" + name + "`") })
	scratch := *cfg
	scratch.DBName = name
	db, err := sql.Open("mysql", scratch.FormatDSN())
	if err != nil {
		t.Fatalf("opening scratch schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustExecIcon(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.Exec(q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

// iconsByID reads a table's id→icon map; NULL reads as "<NULL>".
func iconsByID(t *testing.T, db *sql.DB, table string) map[string]string {
	t.Helper()
	rows, err := db.Query("SELECT id, icon FROM " + table)
	if err != nil {
		t.Fatalf("select %s: %v", table, err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id string
		var icon sql.NullString
		if err := rows.Scan(&id, &icon); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if icon.Valid {
			out[id] = icon.String
		} else {
			out[id] = "<NULL>"
		}
	}
	return out
}

func TestReconcileIconColumns_Integration(t *testing.T) {
	db := newIconScratchDB(t)
	ctx := context.Background()

	// One INT-keyed nullable column and one VARCHAR-keyed NOT NULL column,
	// matching the two shapes the real icon tables have.
	mustExecIcon(t, db, `CREATE TABLE int_icons (id INT AUTO_INCREMENT PRIMARY KEY, icon VARCHAR(100) NULL)
		CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`)
	mustExecIcon(t, db, `CREATE TABLE str_icons (id VARCHAR(36) PRIMARY KEY, icon VARCHAR(100) NOT NULL DEFAULT 'fa-x')
		CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci`)

	intRows := []struct {
		icon *string
		want string
	}{
		{iconPtr("fa-book"), "fa-book"},
		{iconPtr("fa-dice-d20"), "fa-dice-d20"},
		{nil, "<NULL>"},
		{iconPtr(`fa-x" data-y="z`), "fa-circle"},
		{iconPtr("fa-<b>"), "fa-circle"},
		{iconPtr("FA-BOOK"), "fa-circle"},
		{iconPtr("fa-book extra"), "fa-circle"},
		{iconPtr("fa-"), "fa-circle"},
		{iconPtr(""), "fa-circle"},
		{iconPtr("fa-" + strings.Repeat("a", 60)), "fa-circle"},
	}
	for _, r := range intRows {
		mustExecIcon(t, db, `INSERT INTO int_icons (icon) VALUES (?)`, r.icon)
	}
	strRows := map[string][2]string{
		"a": {"fa-map-pin", "fa-map-pin"},
		"b": {"fa-solid fa-map-pin", "fa-map-pin"},
		"c": {"fa-map-pin'", "fa-map-pin"},
	}
	for id, r := range strRows {
		mustExecIcon(t, db, `INSERT INTO str_icons (id, icon) VALUES (?, ?)`, id, r[0])
	}

	cols := []IconColumn{
		{Table: "int_icons", Column: "icon", Default: "fa-circle"},
		{Table: "str_icons", Column: "icon", Default: "fa-map-pin"},
		{Table: "not_migrated", Column: "icon", Default: "fa-circle"}, // skipped, not an error
	}

	n, err := ReconcileIconColumns(ctx, db, cols)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if want := 7 + 2; n != want {
		t.Errorf("first run changed %d rows, want %d", n, want)
	}

	gotInt := iconsByID(t, db, "int_icons")
	for i, r := range intRows {
		id := fmt.Sprint(i + 1)
		if gotInt[id] != r.want {
			t.Errorf("int_icons id %s = %q, want %q", id, gotInt[id], r.want)
		}
	}
	gotStr := iconsByID(t, db, "str_icons")
	for id, r := range strRows {
		if gotStr[id] != r[1] {
			t.Errorf("str_icons id %s = %q, want %q", id, gotStr[id], r[1])
		}
	}

	// Second run: nothing left to change, and nothing does change.
	n, err = ReconcileIconColumns(ctx, db, cols)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if n != 0 {
		t.Errorf("second run changed %d rows, want 0", n)
	}
	if again := iconsByID(t, db, "int_icons"); fmt.Sprint(again) != fmt.Sprint(gotInt) {
		t.Errorf("second run altered int_icons: %v -> %v", gotInt, again)
	}
	if again := iconsByID(t, db, "str_icons"); fmt.Sprint(again) != fmt.Sprint(gotStr) {
		t.Errorf("second run altered str_icons: %v -> %v", gotStr, again)
	}
}

func TestReconcileIconColumns_RefusesBadConfig_Integration(t *testing.T) {
	db := newIconScratchDB(t)
	ctx := context.Background()
	mustExecIcon(t, db, `CREATE TABLE cfg_icons (id INT AUTO_INCREMENT PRIMARY KEY, icon VARCHAR(100) NULL)`)
	mustExecIcon(t, db, `INSERT INTO cfg_icons (icon) VALUES ('bad value')`)

	tests := []struct {
		name string
		col  IconColumn
	}{
		{"invalid default", IconColumn{Table: "cfg_icons", Column: "icon", Default: "not-an-icon"}},
		{"unsafe table", IconColumn{Table: "cfg_icons; DROP TABLE x", Column: "icon", Default: "fa-circle"}},
		{"unsafe column", IconColumn{Table: "cfg_icons", Column: "icon`", Default: "fa-circle"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ReconcileIconColumns(ctx, db, []IconColumn{tt.col}); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
	if got := iconsByID(t, db, "cfg_icons")["1"]; got != "bad value" {
		t.Errorf("a refused config must not write; icon = %q", got)
	}
}

func iconPtr(s string) *string { return &s }
