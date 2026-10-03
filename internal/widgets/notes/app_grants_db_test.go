package notes

// Real-MariaDB test for notes_app_grants: the SQL behind issue, look-up,
// list, touch and revoke. Skips without CHRONICLE_TEST_DB_DSN, like the
// other row-level tests in this package.

import (
	"context"
	"testing"
	"time"
)

func TestDB_AppGrantRoundTrip(t *testing.T) {
	db := newNotesScratchDB(t)
	campaignID, users := seedNotesCampaign(t, db, "ana", "ben")
	ctx := context.Background()
	svc := NewAppGrantService(NewAppGrantRepository(db))

	token, g, err := svc.Issue(ctx, campaignID, users["ana"], "https://foundry.example")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	got, err := svc.Authenticate(ctx, token)
	if err != nil || got.ID != g.ID || got.UserID != users["ana"] || got.Origin != "https://foundry.example" {
		t.Fatalf("authenticate: %+v, %v", got, err)
	}
	if got.LastUsedAt != nil {
		t.Fatal("the first look-up returned a last-used time before any use")
	}
	again, _ := svc.Authenticate(ctx, token)
	if again.LastUsedAt == nil || time.Since(*again.LastUsedAt) > time.Hour {
		t.Fatalf("last use was not recorded: %+v", again.LastUsedAt)
	}

	if l, _ := svc.List(ctx, campaignID, users["ben"]); len(l) != 0 {
		t.Fatalf("ben sees ana's grants: %+v", l)
	}
	if err := svc.Revoke(ctx, campaignID, users["ben"], g.ID); err == nil {
		t.Fatal("ben revoked ana's grant")
	}
	if err := svc.Revoke(ctx, campaignID, users["ana"], g.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.Authenticate(ctx, token); err == nil {
		t.Fatal("revoked grant still authenticates")
	}
	if l, _ := svc.List(ctx, campaignID, users["ana"]); len(l) != 0 {
		t.Fatalf("revoked grant still listed: %+v", l)
	}
}
