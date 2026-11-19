package campaigns

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/keyxmakerx/chronicle/internal/apperror"
)

// removedRig is a campaign whose settings and join code the fake repo keeps,
// so a removal can be followed through to what the join link then does.
type removedRig struct {
	camp    *Campaign
	members map[string]bool
	svc     *campaignService
}

func newRemovedRig(t *testing.T, joinCode string, members ...string) *removedRig {
	t.Helper()
	r := &removedRig{camp: &Campaign{ID: "camp-1", Name: "The Shattered Coast", Settings: "{}"}, members: map[string]bool{}}
	if joinCode != "" {
		r.camp.JoinCode = &joinCode
	}
	for _, m := range members {
		r.members[m] = true
	}
	repo := &mockCampaignRepo{
		findByIDFn: func(context.Context, string) (*Campaign, error) { return r.camp, nil },
		findByJoinCodeFn: func(_ context.Context, code string) (*Campaign, error) {
			if r.camp.JoinCode == nil || *r.camp.JoinCode != code {
				return nil, apperror.NewNotFound("invalid invite code")
			}
			return r.camp, nil
		},
		findMemberFn: func(_ context.Context, _, userID string) (*CampaignMember, error) {
			if r.members[userID] {
				return &CampaignMember{UserID: userID, Role: RolePlayer}, nil
			}
			return nil, apperror.NewNotFound("member not found")
		},
		removeMemberFn: func(_ context.Context, _, userID string) error {
			delete(r.members, userID)
			return nil
		},
		addMemberFn: func(_ context.Context, m *CampaignMember) error {
			r.members[m.UserID] = true
			return nil
		},
		updateSettingsFn: func(_ context.Context, _, settingsJSON string) error {
			r.camp.Settings = settingsJSON
			return nil
		},
		setJoinCodeFn: func(_ context.Context, _, code string) error {
			r.camp.JoinCode = &code
			return nil
		},
	}
	users := &mockUserFinder{findByIDFn: func(_ context.Context, id string) (*MemberUser, error) {
		if id == "gone" {
			return nil, apperror.NewNotFound("user not found")
		}
		return &MemberUser{ID: id, DisplayName: "Name of " + id}, nil
	}}
	r.svc = newTestCampaignService(repo, users).(*campaignService)
	return r
}

func (r *removedRig) removedIDs(t *testing.T) []string {
	t.Helper()
	var s CampaignSettings
	if err := json.Unmarshal([]byte(r.camp.Settings), &s); err != nil {
		t.Fatalf("settings: %v", err)
	}
	return s.RemovedMemberIDs
}

func TestRemoveMember_ReplacesJoinLinkAndTurnsThemAway(t *testing.T) {
	r := newRemovedRig(t, "oldcode", "rook", "sam")
	ctx := context.Background()

	if err := r.svc.RemoveMember(ctx, "camp-1", "rook"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if r.camp.JoinCode == nil || *r.camp.JoinCode == "oldcode" || *r.camp.JoinCode == "" {
		t.Fatalf("join code after a removal = %v, want a new one", r.camp.JoinCode)
	}
	if ids := r.removedIDs(t); len(ids) != 1 || ids[0] != "rook" {
		t.Fatalf("removed list = %v, want [rook]", ids)
	}

	// The old link is dead for everyone; the new one turns the removed
	// person away and lets anyone else in.
	if _, _, err := r.svc.JoinByCode(ctx, "oldcode", "newcomer"); err == nil {
		t.Fatal("the old link still works")
	}
	_, _, err := r.svc.JoinByCode(ctx, *r.camp.JoinCode, "rook")
	assertAppError(t, err, http.StatusForbidden)
	if r.members["rook"] {
		t.Fatal("the removed person is back in")
	}
	if _, joined, err := r.svc.JoinByCode(ctx, *r.camp.JoinCode, "newcomer"); err != nil || !joined {
		t.Fatalf("a newcomer with the new link: joined=%v err=%v", joined, err)
	}

	// Allowed back, the link works for them again.
	if err := r.svc.AllowRemovedMemberBack(ctx, "camp-1", "rook"); err != nil {
		t.Fatalf("AllowRemovedMemberBack: %v", err)
	}
	if ids := r.removedIDs(t); len(ids) != 0 {
		t.Fatalf("removed list after allowing back = %v", ids)
	}
	if _, joined, err := r.svc.JoinByCode(ctx, *r.camp.JoinCode, "rook"); err != nil || !joined {
		t.Fatalf("allowed back: joined=%v err=%v", joined, err)
	}
}

func TestRemoveMember_NoJoinLinkStaysOff(t *testing.T) {
	r := newRemovedRig(t, "", "rook")
	if err := r.svc.RemoveMember(context.Background(), "camp-1", "rook"); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if r.camp.JoinCode != nil {
		t.Fatalf("a removal turned the join link on: %q", *r.camp.JoinCode)
	}
}

// Someone who leaves by deleting their account is not remembered as removed
// and does not cost the owner their join link.
func TestLeavingByChoiceIsNotARemoval(t *testing.T) {
	r := newRemovedRig(t, "oldcode", "rook")
	if err := r.svc.removeMember(context.Background(), "camp-1", "rook", false); err != nil {
		t.Fatalf("removeMember: %v", err)
	}
	if ids := r.removedIDs(t); len(ids) != 0 {
		t.Fatalf("removed list = %v, want none", ids)
	}
	if *r.camp.JoinCode != "oldcode" {
		t.Fatal("the join link was replaced")
	}
}

func TestListRemovedMembers_LeavesOutReturnedAndDeleted(t *testing.T) {
	r := newRemovedRig(t, "", "back")
	r.camp.Settings = `{"removed_member_ids":["rook","back","gone"]}`
	got, err := r.svc.ListRemovedMembers(context.Background(), "camp-1")
	if err != nil {
		t.Fatalf("ListRemovedMembers: %v", err)
	}
	if len(got) != 1 || got[0].ID != "rook" || got[0].DisplayName != "Name of rook" {
		t.Fatalf("removed = %+v, want only rook", got)
	}
}

// Handing a campaign over answers the same for an email with no account as
// for an account outside the campaign.
func TestInitiateTransfer_OnlyToAMember(t *testing.T) {
	r := newRemovedRig(t, "", "owner", "sam")
	r.svc.users = &mockUserFinder{findByEmailFn: func(_ context.Context, email string) (*MemberUser, error) {
		switch email {
		case "sam@example.com":
			return &MemberUser{ID: "sam", Email: email}, nil
		case "outsider@example.com":
			return &MemberUser{ID: "outsider", Email: email}, nil
		}
		return nil, apperror.NewNotFound("user not found")
	}}
	ctx := context.Background()
	_, noAccount := r.svc.InitiateTransfer(ctx, "camp-1", "owner", "nobody@example.com")
	_, outsider := r.svc.InitiateTransfer(ctx, "camp-1", "owner", "outsider@example.com")
	assertAppError(t, noAccount, http.StatusBadRequest)
	assertAppError(t, outsider, http.StatusBadRequest)
	if noAccount.Error() != outsider.Error() {
		t.Fatalf("answers differ: %q vs %q", noAccount, outsider)
	}
	if _, err := r.svc.InitiateTransfer(ctx, "camp-1", "owner", "sam@example.com"); err != nil {
		t.Fatalf("handing over to a member: %v", err)
	}
}
