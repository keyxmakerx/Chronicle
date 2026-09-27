package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// TestUserRepository_UpdateAvatarPath_Integration exercises the idempotent
// clear against a real MariaDB, where the fix matters: MariaDB reports zero
// rows affected when an UPDATE's new value equals the one already stored,
// which a mock repository can't reproduce. Clearing an avatar that is
// already NULL must stay a success (not apperror.NotFound), so double-
// clicking "Remove" on the account page doesn't surface a 404.
//
// Skipped under `-short`. DSN comes from CHRONICLE_TEST_DB_DSN, else the
// DB_* env vars, else the dev default matching the Makefile's DATABASE_URL.
// If no DB answers, the test SKIPS rather than fails.
//
// Run with: `make docker-up && make migrate-up && make test-int`, or
// `make test-int-local`.
func TestUserRepository_UpdateAvatarPath_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openAuthTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewUserRepository(db)

	userID := authTestUUID(t)
	mustAuthExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
		userID, "avatar-int-"+userID+"@example.test", "Avatar Int Test", "x")
	defer mustAuthExec(t, db, `DELETE FROM users WHERE id = ?`, userID)

	// A fresh user's avatar_path starts NULL. Clearing it (no-op at the DB
	// level -- NULL to NULL) must succeed, not report "user not found".
	if err := repo.UpdateAvatarPath(ctx, userID, nil); err != nil {
		t.Fatalf("clearing an already-NULL avatar_path: %v", err)
	}

	// Set a real value, then clear it twice. The second clear is the case
	// that used to surface as a 404: RowsAffected is 0 both times a value
	// doesn't change, whether that's "user not found" or "already this value".
	mediaID := "media-" + userID
	if err := repo.UpdateAvatarPath(ctx, userID, &mediaID); err != nil {
		t.Fatalf("setting avatar_path: %v", err)
	}
	if err := repo.UpdateAvatarPath(ctx, userID, nil); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if err := repo.UpdateAvatarPath(ctx, userID, nil); err != nil {
		t.Fatalf("second clear of an already-cleared avatar_path must succeed (idempotent), got: %v", err)
	}

	// Clearing a genuinely nonexistent user must still report NotFound.
	if err := repo.UpdateAvatarPath(ctx, authTestUUID(t), nil); err == nil {
		t.Error("clearing avatar_path for a nonexistent user must return an error")
	}
}

// TestUserRepository_ClearAvatarPathIfMatches_Integration exercises the
// reconciler's conditional clear against a real MariaDB: it must clear only
// when the stored value still equals the expected one, and must never
// clobber a row a concurrent write already changed.
func TestUserRepository_ClearAvatarPathIfMatches_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}

	db := openAuthTestDB(t)
	defer db.Close()

	ctx := context.Background()
	repo := NewUserRepository(db)

	userID := authTestUUID(t)
	legacyPath := "/uploads/avatars/legacy.jpg"
	mustAuthExec(t, db, `INSERT INTO users (id, email, display_name, password_hash, avatar_path) VALUES (?, ?, ?, ?, ?)`,
		userID, "clearifmatch-int-"+userID+"@example.test", "Clear If Match Int Test", "x", legacyPath)
	defer mustAuthExec(t, db, `DELETE FROM users WHERE id = ?`, userID)

	// A stale expected value (as if a concurrent real upload had already
	// overwritten the row) must not clear it.
	if didClear, err := repo.ClearAvatarPathIfMatches(ctx, userID, "not-the-current-value"); err != nil || didClear {
		t.Fatalf("stale expected value must not clear the row: didClear=%v err=%v", didClear, err)
	}

	// The real current value clears it.
	if didClear, err := repo.ClearAvatarPathIfMatches(ctx, userID, legacyPath); err != nil || !didClear {
		t.Fatalf("matching expected value should clear the row: didClear=%v err=%v", didClear, err)
	}

	// A second attempt with the same (now-stale) expected value is a no-op,
	// not an error.
	if didClear, err := repo.ClearAvatarPathIfMatches(ctx, userID, legacyPath); err != nil || didClear {
		t.Fatalf("clearing an already-cleared row with a stale expected value must be a no-op: didClear=%v err=%v", didClear, err)
	}
}

// --- helpers (mirrors the entities package's own integration-test helpers) ---

func openAuthTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CHRONICLE_TEST_DB_DSN")
	if dsn == "" {
		cfg := mysql.NewConfig()
		cfg.User = authGetenvDefault("DB_USER", "chronicle")
		cfg.Passwd = authGetenvDefault("DB_PASSWORD", "chronicle")
		cfg.Net = "tcp"
		cfg.Addr = authGetenvDefault("DB_HOST", "127.0.0.1:3306")
		cfg.DBName = authGetenvDefault("DB_NAME", "chronicle")
		cfg.ParseTime = true
		dsn = cfg.FormatDSN()
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Skipf("no test DB (sql.Open: %v)", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Skipf("no test DB reachable (ping: %v) — run `make docker-up && make migrate-up`", err)
	}
	return db
}

func authGetenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustAuthExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func authTestUUID(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
