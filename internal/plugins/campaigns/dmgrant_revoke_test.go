package campaigns

// dmgrant_revoke_test.go — C-PERM-DMGRANT-REVOKE.
//
// Removing a member never cleared their co-DM grant, and the middleware
// honoured the grant without asking whether the user was still a member. On a
// PUBLIC campaign those two compose into a live leak: a removed co-DM keeps
// owner-level content visibility, because AllowPublicCampaignAccess admits an
// authenticated non-member as RoleNone and then sets IsDmGranted anyway, and
// VisibilityRole() returns RoleOwner for anyone carrying the grant.
//
// Private campaigns were never exposed — RequireCampaignAccess rejects a
// non-member before any of this runs — so the blast radius was public
// campaigns only. That is still a real hole, and these guards pin both halves
// of the fix: the STORED state (the grant is dropped on removal) and the
// HONOURED state (a grant is not obeyed for a non-member). Either alone leaves
// a gap: clearing on removal does nothing for campaigns already carrying a
// stale id, and gating at resolve time leaves wrong data for the next reader.

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
