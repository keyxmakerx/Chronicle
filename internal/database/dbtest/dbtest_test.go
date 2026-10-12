package dbtest

import (
	"os"
	"path/filepath"
	"testing"
)

// The DSN handling is what every caller depends on and it needs no server, so
// it is the part pinned here; the schema lifecycle is exercised by the
// database-backed tests that use Open.
func TestServerConfig(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		env      map[string]string
		wantAddr string
		wantUser string
		wantDB   string
	}{
		{
			name:     "server-only DSN keeps no database",
			dsn:      "root@tcp(127.0.0.1:13306)/",
			wantAddr: "127.0.0.1:13306",
			wantUser: "root",
		},
		{
			name:     "DSN database name is preserved for the caller to override",
			dsn:      "u:p@tcp(db:3306)/named",
			wantAddr: "db:3306",
			wantUser: "u",
			wantDB:   "named",
		},
		{
			name:     "unset DSN falls back to the DB_* variables",
			env:      map[string]string{"DB_USER": "alice", "DB_HOST": "10.0.0.5:3307"},
			wantAddr: "10.0.0.5:3307",
			wantUser: "alice",
		},
		{
			name:     "unset DSN and variables use the dev defaults",
			wantAddr: "127.0.0.1:3306",
			wantUser: "chronicle",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CHRONICLE_TEST_DB_DSN", tc.dsn)
			for _, k := range []string{"DB_USER", "DB_PASSWORD", "DB_HOST"} {
				t.Setenv(k, tc.env[k])
			}
			cfg := serverConfig(t, false)
			if cfg.Addr != tc.wantAddr || cfg.User != tc.wantUser || cfg.DBName != tc.wantDB {
				t.Errorf("got addr=%q user=%q db=%q, want addr=%q user=%q db=%q",
					cfg.Addr, cfg.User, cfg.DBName, tc.wantAddr, tc.wantUser, tc.wantDB)
			}
			if !cfg.ParseTime {
				t.Error("ParseTime must be on so DATETIME columns scan into time.Time")
			}
		})
	}
}

func TestRepoRootHoldsGoMod(t *testing.T) {
	if _, err := os.Stat(filepath.Join(RepoRoot(t), "go.mod")); err != nil {
		t.Fatalf("RepoRoot is not the module root: %v", err)
	}
}
