package campaigns

// dmgrant_revoke_test.go pins that removing a member clears their co-DM
// grant (STORED state) and that the grant is not honoured for a non-member
// (HONOURED state). On a public campaign, either gap alone lets a removed
// co-DM keep owner-level visibility via AllowPublicCampaignAccess +
// VisibilityRole(); private campaigns are unaffected since
// RequireCampaignAccess rejects a non-member first. Both halves are needed:
// clearing on removal does nothing for campaigns already carrying a stale
// id, and gating at resolve time alone leaves wrong stored data.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
	"github.com/keyxmakerx/chronicle/internal/plugins/auth"
)

// campaignWithGrants builds a campaign whose settings carry the given grant ids.
func campaignWithGrants(t *testing.T, public bool, ids ...string) *Campaign {
	t.Helper()
	b, err := json.Marshal(CampaignSettings{DmGrantIDs: ids})
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	return &Campaign{
		ID: "camp-1", Name: "Test", Slug: "test",
		Settings: string(b), SidebarConfig: "{}", IsPublic: public,
	}
}

// TestRemoveMember_ClearsDmGrant is half one: the stored state.
//
// Without this, the id lingers in settings.dm_grant_ids forever and every
// future reader of that campaign inherits the grant.
func TestRemoveMember_ClearsDmGrant(t *testing.T) {
	var wrote string
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
		},
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "u-gone", "u-stays"), nil
		},
		updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
			wrote = settingsJSON
			return nil
		},
	}
	svc := &campaignService{repo: repo}

	if err := svc.RemoveMember(context.Background(), "camp-1", "u-gone"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	if wrote == "" {
		t.Fatal("settings were never rewritten — the removed member's dm grant is still stored, " +
			"so the next reader of this campaign still treats them as a co-DM")
	}
	var got CampaignSettings
	if err := json.Unmarshal([]byte(wrote), &got); err != nil {
		t.Fatalf("unmarshal written settings: %v", err)
	}
	for _, id := range got.DmGrantIDs {
		if id == "u-gone" {
			t.Error("removed member is still in dm_grant_ids")
		}
	}
	if len(got.DmGrantIDs) != 1 || got.DmGrantIDs[0] != "u-stays" {
		t.Errorf("dm_grant_ids = %v, want exactly [u-stays] — the fix must not "+
			"revoke grants belonging to members who were not removed", got.DmGrantIDs)
	}
}

// TestRemoveMember_WithoutGrant_DoesNotRewriteSettings keeps the fix cheap:
// removing an ordinary member must not churn the settings row.
func TestRemoveMember_WithoutGrant_DoesNotRewriteSettings(t *testing.T) {
	rewritten := false
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RolePlayer}, nil
		},
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "someone-else"), nil
		},
		updateSettingsFn: func(context.Context, string, string) error {
			rewritten = true
			return nil
		},
	}
	svc := &campaignService{repo: repo}

	if err := svc.RemoveMember(context.Background(), "camp-1", "u-plain"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if rewritten {
		t.Error("settings rewritten for a member who held no grant — unnecessary write")
	}
}

// TestPublicAccess_GrantNotHonouredForNonMember is half two: the resolve-time
// gate. This is the guard that actually closes the leak for campaigns whose
// stored data is ALREADY corrupt.
func TestPublicAccess_GrantNotHonouredForNonMember(t *testing.T) {
	svc := &stubPublicSvc{
		campaign:  campaignWithGrants(t, true, "u-removed"),
		memberErr: apperror.NewNotFound("not a member"),
	}
	cc, _ := runPublicAccess(t, svc, &auth.Session{UserID: "u-removed"})
	if cc == nil {
		t.Fatal("no campaign context resolved")
	}

	if cc.IsMember {
		t.Fatal("test setup wrong: the user should not resolve as a member")
	}
	if cc.IsDmGranted {
		t.Error("a non-member still carries IsDmGranted — a removed co-DM keeps GM sight " +
			"on a public campaign")
	}
	if got := cc.VisibilityRole(); got == int(RoleOwner) {
		t.Errorf("VisibilityRole = %d (owner) for a non-member — every dm_only event, "+
			"note and marker in this campaign is visible to someone who was removed from it", got)
	}
}

// TestPublicAccess_GrantHonouredForMember is the over-correction guard. The fix
// must not cost a legitimate co-DM their sight.
func TestPublicAccess_GrantHonouredForMember(t *testing.T) {
	svc := &stubPublicSvc{
		campaign: campaignWithGrants(t, true, "u-codm"),
		member:   &CampaignMember{CampaignID: "camp-1", UserID: "u-codm", Role: RolePlayer},
	}
	cc, _ := runPublicAccess(t, svc, &auth.Session{UserID: "u-codm"})
	if cc == nil {
		t.Fatal("no campaign context resolved")
	}
	if !cc.IsDmGranted {
		t.Fatal("a dm-granted MEMBER lost their grant — the fix over-corrected")
	}
	if got := cc.VisibilityRole(); got != int(RoleOwner) {
		t.Errorf("VisibilityRole = %d, want owner-equivalent for a dm-granted member", got)
	}
}

