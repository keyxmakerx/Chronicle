package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// TestCustomizeDraft_Integration runs the draft service against a real MariaDB
// with every core migration applied (000055 creates the table): optimistic
// versions, one row per owner and campaign, and cascade on campaign delete.
//
// Skipped under -short or with no database; see TestNavPins_Integration for
// how the DSN is found. A migration that fails to apply FAILS the test.
func TestCustomizeDraft_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()

	owner, other := navPinsUUID(t), navPinsUUID(t)
	campaignID := navPinsUUID(t)
	for _, u := range []string{owner, other} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "draft-"+u+"@example.test", "Draft", "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Draft", "draft-"+campaignID[:8], owner)

	svc := NewCustomizeDraftService(NewCustomizeDraftRepository(db))
	keys := []string{"look", "categories"}

	if d, err := svc.Get(ctx, campaignID, owner); err != nil || d != nil {
		t.Fatalf("no draft yet: %v, %v", d, err)
	}
	v, err := svc.Save(ctx, campaignID, owner, 0, json.RawMessage(`{"look":{"a":1}}`), keys)
	if err != nil || v != 1 {
		t.Fatalf("first save = %d, %v", v, err)
	}
	// A second tab that never saw the draft loses the insert race.
	if _, err := svc.Save(ctx, campaignID, owner, 0, json.RawMessage(`{"look":{"a":9}}`), keys); apperror.SafeCode(err) != http.StatusConflict {
		t.Fatalf("second insert err = %v, want conflict", err)
	}
	if v, err = svc.Save(ctx, campaignID, owner, 1, json.RawMessage(`{"look":{"a":2},"categories":{"items":{}}}`), keys); err != nil || v != 2 {
		t.Fatalf("update = %d, %v", v, err)
	}
	if _, err := svc.Save(ctx, campaignID, owner, 1, json.RawMessage(`{"look":{"a":3}}`), keys); apperror.SafeCode(err) != http.StatusConflict {
		t.Fatalf("stale update err = %v, want conflict", err)
	}
	d, err := svc.Get(ctx, campaignID, owner)
	if err != nil || d == nil || d.Version != 2 || string(d.Draft) != `{"look":{"a":2},"categories":{"items":{}}}` {
		t.Fatalf("stored = %+v, %v", d, err)
	}
	if d.UpdatedAt.IsZero() {
		t.Error("updated_at was not read back")
	}
	if got, _ := svc.Get(ctx, campaignID, other); got != nil {
		t.Fatalf("another user sees the draft: %+v", got)
	}

	if err := svc.Discard(ctx, campaignID, owner, 1); apperror.SafeCode(err) != http.StatusConflict {
		t.Fatalf("stale discard err = %v, want conflict", err)
	}
	// An empty object removes the row and reports version 0.
	if v, err = svc.Save(ctx, campaignID, owner, 2, json.RawMessage(`{}`), keys); err != nil || v != 0 {
		t.Fatalf("clearing = %d, %v", v, err)
	}
	if d, _ := svc.Get(ctx, campaignID, owner); d != nil {
		t.Fatalf("draft survived clearing: %+v", d)
	}

	// Deleting the campaign takes its drafts with it.
	if _, err := svc.Save(ctx, campaignID, owner, 0, json.RawMessage(`{"look":{}}`), keys); err != nil {
		t.Fatal(err)
	}
	navPinsExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM customize_drafts WHERE campaign_id = ?`, campaignID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("drafts after campaign delete = %d, %v", n, err)
	}
}
