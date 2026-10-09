package syncapi

import (
	"context"
	"testing"
)

// A key stops authenticating while its creator's account is disabled, and
// works again once the account is re-enabled. Runs against the same scratch
// schema as the sync stats test; skips without a test DB.
func TestFindKeyByPrefixIntegration_DisabledOwner(t *testing.T) {
	db := newSyncStatsScratchDB(t)
	ctx := context.Background()
	uid, cid := "u-dis-00000000000000000000000000001", "c-dis-00000000000000000000000000001"
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (id, email, display_name, password_hash) VALUES (?,?,?,?)`, uid, "dis@example.test", "Ana", "x")
	mustExec(`INSERT INTO campaigns (id, name, slug, created_by) VALUES (?,?,?,?)`, cid, "Disabled", "disabled", uid)
	mustExec(`INSERT INTO api_keys (key_hash, key_prefix, user_id, campaign_id, name, permissions) VALUES (?,?,?,?,?,?)`,
		"h1", "chr_dddd", uid, cid, "Foundry", `["read","write","sync"]`)

	repo := NewSyncAPIRepository(db)
	tests := []struct {
		name     string
		disabled bool
		wantKey  bool
	}{
		{"enabled owner finds the key", false, true},
		{"disabled owner's key is not found", true, false},
		{"re-enabled owner finds it again", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustExec(`UPDATE users SET is_disabled = ? WHERE id = ?`, tt.disabled, uid)
			key, err := repo.FindKeyByPrefix(ctx, "chr_dddd")
			if tt.wantKey && (err != nil || key == nil) {
				t.Fatalf("want key, got %v, %v", key, err)
			}
			if !tt.wantKey && err == nil && key != nil {
				t.Fatalf("want no key, got id %d", key.ID)
			}
		})
	}
}