// TestUpdateDmGrants_RejectsNonMembers pins the write path. Once membership
// gates the grant, an unvalidated write is the remaining way to get a junk id
// into the list.
func TestUpdateDmGrants_RejectsNonMembers(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true), nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			if userID == "u-member" {
				return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
			}
			return nil, apperror.NewNotFound("not a member")
		},
		updateSettingsFn: func(context.Context, string, string) error { return nil },
	}
	svc := &campaignService{repo: repo}

	err := svc.UpdateDmGrants(context.Background(), "camp-1", []string{"u-member", "u-outsider"})
	if err == nil {
		t.Fatal("granting co-DM to a non-member succeeded; the write path accepts any id")
	}
}

// TestUpdateDmGrants_Dedups keeps the stored list honest.
func TestUpdateDmGrants_Dedups(t *testing.T) {
	var wrote string
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true), nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
		},
		updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
			wrote = settingsJSON
			return nil
		},
	}
	svc := &campaignService{repo: repo}

	if err := svc.UpdateDmGrants(context.Background(), "camp-1", []string{"u-a", "u-a", "u-b"}); err != nil {
		t.Fatalf("UpdateDmGrants: %v", err)
	}
	var got CampaignSettings
	if err := json.Unmarshal([]byte(wrote), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.DmGrantIDs) != 2 {
		t.Errorf("dm_grant_ids = %v, want 2 entries after dedup", got.DmGrantIDs)
	}
}

// mockConnRevoker records RevokeUser calls for assertions.
type mockConnRevoker struct {
	revoked          []struct{ campaignID, userID string }
	revokedCampaigns []string
}

func (m *mockConnRevoker) RevokeUser(campaignID, userID string) {
	m.revoked = append(m.revoked, struct{ campaignID, userID string }{campaignID, userID})
}

func (m *mockConnRevoker) RevokeCampaign(campaignID string) {
	m.revokedCampaigns = append(m.revokedCampaigns, campaignID)
}

// TestRemoveMember_DropsLiveConnectionForRevokedGrant pins that clearing a
// removed member's co-DM grant also drops their live socket — IsDmGranted
// is cached at connect time and never rechecked otherwise.
func TestRemoveMember_DropsLiveConnectionForRevokedGrant(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
		},
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "u-gone"), nil
		},
		updateSettingsFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.RemoveMember(context.Background(), "camp-1", "u-gone"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].campaignID != "camp-1" || revoker.revoked[0].userID != "u-gone" {
		t.Errorf("revoked = %v, want exactly [{camp-1 u-gone}] — removing a member's dm grant "+
			"must drop their live socket", revoker.revoked)
	}
}

// TestRemoveMember_WithoutGrant_StillRevokesConnection pins that the drop
// is unconditional: a plain member still loses their socket, since cached
// Role (not just IsDmGranted) governs the hub's audience checks too.
func TestRemoveMember_WithoutGrant_StillRevokesConnection(t *testing.T) {
	repo := &mockCampaignRepo{
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RolePlayer}, nil
		},
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "someone-else"), nil
		},
		updateSettingsFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.RemoveMember(context.Background(), "camp-1", "u-plain"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if len(revoker.revoked) != 1 || revoker.revoked[0].campaignID != "camp-1" || revoker.revoked[0].userID != "u-plain" {
		t.Errorf("revoked = %v, want exactly [{camp-1 u-plain}] — removing a member must drop "+
			"their live socket even when they held no co-DM grant", revoker.revoked)
	}
}

// TestUpdateDmGrants_DropsLiveConnectionsForRemovedGrants pins the write
// path: an owner pulling someone from the co-DM list (without removing
// them as a member) must also drop their live socket.
func TestUpdateDmGrants_DropsLiveConnectionsForRemovedGrants(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "u-kept", "u-removed"), nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
		},
		updateSettingsFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.UpdateDmGrants(context.Background(), "camp-1", []string{"u-kept"}); err != nil {
		t.Fatalf("UpdateDmGrants: %v", err)
	}

	if len(revoker.revoked) != 1 || revoker.revoked[0].userID != "u-removed" {
		t.Errorf("revoked = %v, want exactly one entry for u-removed", revoker.revoked)
	}
}

// TestUpdateDmGrants_NoRemovals_DoesNotRevoke keeps the fix scoped: keeping
// or adding grants must not disconnect anyone.
func TestUpdateDmGrants_NoRemovals_DoesNotRevoke(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) {
			return campaignWithGrants(t, true, "u-kept"), nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			return &CampaignMember{CampaignID: "camp-1", UserID: userID, Role: RoleScribe}, nil
		},
		updateSettingsFn: func(context.Context, string, string) error { return nil },
	}
	revoker := &mockConnRevoker{}
	svc := &campaignService{repo: repo, connRevoker: revoker}

	if err := svc.UpdateDmGrants(context.Background(), "camp-1", []string{"u-kept", "u-new"}); err != nil {
		t.Fatalf("UpdateDmGrants: %v", err)
	}
	if len(revoker.revoked) != 0 {
		t.Errorf("revoked = %v, want none — nobody was removed from the grant list", revoker.revoked)
	}
}

// TestUpdateDmGrants_MissingCampaignIs404 pins the nil guard its sibling
// settings mutators already carry. Without it a missing campaign is a nil
// dereference, not a 404.
func TestUpdateDmGrants_MissingCampaignIs404(t *testing.T) {
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) { return nil, nil },
	}
	svc := &campaignService{repo: repo}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("UpdateDmGrants panicked on a missing campaign: %v", r)
		}
	}()
	err := svc.UpdateDmGrants(context.Background(), "camp-missing", []string{"u-a"})
	if err == nil {
		t.Fatal("no error for a missing campaign")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("error = %q, want a not-found", err.Error())
	}
}
