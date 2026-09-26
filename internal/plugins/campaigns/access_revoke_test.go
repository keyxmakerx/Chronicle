package campaigns

// access_revoke_test.go pins the rest of the connection-revocation class
// (see dmgrant_revoke_test.go for the co-DM grant half): every path that
// lowers a user's campaign access must drop their live sockets too.

import (
	"context"
	"testing"
	"time"
)

// TestAcceptTransfer_DropsOldOwnersLiveConnection pins that accepting a
// transfer drops the old owner's live socket — their cached Role=Owner
// isn't rechecked, so they'd keep dm_only visibility otherwise.
func TestAcceptTransfer_DropsOldOwnersLiveConnection(t *testing.T) {
	repo := &mockCampaignRepo{
		findTransferByTokenFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{
				ID:         "transfer-1",
				CampaignID: "camp-1",
				FromUserID: "old-owner",
				ToUserID:   "new-owner",
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			}, nil
		},
		transferOwnershipFn: func(context.Context, string, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.AcceptTransfer(context.Background(), "valid-token", "new-owner"); err != nil {
		t.Fatalf("AcceptTransfer: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].campaignID != "camp-1" || revoker.revoked[0].userID != "old-owner" {
		t.Errorf("revoked = %v, want exactly [{camp-1 old-owner}] — accepting a transfer must drop "+
			"the demoted owner's live socket", revoker.revoked)
	}
}

// TestAcceptTransfer_UnwiredRevokerStillTransfers matches the fail-open
// convention: a nil (not-yet-wired) revoker must not block the transfer.
func TestAcceptTransfer_UnwiredRevokerStillTransfers(t *testing.T) {
	transferCalled := false
	repo := &mockCampaignRepo{
		findTransferByTokenFn: func(_ context.Context, _ string) (*OwnershipTransfer, error) {
			return &OwnershipTransfer{
				ID:         "transfer-1",
				CampaignID: "camp-1",
				FromUserID: "old-owner",
				ToUserID:   "new-owner",
				ExpiresAt:  time.Now().Add(24 * time.Hour),
			}, nil
		},
		transferOwnershipFn: func(context.Context, string, string, string) error {
			transferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	if err := svc.AcceptTransfer(context.Background(), "valid-token", "new-owner"); err != nil {
		t.Fatalf("unexpected error with no connection revoker wired: %v", err)
	}
	if !transferCalled {
		t.Error("TransferOwnership was not called even with no revoker wired")
	}
}

// TestForceTransferOwnership_DropsPreviousOwnersLiveConnection pins that the
// admin path looks up the previous owner before demoting (the repo call
// demotes by role, not by id) so it still knows who to revoke.
func TestForceTransferOwnership_DropsPreviousOwnersLiveConnection(t *testing.T) {
	repo := &mockCampaignRepo{
		findOwnerMemberFn: func(_ context.Context, _ string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: "old-owner", Role: RoleOwner}, nil
		},
		forceTransferOwnershipFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.ForceTransferOwnership(context.Background(), "camp-1", "admin-1"); err != nil {
		t.Fatalf("ForceTransferOwnership: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].campaignID != "camp-1" || revoker.revoked[0].userID != "old-owner" {
		t.Errorf("revoked = %v, want exactly [{camp-1 old-owner}] — force-transferring ownership "+
			"must drop the demoted owner's live socket", revoker.revoked)
	}
}

// TestForceTransferOwnership_NoPriorOwner_DoesNotRevoke keeps the fix
// scoped: a campaign with no current owner row (shouldn't normally happen,
// but FindOwnerMember can 404) has nobody to drop.
func TestForceTransferOwnership_NoPriorOwner_DoesNotRevoke(t *testing.T) {
	repo := &mockCampaignRepo{
		forceTransferOwnershipFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.ForceTransferOwnership(context.Background(), "camp-1", "admin-1"); err != nil {
		t.Fatalf("ForceTransferOwnership: %v", err)
	}
	if len(revoker.revoked) != 0 {
		t.Errorf("revoked = %v, want none — FindOwnerMember found nobody to demote", revoker.revoked)
	}
}

// TestForceTransferOwnership_UnwiredRevokerStillTransfers matches the
// fail-open convention: no revoker wired must not block the force-transfer.
func TestForceTransferOwnership_UnwiredRevokerStillTransfers(t *testing.T) {
	transferCalled := false
	repo := &mockCampaignRepo{
		findOwnerMemberFn: func(_ context.Context, _ string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: "old-owner", Role: RoleOwner}, nil
		},
		forceTransferOwnershipFn: func(context.Context, string, string) error {
			transferCalled = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	if err := svc.ForceTransferOwnership(context.Background(), "camp-1", "admin-1"); err != nil {
		t.Fatalf("unexpected error with no connection revoker wired: %v", err)
	}
	if !transferCalled {
		t.Error("ForceTransferOwnership was not called even with no revoker wired")
	}
}

// TestUpdateMemberRole_DowngradeDropsLiveConnection pins that lowering a
// member's role drops their live socket: their cached Role still governs
// the hub's gates until they reconnect.
func TestUpdateMemberRole_DowngradeDropsLiveConnection(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{UserID: userID, Role: RoleScribe}, nil
		},
		updateMemberRoleFn: func(context.Context, string, string, Role) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.UpdateMemberRole(context.Background(), "camp-1", "u-demoted", RolePlayer); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].campaignID != "camp-1" || revoker.revoked[0].userID != "u-demoted" {
		t.Errorf("revoked = %v, want exactly [{camp-1 u-demoted}] — lowering a member's role "+
			"must drop their live socket", revoker.revoked)
	}
}

// TestUpdateMemberRole_UpgradeDoesNotRevoke is the over-correction guard: a
// user whose access went up needs nothing dropped.
func TestUpdateMemberRole_UpgradeDoesNotRevoke(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{UserID: userID, Role: RolePlayer}, nil
		},
		updateMemberRoleFn: func(context.Context, string, string, Role) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.UpdateMemberRole(context.Background(), "camp-1", "u-promoted", RoleScribe); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if len(revoker.revoked) != 0 {
		t.Errorf("revoked = %v, want none — promoting a member is not an access loss", revoker.revoked)
	}
}

// TestAdminAddMember_RoleDowngradeDropsLiveConnection pins the admin path,
// which updates an existing member's role directly rather than through
// UpdateMemberRole — it needs the same downgrade check.
func TestAdminAddMember_RoleDowngradeDropsLiveConnection(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{UserID: userID, Role: RoleScribe}, nil
		},
		updateMemberRoleFn: func(context.Context, string, string, Role) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.AdminAddMember(context.Background(), "camp-1", "u-demoted", RolePlayer); err != nil {
		t.Fatalf("AdminAddMember: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].userID != "u-demoted" {
		t.Errorf("revoked = %v, want exactly one entry for u-demoted — an admin lowering a "+
			"member's role must drop their live socket too", revoker.revoked)
	}
}

// TestAdminAddMember_RoleUpgradeDoesNotRevoke is the admin-path
// over-correction guard.
func TestAdminAddMember_RoleUpgradeDoesNotRevoke(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{UserID: userID, Role: RolePlayer}, nil
		},
		updateMemberRoleFn: func(context.Context, string, string, Role) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.AdminAddMember(context.Background(), "camp-1", "u-promoted", RoleScribe); err != nil {
		t.Fatalf("AdminAddMember: %v", err)
	}
	if len(revoker.revoked) != 0 {
		t.Errorf("revoked = %v, want none — promoting a member is not an access loss", revoker.revoked)
	}
}

// TestDelete_DropsEveryLiveConnectionInCampaign pins that deleting a
// campaign closes every socket in it, regardless of user or source — once
// the delete commits there is no member, grant or key left to hold one open.
func TestDelete_DropsEveryLiveConnectionInCampaign(t *testing.T) {
	repo := &mockCampaignRepo{
		deleteFn: func(context.Context, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.Delete(context.Background(), "camp-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(revoker.revokedCampaigns) != 1 || revoker.revokedCampaigns[0] != "camp-1" {
		t.Errorf("revokedCampaigns = %v, want [camp-1]", revoker.revokedCampaigns)
	}
}

// TestDelete_UnwiredRevokerStillDeletes matches the fail-open convention
// used elsewhere: a nil revoker must not block the delete itself.
func TestDelete_UnwiredRevokerStillDeletes(t *testing.T) {
	deleted := false
	repo := &mockCampaignRepo{
		deleteFn: func(context.Context, string) error {
			deleted = true
			return nil
		},
	}
	svc := newTestCampaignService(repo, &mockUserFinder{})

	if err := svc.Delete(context.Background(), "camp-1"); err != nil {
		t.Fatalf("unexpected error with no connection revoker wired: %v", err)
	}
	if !deleted {
		t.Error("campaign was not deleted even though the revoker was never wired")
	}
}
