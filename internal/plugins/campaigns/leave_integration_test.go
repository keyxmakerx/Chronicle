package campaigns

import (
	"context"
	"slices"
	"testing"
)

// Leaving on a real database: the membership goes, the person is not marked
// removed, the join link is the one it was and still brings them back, and
// the owner's own role and the campaign are untouched. Skipped under -short;
// same database setup as TestNavPins_Integration.
func TestLeaveCampaign_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test requires a database; skipped under -short")
	}
	db := openNavPinsTestDB(t)
	ctx := context.Background()
	repo := NewCampaignRepository(db)
	svc := NewCampaignService(repo, &mockUserFinder{}, nil, nil, "http://localhost")

	owner, tam, campaignID := navPinsUUID(t), navPinsUUID(t), navPinsUUID(t)
	for _, u := range []string{owner, tam} {
		navPinsExec(t, db, `INSERT INTO users (id, email, display_name, password_hash) VALUES (?, ?, ?, ?)`,
			u, "leave-"+u+"@example.test", "Leave Int", "x")
	}
	navPinsExec(t, db, `INSERT INTO campaigns (id, name, slug, created_by, join_code) VALUES (?, ?, ?, ?, 'abc123def456')`,
		campaignID, "Leave Int", "leave-"+campaignID[:8], owner)
	navPinsExec(t, db, `INSERT INTO campaign_members (campaign_id, user_id, role) VALUES (?, ?, 'owner'), (?, ?, 'player')`,
		campaignID, owner, campaignID, tam)
	defer func() {
		navPinsExec(t, db, `DELETE FROM campaigns WHERE id = ?`, campaignID)
		navPinsExec(t, db, `DELETE FROM users WHERE id IN (?, ?)`, owner, tam)
	}()

	owned, err := svc.OwnedMemberships(ctx, owner)
	if err != nil || !slices.Contains(owned, campaignID) {
		t.Fatalf("owner's campaigns = %v, %v", owned, err)
	}
	if owned, _ := svc.OwnedMemberships(ctx, tam); slices.Contains(owned, campaignID) {
		t.Fatal("a player is listed as the owner")
	}

	assertAppError(t, svc.LeaveCampaign(ctx, campaignID, owner), 400)

	if err := svc.LeaveCampaign(ctx, campaignID, tam); err != nil {
		t.Fatalf("LeaveCampaign: %v", err)
	}
	if _, err := repo.FindMember(ctx, campaignID, tam); err == nil {
		t.Fatal("tam is still a member")
	}
	c, err := repo.FindByID(ctx, campaignID)
	if err != nil {
		t.Fatal(err)
	}
	if c.JoinCode == nil || *c.JoinCode != "abc123def456" {
		t.Errorf("join code = %v, want it unchanged", c.JoinCode)
	}
	if slices.Contains(c.ParseSettings().RemovedMemberIDs, tam) {
		t.Error("tam was marked removed")
	}
	if _, err := repo.FindMember(ctx, campaignID, owner); err != nil {
		t.Errorf("owner lost their seat: %v", err)
	}
	assertAppError(t, svc.LeaveCampaign(ctx, campaignID, tam), 404)

	if _, joined, err := svc.JoinByCode(ctx, "abc123def456", tam); err != nil || !joined {
		t.Fatalf("rejoining with the link: joined=%v err=%v", joined, err)
	}
}
