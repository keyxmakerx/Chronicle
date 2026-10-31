package campaigns

import (
	"context"
	"testing"
)

// TestGroupRowsFollowMembership_Integration pins that a group row exists only
// while its user belongs to the group's campaign. Page and tag sharing reads
// group rows without checking membership, so a row that outlived a removal
// kept the pages shared with that group open to the removed user.
//
// Skipped under -short; uses the same database setup as TestNavPins_Integration.
func TestGroupRowsFollowMembership_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)
	groups := NewGroupRepository(db)

	owner, player, outsider, campaignID := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	for _, u := range []string{owner, player, outsider} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "groups-"+u+"@example.test", "Groups", "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by) VALUES (?, ?, ?, ?)`,
		campaignID, "Groups", "groups-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player')`,
		campaignID, owner, campaignID, player)
	group, err := groups.CreateGroup(ctx, campaignID, "Party", nil)
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	inGroup := func(userID string) bool {
		t.Helper()
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM campaign_group_members WHERE group_id = ? AND user_id = ?`,
			group.ID, userID).Scan(&n); err != nil {
			t.Fatalf("count group rows: %v", err)
		}
		return n > 0
	}

	if err := groups.AddGroupMember(ctx, group.ID, outsider); err == nil {
		t.Fatal("adding a user who is not a campaign member succeeded")
	}
	if inGroup(outsider) {
		t.Fatal("a non-member got a group row")
	}

	if err := groups.AddGroupMember(ctx, group.ID, player); err != nil {
		t.Fatalf("adding a member: %v", err)
	}
	if err := groups.AddGroupMember(ctx, group.ID, player); err != nil {
		t.Fatalf("adding a member twice should be a no-op: %v", err)
	}

	if err := repo.RemoveMember(ctx, campaignID, player); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if inGroup(player) {
		t.Fatal("a removed member kept their group row")
	}

	// A row left by a removal from before the fix is cleared by the pruner,
	// and a current member's row is not touched.
	navPinsExec(t, db, `INSERT INTO campaign_group_members (group_id, user_id) VALUES (?, ?), (?, ?)`,
		group.ID, player, group.ID, owner)
	if _, err := groups.PruneNonMemberGroupRows(ctx); err != nil {
		t.Fatalf("PruneNonMemberGroupRows: %v", err)
	}
	if inGroup(player) {
		t.Fatal("pruner left a former member's group row")
	}
	if !inGroup(owner) {
		t.Fatal("pruner removed a current member's group row")
	}
}
